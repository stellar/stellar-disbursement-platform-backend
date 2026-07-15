package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

// Test_AuditTables_walletDimension proves the audit requirement: disbursement and payment
// changes produce immutable audit rows that carry source_wallet_id — the audit log is
// wallet-attributable for compliance, dispute, and treasury queries.
func Test_AuditTables_walletDimension(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	wallet := EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Name: "audit-dim", SourceWalletID: wallet.ID, Status: StartedDisbursementStatus,
	})
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursement.Wallet.ID, ReadyReceiversWalletStatus)
	payment := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: receiverWallet, Disbursement: disbursement, Asset: *disbursement.Asset,
		Amount: "9", Status: ReadyPaymentStatus,
	})

	// Mutations produce audit rows.
	_, err = dbConnectionPool.ExecContext(ctx, `UPDATE disbursements SET name = 'audit-dim-renamed' WHERE id = $1`, disbursement.ID)
	require.NoError(t, err)
	_, err = dbConnectionPool.ExecContext(ctx, `UPDATE payments SET status = 'PAUSED' WHERE id = $1`, payment.ID)
	require.NoError(t, err)

	t.Run("disbursement audit rows carry source_wallet_id", func(t *testing.T) {
		var walletIDs []string
		require.NoError(t, dbConnectionPool.SelectContext(ctx, &walletIDs, `
			SELECT source_wallet_id FROM disbursements_audit WHERE id = $1`, disbursement.ID))
		require.GreaterOrEqual(t, len(walletIDs), 2, "INSERT + UPDATE rows expected")
		for _, id := range walletIDs {
			assert.Equal(t, wallet.ID, id)
		}
	})

	t.Run("payment audit rows carry source_wallet_id", func(t *testing.T) {
		var walletIDs []string
		require.NoError(t, dbConnectionPool.SelectContext(ctx, &walletIDs, `
			SELECT source_wallet_id FROM payments_audit WHERE id = $1`, payment.ID))
		require.GreaterOrEqual(t, len(walletIDs), 2)
		for _, id := range walletIDs {
			assert.Equal(t, wallet.ID, id)
		}
	})

	t.Run("disbursements_audit is append-only", func(t *testing.T) {
		_, dbErr := dbConnectionPool.ExecContext(ctx, `
			UPDATE disbursements_audit SET name = 'tampered' WHERE id = $1`, disbursement.ID)
		assert.ErrorContains(t, dbErr, "append-only")

		_, dbErr = dbConnectionPool.ExecContext(ctx, `
			DELETE FROM disbursements_audit WHERE id = $1`, disbursement.ID)
		assert.ErrorContains(t, dbErr, "append-only")
	})

	t.Run("payments_audit is append-only", func(t *testing.T) {
		_, dbErr := dbConnectionPool.ExecContext(ctx, `
			UPDATE payments_audit SET amount = '0' WHERE id = $1`, payment.ID)
		assert.ErrorContains(t, dbErr, "append-only")

		_, dbErr = dbConnectionPool.ExecContext(ctx, `
			DELETE FROM payments_audit WHERE id = $1`, payment.ID)
		assert.ErrorContains(t, dbErr, "append-only")
	})
}
