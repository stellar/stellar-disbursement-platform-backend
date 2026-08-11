package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

// Test_DistributionWallets_zeroActiveInvariant_serializesConcurrentArchives covers the DB-layer
// half of the zero-active-wallets invariant under concurrency.
//
// enforce_distribution_wallet_not_last_active decided from a plain, unlocked EXISTS, so two
// transactions each archiving one of the tenant's last two ACTIVE wallets each read the OTHER as
// still ACTIVE and both committed, leaving the tenant with zero. Migration 2026-08-11.0 closes
// that by taking a transaction-scoped, per-tenant advisory lock inside the trigger before the
// check. The service's own advisory lock is no substitute: this trigger is the backstop for
// writers that never reach the service (psql, data migrations, ops scripts), and they take no
// such lock — which is exactly what these two transactions model.
//
// Both subtests exercise the DB directly for that reason, and both fail against the pre-migration
// trigger: the first because the losing archive returns immediately instead of blocking, the
// second because both archives commit and no active wallet is left.
func Test_DistributionWallets_zeroActiveInvariant_serializesConcurrentArchives(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	const archiveQuery = `UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`
	const countActiveQuery = `SELECT COUNT(*) FROM distribution_wallets WHERE status = 'ACTIVE'`

	// lastTwoActiveWallets leaves the tenant holding EXACTLY two ACTIVE wallets — one archive away
	// from violating the invariant — and returns them. Neither is the default: the default wallet
	// is independently unarchivable (distribution_wallets_default_must_be_active), which would mask
	// the trigger under test.
	lastTwoActiveWallets := func(t *testing.T, namePrefix string) (string, string) {
		t.Helper()

		walletA := insertDistributionWallet(t, ctx, dbConnectionPool, namePrefix+"-a", nil, false)
		walletB := insertDistributionWallet(t, ctx, dbConnectionPool, namePrefix+"-b", nil, false)

		_, resetErr := dbConnectionPool.ExecContext(ctx, `
			UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW()
			WHERE status = 'ACTIVE' AND NOT is_default AND id NOT IN ($1, $2)`, walletA, walletB)
		require.NoError(t, resetErr)

		var active int
		require.NoError(t, dbConnectionPool.GetContext(ctx, &active, countActiveQuery))
		require.Equal(t, 2, active, "fixture must leave exactly two active wallets")

		return walletA, walletB
	}

	t.Run("a concurrent archive blocks instead of reading the pre-archive state", func(t *testing.T) {
		walletA, walletB := lastTwoActiveWallets(t, "zero-active-block")

		// TX1 archives one of the two and stays open, holding the tenant's serialization lock.
		tx1, txErr := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, txErr)
		defer func() { require.NoError(t, tx1.Rollback()) }()

		_, execErr := tx1.ExecContext(ctx, archiveQuery, walletA)
		require.NoError(t, execErr, "sanity: archiving the first of two active wallets is legal")

		// TX2 archives the other. It must wait for TX1 to resolve rather than evaluate the
		// invariant against TX1's uncommitted state — proven deterministically with lock_timeout
		// rather than a sleep, mirroring Test_WalletLifecycleTrigger_ArchiveRace.
		tx2, txErr := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, txErr)
		defer func() { require.NoError(t, tx2.Rollback()) }()

		_, execErr = tx2.ExecContext(ctx, "SET LOCAL lock_timeout = '1s'")
		require.NoError(t, execErr)

		_, execErr = tx2.ExecContext(ctx, archiveQuery, walletB)
		require.Error(t, execErr, "unserialized, this UPDATE succeeds immediately off TX1's stale pre-archive state")
		assert.ErrorContains(t, execErr, "canceling statement due to lock timeout")
	})

	t.Run("exactly one of two concurrent archives of the last two wallets succeeds", func(t *testing.T) {
		walletA, walletB := lastTwoActiveWallets(t, "zero-active-race")

		tx1, txErr := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, txErr)
		tx1Committed := false
		defer func() {
			if !tx1Committed {
				require.NoError(t, tx1.Rollback())
			}
		}()

		tx2, txErr := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, txErr)
		defer func() { require.NoError(t, tx2.Rollback()) }()

		// Bound TX2's wait so a regression that reintroduces an indefinite block fails the test
		// with a diagnosable error instead of hanging.
		_, execErr := tx2.ExecContext(ctx, "SET LOCAL lock_timeout = '25s'")
		require.NoError(t, execErr)

		// Have each transaction row-lock its own target before either evaluates the invariant.
		// The executor does this on their behalf anyway — a BEFORE UPDATE row trigger runs with
		// its target already locked — so this only pins down which interleaving runs. It is
		// deliberately the interleaving that deadlocks if the check is serialized by locking the
		// other candidate rows (FOR UPDATE) instead of on a per-tenant key.
		var lockedID string
		require.NoError(t, tx1.GetContext(ctx, &lockedID,
			`SELECT id FROM distribution_wallets WHERE id = $1 FOR NO KEY UPDATE`, walletA))
		require.NoError(t, tx2.GetContext(ctx, &lockedID,
			`SELECT id FROM distribution_wallets WHERE id = $1 FOR NO KEY UPDATE`, walletB))

		_, execErr = tx1.ExecContext(ctx, archiveQuery, walletA)
		require.NoError(t, execErr, "sanity: archiving the first of two active wallets is legal")

		tx2ArchiveErr := make(chan error, 1)
		go func() {
			_, archiveErr := tx2.ExecContext(ctx, archiveQuery, walletB)
			tx2ArchiveErr <- archiveErr
		}()

		select {
		case earlyErr := <-tx2ArchiveErr:
			t.Fatalf("TX2's archive resolved while TX1 was still open (err=%v): the two archives did not serialize", earlyErr)
		case <-time.After(time.Second):
		}

		require.NoError(t, tx1.Commit())
		tx1Committed = true

		select {
		case raceErr := <-tx2ArchiveErr:
			require.Error(t, raceErr, "the loser of the race must be rejected: committing it would leave zero active wallets")
			assert.ErrorContains(t, raceErr, "must keep at least one active wallet")
			assert.NotContains(t, raceErr.Error(), "deadlock",
				"the invariant must serialize on a per-tenant key, not on an inverted row-lock order")
		case <-time.After(30 * time.Second):
			t.Fatal("TX2's archive never resolved after TX1 committed")
		}

		var active int
		require.NoError(t, dbConnectionPool.GetContext(ctx, &active, countActiveQuery))
		assert.Equal(t, 1, active, "exactly one archive won; the tenant must still have an active wallet")
	})
}
