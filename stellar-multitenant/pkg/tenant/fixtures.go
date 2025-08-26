package tenant

import (
	"context"
	"fmt"
	"testing"

	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func DeleteAllTenantsFixture(t *testing.T, ctx context.Context, adminDBConnectionPool db.DBConnectionPool) {
	t.Helper()

	q := "DELETE FROM tenants"
	_, err := adminDBConnectionPool.ExecContext(ctx, q)
	require.NoError(t, err)

	var schemasToDrop []string
	q = "SELECT schema_name FROM information_schema.schemata WHERE schema_name ILIKE 'sdp_%'"
	err = adminDBConnectionPool.SelectContext(ctx, &schemasToDrop, q)
	require.NoError(t, err)

	for _, schema := range schemasToDrop {
		q = fmt.Sprintf("DROP SCHEMA %s CASCADE", pq.QuoteIdentifier(schema))
		_, err = adminDBConnectionPool.ExecContext(ctx, q)
		require.NoError(t, err)
	}
}

func AssertRegisteredAssetsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, expectedAssets []string) {
	t.Helper()

	var registeredAssets []string
	queryRegisteredAssets := `
		SELECT CONCAT(code, ':', issuer) FROM assets
	`
	err := dbConnectionPool.SelectContext(ctx, &registeredAssets, queryRegisteredAssets)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedAssets, registeredAssets)
}

func AssertRegisteredWalletsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, expectedWallets []string) {
	t.Helper()

	var registeredWallets []string
	queryRegisteredWallets := `
		SELECT name FROM wallets
	`
	err := dbConnectionPool.SelectContext(ctx, &registeredWallets, queryRegisteredWallets)
	require.NoError(t, err)

	// Check that all expected wallets are present (allows for additional wallets to exist)
	registeredMap := make(map[string]bool)
	for _, wallet := range registeredWallets {
		registeredMap[wallet] = true
	}

	for _, expectedWallet := range expectedWallets {
		assert.True(t, registeredMap[expectedWallet], "Expected wallet %s not found in registered wallets", expectedWallet)
	}
}

func AssertRegisteredUserFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, userFirstName, userLastName, userEmail string) {
	t.Helper()

	var user struct {
		FirstName string         `db:"first_name"`
		LastName  string         `db:"last_name"`
		Email     string         `db:"email"`
		Roles     pq.StringArray `db:"roles"`
		IsOwner   bool           `db:"is_owner"`
	}
	queryRegisteredUser := `
		SELECT first_name, last_name, email, roles, is_owner FROM auth_users WHERE email = $1
	`
	err := dbConnectionPool.GetContext(ctx, &user, queryRegisteredUser, userEmail)
	require.NoError(t, err)
	assert.Equal(t, userFirstName, user.FirstName)
	assert.Equal(t, userLastName, user.LastName)
	assert.Equal(t, userEmail, user.Email)
	assert.Equal(t, pq.StringArray{"owner"}, user.Roles)
	assert.True(t, user.IsOwner)
}

func CreateTenantFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, name, distributionPubKey string) *schema.Tenant {
	t.Helper()

	tenantName := name
	if name == "" {
		name, err := utils.RandomString(56)
		require.NoError(t, err)
		tenantName = name
	}

	const query = `
		WITH create_tenant AS (
			INSERT INTO tenants
				(name, distribution_account_address, base_url)
			VALUES
				($1, $2, $3)
			ON CONFLICT DO NOTHING
			RETURNING *
		)
		SELECT * FROM create_tenant ct
	`

	baseURL := fmt.Sprintf("http://%s.stellar.local:8000", tenantName)
	tnt := &schema.Tenant{
		Name:                       tenantName,
		DistributionAccountAddress: &distributionPubKey,
		BaseURL:                    &baseURL,
	}

	err := sqlExec.GetContext(ctx, tnt, query, tnt.Name, tnt.DistributionAccountAddress, tnt.BaseURL)
	require.Nil(t, err)

	return tnt
}

func LoadDefaultTenantInContext(t *testing.T, dbConnectionPool db.DBConnectionPool) (*schema.Tenant, context.Context) {
	ctx := context.Background()
	const publicKey = "GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL"
	tnt := CreateTenantFixture(t, ctx, dbConnectionPool, "default-tenant", publicKey)
	return tnt, sdpcontext.SetTenantInContext(ctx, tnt)
}

func CheckSchemaExistsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, schemaName string) bool {
	t.Helper()

	const q = `
		SELECT EXISTS(
			SELECT schema_name FROM information_schema.schemata WHERE schema_name = $1
		)
	`

	var exists bool
	err := dbConnectionPool.GetContext(ctx, &exists, q, schemaName)
	require.NoError(t, err)

	return exists
}

// TenantSchemaMatchTablesFixture asserts if the new tenant database schema has the tables passed by parameter.
func TenantSchemaMatchTablesFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, schemaName string, tableNames []string) {
	t.Helper()

	const q = `
		SELECT table_name FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name
	`

	var schemaTables []string
	err := dbConnectionPool.SelectContext(ctx, &schemaTables, q, schemaName)
	require.NoError(t, err)

	assert.ElementsMatch(t, tableNames, schemaTables)
}

func ApplyMigrationsForTenantFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, tenantName string) {
	t.Helper()

	m := NewManager(WithDatabase(dbConnectionPool))
	dsn, err := m.GetDSNForTenant(ctx, tenantName)
	require.NoError(t, err)

	_, err = db.Migrate(dsn, migrate.Up, 0, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	_, err = db.Migrate(dsn, migrate.Up, 0, migrations.AuthMigrationRouter)
	require.NoError(t, err)
}

func PrepareDBForTenant(t *testing.T, dbt *dbtest.DB, tenantName string) string {
	t.Helper()

	conn := dbt.Open()
	defer conn.Close()

	ctx := context.Background()
	schemaName := fmt.Sprintf("sdp_%s", tenantName)
	_, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", pq.QuoteIdentifier(schemaName)))
	require.NoError(t, err)

	tDSN, err := router.GetDSNForTenant(dbt.DSN, tenantName)
	require.NoError(t, err)

	_, err = db.Migrate(tDSN, migrate.Up, 0, migrations.SDPMigrationRouter)
	require.NoError(t, err)

	_, err = db.Migrate(tDSN, migrate.Up, 0, migrations.AuthMigrationRouter)
	require.NoError(t, err)

	return tDSN
}
