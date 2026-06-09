package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

// insertDistributionWallet is a test helper that inserts a distribution wallet row with raw SQL,
// returning the generated id. Pass nil address for wallets without a provisioned account yet.
func insertDistributionWallet(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, name string, address *string, isDefault bool) string {
	t.Helper()

	const q = `
		INSERT INTO distribution_wallets (name, distribution_account_address, distribution_account_type, is_default)
		VALUES ($1, $2, 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', $3)
		RETURNING id
	`
	var id string
	err := sqlExec.GetContext(ctx, &id, q, name, address, isDefault)
	require.NoError(t, err)
	return id
}

func Test_DistributionWallets_schema_invariants(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Run("happy path: defaults are populated", func(t *testing.T) {
		id := insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-defaults", nil, false)

		var got struct {
			Status            string `db:"status"`
			AccountStatus     string `db:"distribution_account_status"`
			IsDefault         bool   `db:"is_default"`
			HasCreatedAt      bool   `db:"has_created_at"`
			HasUpdatedAt      bool   `db:"has_updated_at"`
			ArchivedAtIsNull  bool   `db:"archived_at_is_null"`
			DescriptionIsNull bool   `db:"description_is_null"`
		}
		err := dbConnectionPool.GetContext(ctx, &got, `
			SELECT status, distribution_account_status, is_default,
				created_at IS NOT NULL AS has_created_at,
				updated_at IS NOT NULL AS has_updated_at,
				archived_at IS NULL AS archived_at_is_null,
				description IS NULL AS description_is_null
			FROM distribution_wallets WHERE id = $1`, id)
		require.NoError(t, err)

		assert.Equal(t, "ACTIVE", got.Status)
		assert.Equal(t, "ACTIVE", got.AccountStatus)
		assert.False(t, got.IsDefault)
		assert.True(t, got.HasCreatedAt)
		assert.True(t, got.HasUpdatedAt)
		assert.True(t, got.ArchivedAtIsNull)
		assert.True(t, got.DescriptionIsNull)
	})

	t.Run("updated_at trigger refreshes on UPDATE", func(t *testing.T) {
		id := insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-trigger", nil, false)

		var bumped bool
		err := dbConnectionPool.GetContext(ctx, &bumped, `
			WITH before AS (SELECT updated_at FROM distribution_wallets WHERE id = $1),
			upd AS (
				UPDATE distribution_wallets SET description = 'touched'
				WHERE id = $1 RETURNING updated_at
			)
			SELECT upd.updated_at >= before.updated_at FROM upd, before`, id)
		require.NoError(t, err)
		assert.True(t, bumped)
	})

	t.Run("at most one default wallet per tenant", func(t *testing.T) {
		id := insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-default-1", nil, true)

		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type, is_default)
			VALUES ('wallet-default-2', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', TRUE)`)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_wallets_single_default_idx")

		// Cleanup the default so sibling tests can create their own.
		_, err = dbConnectionPool.ExecContext(ctx, `DELETE FROM distribution_wallets WHERE id = $1`, id)
		require.NoError(t, err)
	})

	t.Run("wallet names are unique", func(t *testing.T) {
		insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-dupe-name", nil, false)

		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type)
			VALUES ('wallet-dupe-name', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT')`)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_wallets_name_unique_idx")
	})

	t.Run("addresses are unique when set, NULL allowed repeatedly", func(t *testing.T) {
		addr := "GDUMMYADDRESSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-addr-1", &addr, false)

		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_address, distribution_account_type)
			VALUES ('wallet-addr-2', $1, 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT')`, addr)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_wallets_address_unique_idx")

		// Multiple NULL addresses are fine (e.g. CIRCLE wallets pending activation).
		insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-addr-null-1", nil, false)
		insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-addr-null-2", nil, false)
	})

	t.Run("invalid wallet status is rejected", func(t *testing.T) {
		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type, status)
			VALUES ('wallet-bad-status', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'DELETED')`)
		require.Error(t, err)
		assert.ErrorContains(t, err, "status")
	})

	t.Run("invalid distribution_account_type is rejected", func(t *testing.T) {
		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type)
			VALUES ('wallet-bad-type', 'CHANNEL_ACCOUNT.STELLAR.DB')`)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_account_type")
	})

	t.Run("default wallet cannot be archived", func(t *testing.T) {
		// Direct insert of an archived default is rejected.
		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type, is_default, status, archived_at)
			VALUES ('wallet-archived-default', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', TRUE, 'ARCHIVED', NOW())`)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_wallets_default_must_be_active")

		// Updating the current default to ARCHIVED is rejected too.
		id := insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-live-default", nil, true)
		_, err = dbConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, id)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_wallets_default_must_be_active")

		// Cleanup the default so sibling tests can create their own.
		_, err = dbConnectionPool.ExecContext(ctx, `DELETE FROM distribution_wallets WHERE id = $1`, id)
		require.NoError(t, err)
	})

	t.Run("archived_at is set iff wallet is archived", func(t *testing.T) {
		// ARCHIVED without archived_at → rejected.
		_, err := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type, status)
			VALUES ('wallet-archived-no-ts', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'ARCHIVED')`)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_wallets_archived_at_consistency")

		// ACTIVE with archived_at → rejected.
		_, err = dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type, status, archived_at)
			VALUES ('wallet-active-with-ts', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'ACTIVE', NOW())`)
		require.Error(t, err)
		assert.ErrorContains(t, err, "distribution_wallets_archived_at_consistency")

		// ARCHIVED with archived_at → accepted.
		_, err = dbConnectionPool.ExecContext(ctx, `
			INSERT INTO distribution_wallets (name, distribution_account_type, status, archived_at)
			VALUES ('wallet-archived-ok', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'ARCHIVED', NOW())`)
		require.NoError(t, err)
	})

	t.Run("audit table records changes", func(t *testing.T) {
		id := insertDistributionWallet(t, ctx, dbConnectionPool, "wallet-audited", nil, false)
		_, err := dbConnectionPool.ExecContext(ctx,
			`UPDATE distribution_wallets SET description = 'audit me' WHERE id = $1`, id)
		require.NoError(t, err)

		var auditCount int
		err = dbConnectionPool.GetContext(ctx, &auditCount,
			`SELECT COUNT(*) FROM distribution_wallets_audit WHERE id = $1`, id)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, auditCount, 2, "expected INSERT + UPDATE audit entries")
	})
}
