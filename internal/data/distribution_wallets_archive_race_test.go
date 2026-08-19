package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

// Test_WalletLifecycleTrigger_ArchiveRace proves enforce_wallet_membership_wallet_active's
// SELECT ... FOR SHARE actually blocks on a concurrent, uncommitted archive — the race that
// migration 2026-07-30.2 closes. Before that fix, the SELECT was unlocked and this second
// transaction would have read the pre-archive status and succeeded immediately, regardless of
// the first transaction's outcome. enforce_disbursement_source_wallet_active got the identical
// fix in the same migration; proving the mechanism once here (via the simpler table) covers both.
func Test_WalletLifecycleTrigger_ArchiveRace(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// A second, non-default ACTIVE wallet — the one about to be archived mid-flight.
	EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('archive-race-wallet', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	// TX1: start (but do not commit) archiving the wallet — mirrors ArchiveWallet's plain
	// UPDATE, which takes an implicit FOR NO KEY UPDATE row lock.
	tx1, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx1.Rollback()) }()

	_, err = tx1.ExecContext(ctx,
		`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletID)
	require.NoError(t, err)

	// TX2: attempt a membership grant against the same wallet while TX1 is still open and
	// uncommitted. enforce_wallet_membership_wallet_active's FOR SHARE SELECT must block behind
	// TX1's lock rather than reading the stale pre-archive status — proven deterministically via
	// lock_timeout instead of a sleep-based race, mirroring this codebase's existing pattern for
	// asserting row-lock contention (see Test_PaymentModel_GetBatchForUpdate).
	tx2, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx2.Rollback()) }()

	_, err = tx2.ExecContext(ctx, "SET LOCAL lock_timeout = '1s'")
	require.NoError(t, err)

	_, err = tx2.ExecContext(ctx,
		`INSERT INTO wallet_memberships (user_id, wallet_id, role) VALUES ('race-test-user', $1, 'developer')`, walletID)
	assert.ErrorContains(t, err, "canceling statement due to lock timeout",
		"the trigger's SELECT must block on TX1's uncommitted archive, not read the stale pre-archive status")
}
