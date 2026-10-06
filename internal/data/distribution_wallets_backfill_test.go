package data

import (
	"context"
	"fmt"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
)

// Test_DistributionWallets_default_backfill exercises the default-wallet shim migration
// against a production-like layout: a real `admin` schema holding `tenants`, plus one
// `sdp_<name>` schema per tenant, migrated through the same router used in production.
func Test_DistributionWallets_default_backfill(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// 1. Production-like admin layout: `admin` schema + admin migrations.
	_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA admin")
	require.NoError(t, err)
	adminDSN, err := router.GetDSNForAdmin(dbt.DSN)
	require.NoError(t, err)
	_, err = db.Migrate(adminDSN, migrate.Up, 0, migrations.AdminMigrationRouter)
	require.NoError(t, err)

	// 2. Three tenants in admin.tenants:
	//    - provisioned: stellar account active
	//    - pending: CIRCLE account awaiting user activation (NULL address)
	//    - ghost: soft-deleted tenant (must NOT receive a wallet)
	const provisionedAddr = "GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL"
	_, err = dbConnectionPool.ExecContext(ctx, `
		INSERT INTO admin.tenants (name, distribution_account_address, distribution_account_type, distribution_account_status)
		VALUES
			('provisioned', $1, 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'ACTIVE'),
			('pending', NULL, 'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT', 'PENDING_USER_ACTIVATION')`,
		provisionedAddr)
	require.NoError(t, err)
	_, err = dbConnectionPool.ExecContext(ctx, `
		INSERT INTO admin.tenants (name, distribution_account_address, deleted_at)
		VALUES ('ghost', 'GBT4ZH4QSJUKM5SLMCXTJZBSHPWNAFPV3OWLDPNUTC7TIM2FUDQXGHOST', NOW())`)
	require.NoError(t, err)

	// 3. Create + migrate each tenant schema through the production router.
	for _, tenantName := range []string{"provisioned", "pending", "ghost"} {
		_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA sdp_%s", tenantName))
		require.NoError(t, err)
		tenantDSN, dsnErr := router.GetDSNForTenant(dbt.DSN, tenantName)
		require.NoError(t, dsnErr)
		_, err = db.Migrate(tenantDSN, migrate.Up, 0, migrations.SDPMigrationRouter)
		require.NoError(t, err)
	}

	type walletRow struct {
		Name          string  `db:"name"`
		Address       *string `db:"distribution_account_address"`
		AccountType   string  `db:"distribution_account_type"`
		AccountStatus string  `db:"distribution_account_status"`
		Status        string  `db:"status"`
		IsDefault     bool    `db:"is_default"`
	}
	getWallets := func(schemaName string) []walletRow {
		var rows []walletRow
		err = dbConnectionPool.SelectContext(ctx, &rows, fmt.Sprintf(`
			SELECT name, distribution_account_address, distribution_account_type,
				distribution_account_status, status, is_default
			FROM %s.distribution_wallets ORDER BY name`, schemaName))
		require.NoError(t, err)
		return rows
	}

	t.Run("provisioned tenant gets default wallet pointing at its current account", func(t *testing.T) {
		rows := getWallets("sdp_provisioned")
		require.Len(t, rows, 1)
		assert.Equal(t, "default", rows[0].Name)
		require.NotNil(t, rows[0].Address)
		assert.Equal(t, provisionedAddr, *rows[0].Address)
		assert.Equal(t, "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT", rows[0].AccountType)
		assert.Equal(t, "ACTIVE", rows[0].AccountStatus)
		assert.Equal(t, "ACTIVE", rows[0].Status)
		assert.True(t, rows[0].IsDefault)
	})

	t.Run("pending CIRCLE tenant gets default wallet with NULL address", func(t *testing.T) {
		rows := getWallets("sdp_pending")
		require.Len(t, rows, 1)
		assert.Equal(t, "default", rows[0].Name)
		assert.Nil(t, rows[0].Address)
		assert.Equal(t, "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT", rows[0].AccountType)
		assert.Equal(t, "PENDING_USER_ACTIVATION", rows[0].AccountStatus)
		assert.True(t, rows[0].IsDefault)
	})

	t.Run("soft-deleted tenant gets no wallet", func(t *testing.T) {
		assert.Empty(t, getWallets("sdp_ghost"))
	})

	t.Run("flat test layout (no admin schema) is a no-op", func(t *testing.T) {
		// db/dbtest.Open applies every migration domain to one flat schema with no
		// admin schema; the shim must skip silently. Assert against this layout.
		flat := dbtest.Open(t)
		defer flat.Close()
		flatPool, openErr := db.OpenDBConnectionPool(flat.DSN)
		require.NoError(t, openErr)
		defer flatPool.Close()

		var count int
		require.NoError(t, flatPool.GetContext(ctx, &count, "SELECT COUNT(*) FROM distribution_wallets"))
		assert.Zero(t, count)
	})
}
