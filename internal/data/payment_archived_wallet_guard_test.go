package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

// Test_PaymentSourceWalletActiveTrigger covers enforce_payment_source_wallet_active
// (2026-08-07.1): direct payments are the only fund-moving write that states its own source
// wallet, and until that migration the sole archive check on them was in the HTTP layer, on a
// different connection from the INSERT.
func Test_PaymentSourceWalletActiveTrigger(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	// The default wallet stays ACTIVE so the wallets under test can legally be archived
	// (a tenant must keep at least one active wallet, and the default can never be archived).
	EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)

	newActiveWallet := func(name string) string {
		t.Helper()
		var id string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &id, `
			INSERT INTO distribution_wallets (name, distribution_account_type)
			VALUES ($1, 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`, name))
		return id
	}

	asset := GetAssetFixture(t, ctx, dbConnectionPool, FixtureAssetUSDC)
	receiverWalletProvider := CreateWalletFixture(t, ctx, dbConnectionPool,
		"Guard Wallet", "https://guard.com", "guard.com", "guard://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool,
		receiver.ID, receiverWalletProvider.ID, RegisteredReceiversWalletStatus)

	insertDirectPayment := func(sqlExec db.SQLExecuter, sourceWalletID string) error {
		_, execErr := sqlExec.ExecContext(ctx, `
			INSERT INTO payments (amount, asset_id, receiver_id, receiver_wallet_id, type, source_wallet_id)
			VALUES ('5', $1, $2, $3, 'DIRECT', $4)`,
			asset.ID, receiver.ID, receiverWallet.ID, sourceWalletID)
		return execErr
	}

	t.Run("direct payment on an archived wallet is rejected", func(t *testing.T) {
		walletID := newActiveWallet("guard-archived")
		require.NoError(t, insertDirectPayment(dbConnectionPool, walletID), "sanity: allowed while ACTIVE")

		_, err = dbConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletID)
		require.NoError(t, err)

		err = insertDirectPayment(dbConnectionPool, walletID)
		require.Error(t, err)
		// The guard rejects any non-ACTIVE status, so the message names the status.
		assert.ErrorContains(t, err, "is not active")
		assert.ErrorContains(t, err, "ARCHIVED")
	})

	t.Run("disbursement payments are out of scope: they inherit an already-gated wallet", func(t *testing.T) {
		walletID := newActiveWallet("guard-disbursement")
		disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
			Name: "guard-disbursement-parent", SourceWalletID: walletID,
			Wallet: receiverWalletProvider, Asset: asset,
		})

		_, err = dbConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletID)
		require.NoError(t, err)

		// Archiving mid-flight must not strand a disbursement that is still accepting
		// instructions — the API deliberately permits this (PostDisbursementInstructions
		// checks entitlement, not wallet status).
		_, err = dbConnectionPool.ExecContext(ctx, `
			INSERT INTO payments (amount, asset_id, receiver_id, receiver_wallet_id, type, disbursement_id)
			VALUES ('5', $1, $2, $3, 'DISBURSEMENT', $4)`,
			asset.ID, receiver.ID, receiverWallet.ID, disbursement.ID)
		require.NoError(t, err)
	})

	// Mirrors Test_WalletLifecycleTrigger_ArchiveRace: the guard is only worth anything if it
	// also blocks on an archive that has not committed yet, which is precisely the window the
	// HTTP-layer check cannot see. Proven deterministically with lock_timeout rather than a sleep.
	t.Run("direct payment blocks on an uncommitted archive", func(t *testing.T) {
		walletID := newActiveWallet("guard-race")

		tx1, txErr := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, txErr)
		defer func() { require.NoError(t, tx1.Rollback()) }()

		_, err = tx1.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletID)
		require.NoError(t, err)

		tx2, txErr := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, txErr)
		defer func() { require.NoError(t, tx2.Rollback()) }()

		_, err = tx2.ExecContext(ctx, "SET LOCAL lock_timeout = '1s'")
		require.NoError(t, err)

		err = insertDirectPayment(tx2, walletID)
		assert.ErrorContains(t, err, "canceling statement due to lock timeout",
			"the trigger's SELECT must block on TX1's uncommitted archive, not read the stale pre-archive status")
	})
}
