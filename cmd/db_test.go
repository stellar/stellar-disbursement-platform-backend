package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getSDPMigrationsApplied(t *testing.T, ctx context.Context, db db.DBConnectionPool) []string {
	t.Helper()

	rows, err := db.QueryContext(ctx, "SELECT id FROM gorp_migrations")
	require.NoError(t, err)
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		err := rows.Scan(&id)
		require.NoError(t, err)

		ids = append(ids, id)
	}

	require.NoError(t, rows.Err())

	return ids
}

func getAuthMigrationsApplied(t *testing.T, ctx context.Context, db db.DBConnectionPool) []string {
	t.Helper()

	rows, err := db.QueryContext(ctx, "SELECT id FROM auth_migrations")
	require.NoError(t, err)
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		err := rows.Scan(&id)
		require.NoError(t, err)

		ids = append(ids, id)
	}

	require.NoError(t, rows.Err())

	return ids
}

func Test_DatabaseCommand_db_help(t *testing.T) {
	buf := new(strings.Builder)

	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	rootCmd.SetArgs([]string{"db"})
	rootCmd.SetOut(buf)
	err := rootCmd.Execute()
	require.NoError(t, err)

	expectedContains := []string{
		"Database related commands",
		"stellar-disbursement-platform db [flags]",
		"stellar-disbursement-platform db [command]",
		"auth              Stellar Auth schema migration helpers",
		"migrate           Schema migration helpers",
		"setup-for-network Set up the assets and wallets registered in the database based on the network passphrase.",
		"--all                Apply the migrations to all tenants. (ALL)",
		"-h, --help               help for db",
		"--tenant-id string   The tenant ID where the migrations will be applied. (TENANT_ID)",
		`--base-url string             The SDP backend server's base URL. (BASE_URL) (default "http://localhost:8000")`,
		`--database-url string         Postgres DB URL (DATABASE_URL) (default "postgres://localhost:5432/sdp?sslmode=disable")`,
		`--environment string          The environment where the application is running. Example: "development", "staging", "production". (ENVIRONMENT) (default "development")`,
		`--log-level string            The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")`,
		`--network-passphrase string   The Stellar network passphrase (NETWORK_PASSPHRASE) (default "Test SDF Network ; September 2015")`,
		`--sentry-dsn string           The DSN (client key) of the Sentry project. If not provided, Sentry will not be used. (SENTRY_DSN)`,
	}

	output := buf.String()
	for _, expected := range expectedContains {
		assert.Contains(t, output, expected)
	}

	buf.Reset()
	rootCmd.SetArgs([]string{"db", "--help"})
	err = rootCmd.Execute()
	require.NoError(t, err)

	output = buf.String()
	for _, expected := range expectedContains {
		assert.Contains(t, output, expected)
	}
}

func Test_DatabaseCommand_db_migrate(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	buf := new(strings.Builder)

	t.Run("migrate usage", func(t *testing.T) {
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{"db", "migrate"})
		rootCmd.SetOut(buf)
		err = rootCmd.Execute()
		require.NoError(t, err)

		expectedContains := []string{
			"Schema migration helpers",
			"stellar-disbursement-platform db migrate [flags]",
			"stellar-disbursement-platform db migrate [command]",
			"down        Migrates database down [count] migrations",
			"up          Migrates database up [count]",
			"-h, --help   help for migrate",
			`--all                         Apply the migrations to all tenants. (ALL)`,
			`--base-url string             The SDP backend server's base URL. (BASE_URL) (default "http://localhost:8000")`,
			`--database-url string         Postgres DB URL (DATABASE_URL) (default "postgres://localhost:5432/sdp?sslmode=disable")`,
			`--environment string          The environment where the application is running. Example: "development", "staging", "production". (ENVIRONMENT) (default "development")`,
			`--log-level string            The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")`,
			`--network-passphrase string   The Stellar network passphrase (NETWORK_PASSPHRASE) (default "Test SDF Network ; September 2015")`,
			`--sentry-dsn string           The DSN (client key) of the Sentry project. If not provided, Sentry will not be used. (SENTRY_DSN)`,
			`--tenant-id string            The tenant ID where the migrations will be applied. (TENANT_ID)`,
		}

		output := buf.String()
		for _, expected := range expectedContains {
			assert.Contains(t, output, expected)
		}
	})

	t.Run("migrate up and down --all", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		// Creating Tenants
		tnt1, err := m.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tnt2, err := m.AddTenant(ctx, "myorg2")
		require.NoError(t, err)

		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg1")
		require.NoError(t, err)
		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg2")
		require.NoError(t, err)

		buf.Reset()
		log.DefaultLogger.SetOutput(buf)
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{"db", "migrate", "up", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--all"})
		err = rootCmd.Execute()
		require.NoError(t, err)

		tnt1DSN, err := m.GetDSNForTenant(ctx, tnt1.Name)
		require.NoError(t, err)

		tnt2DSN, err := m.GetDSNForTenant(ctx, tnt2.Name)
		require.NoError(t, err)

		tenant1SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt1DSN)
		require.NoError(t, err)
		defer tenant1SchemaConnectionPool.Close()

		tenant2SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt2DSN)
		require.NoError(t, err)
		defer tenant2SchemaConnectionPool.Close()

		// Checking if the migrations were applied on the Tenant 1
		ids := getSDPMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{"2023-01-20.0-initial.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"gorp_migrations"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getSDPMigrationsApplied(t, ctx, tenant2SchemaConnectionPool)
		assert.Equal(t, []string{"2023-01-20.0-initial.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"gorp_migrations"})

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--all"})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1
		ids = getSDPMigrationsApplied(t, context.Background(), tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"gorp_migrations"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getSDPMigrationsApplied(t, context.Background(), tenant2SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"gorp_migrations"})
	})

	t.Run("migrate up and down --tenant-id", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		// Creating Tenants
		tnt1, err := m.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tnt2, err := m.AddTenant(ctx, "myorg2")
		require.NoError(t, err)

		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg1")
		require.NoError(t, err)
		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg2")
		require.NoError(t, err)

		buf.Reset()
		log.DefaultLogger.SetOutput(buf)
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{"db", "migrate", "up", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--tenant-id", tnt1.ID})
		err = rootCmd.Execute()
		require.NoError(t, err)

		tnt1DSN, err := m.GetDSNForTenant(ctx, tnt1.Name)
		require.NoError(t, err)

		tnt2DSN, err := m.GetDSNForTenant(ctx, tnt2.Name)
		require.NoError(t, err)

		tenant1SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt1DSN)
		require.NoError(t, err)
		defer tenant1SchemaConnectionPool.Close()

		tenant2SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt2DSN)
		require.NoError(t, err)
		defer tenant2SchemaConnectionPool.Close()

		// Checking if the migrations were applied on the Tenant 1 schema
		ids := getSDPMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{"2023-01-20.0-initial.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"gorp_migrations"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt2.ID))

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--tenant-id", tnt1.ID})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1 schema
		ids = getSDPMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"gorp_migrations"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt2.ID))
	})

	t.Run("migrate up and down auth migrations --all", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		// Creating Tenants
		tnt1, err := m.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tnt2, err := m.AddTenant(ctx, "myorg2")
		require.NoError(t, err)

		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg1")
		require.NoError(t, err)
		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg2")
		require.NoError(t, err)

		buf.Reset()
		log.DefaultLogger.SetOutput(buf)
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{"db", "auth", "migrate", "up", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--all"})
		err = rootCmd.Execute()
		require.NoError(t, err)

		tnt1DSN, err := m.GetDSNForTenant(ctx, tnt1.Name)
		require.NoError(t, err)

		tnt2DSN, err := m.GetDSNForTenant(ctx, tnt2.Name)
		require.NoError(t, err)

		tenant1SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt1DSN)
		require.NoError(t, err)
		defer tenant1SchemaConnectionPool.Close()

		tenant2SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt2DSN)
		require.NoError(t, err)
		defer tenant2SchemaConnectionPool.Close()

		// Checking if the migrations were applied on the Tenant 1
		ids := getAuthMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{"2023-02-09.0.add-users-table.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations", "auth_users"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getAuthMigrationsApplied(t, ctx, tenant2SchemaConnectionPool)
		assert.Equal(t, []string{"2023-02-09.0.add-users-table.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"auth_migrations", "auth_users"})

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "auth", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--all"})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1
		ids = getAuthMigrationsApplied(t, context.Background(), tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getAuthMigrationsApplied(t, context.Background(), tenant2SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"auth_migrations"})
	})

	t.Run("migrate up and down auth migrations --tenant-id", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		// Creating Tenants
		tnt1, err := m.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		tnt2, err := m.AddTenant(ctx, "myorg2")
		require.NoError(t, err)

		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg1")
		require.NoError(t, err)
		_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg2")
		require.NoError(t, err)

		buf.Reset()
		log.DefaultLogger.SetOutput(buf)
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{"db", "auth", "migrate", "up", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--tenant-id", tnt1.ID})
		err = rootCmd.Execute()
		require.NoError(t, err)

		tnt1DSN, err := m.GetDSNForTenant(ctx, tnt1.Name)
		require.NoError(t, err)

		tnt2DSN, err := m.GetDSNForTenant(ctx, tnt2.Name)
		require.NoError(t, err)

		tenant1SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt1DSN)
		require.NoError(t, err)
		defer tenant1SchemaConnectionPool.Close()

		tenant2SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt2DSN)
		require.NoError(t, err)
		defer tenant2SchemaConnectionPool.Close()

		// Checking if the migrations were applied on the Tenant 1 schema
		ids := getAuthMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{"2023-02-09.0.add-users-table.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations", "auth_users"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt2.ID))

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "auth", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--tenant-id", tnt1.ID})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1 schema
		ids = getAuthMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("applying migrations on tenant ID %s", tnt2.ID))
	})
}

func Test_DatabaseCommand_db_setup_for_network(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	// Creating Tenants
	tnt1, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)

	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)

	_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg1")
	require.NoError(t, err)
	_, err = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg2")
	require.NoError(t, err)

	tenant.ApplyMigrationsForTenantFixture(t, ctx, dbConnectionPool, tnt1.Name)
	tenant.ApplyMigrationsForTenantFixture(t, ctx, dbConnectionPool, tnt2.Name)

	tnt1DSN, err := m.GetDSNForTenant(ctx, tnt1.Name)
	require.NoError(t, err)

	tnt2DSN, err := m.GetDSNForTenant(ctx, tnt2.Name)
	require.NoError(t, err)

	tenant1SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt1DSN)
	require.NoError(t, err)
	defer tenant1SchemaConnectionPool.Close()

	tenant2SchemaConnectionPool, err := db.OpenDBConnectionPool(tnt2DSN)
	require.NoError(t, err)
	defer tenant2SchemaConnectionPool.Close()

	testnetUSDCIssuer := keypair.MustRandom().Address()
	clearTenantTables := func(t *testing.T, ctx context.Context, tenantSchemaConnectionPool db.DBConnectionPool) {
		models, mErr := data.NewModels(tenantSchemaConnectionPool)
		require.NoError(t, mErr)

		// Assets
		data.DeleteAllWalletFixtures(t, ctx, tenantSchemaConnectionPool)
		data.DeleteAllAssetFixtures(t, ctx, tenantSchemaConnectionPool)

		data.CreateAssetFixture(t, ctx, tenantSchemaConnectionPool, "USDC", testnetUSDCIssuer)

		assets, aErr := models.Assets.GetAll(ctx)
		require.NoError(t, aErr)

		assert.Len(t, assets, 1)
		assert.Equal(t, "USDC", assets[0].Code)
		assert.Equal(t, testnetUSDCIssuer, assets[0].Issuer)

		// Wallets
		data.CreateWalletFixture(t, ctx, tenantSchemaConnectionPool, "Vibrant Assist", "https://vibrantapp.com", "api-dev.vibrantapp.com", "https://vibrantapp.com/sdp-dev")

		wallets, wErr := models.Wallets.GetAll(ctx)
		require.NoError(t, wErr)

		assert.Len(t, wallets, 1)
		assert.Equal(t, "Vibrant Assist", wallets[0].Name)
		assert.Equal(t, "https://vibrantapp.com", wallets[0].Homepage)
		assert.Equal(t, "api-dev.vibrantapp.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-dev", wallets[0].DeepLinkSchema)
	}

	buf := new(strings.Builder)
	log.DefaultLogger.SetLevel(log.InfoLevel)
	log.DefaultLogger.SetOutput(buf)

	t.Run("run for all tenants", func(t *testing.T) {
		clearTenantTables(t, ctx, tenant1SchemaConnectionPool)
		clearTenantTables(t, ctx, tenant2SchemaConnectionPool)

		// Setup
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{
			"db",
			"setup-for-network",
			"--database-url",
			dbt.DSN,
			"--network-passphrase",
			network.PublicNetworkPassphrase,
			"--all",
		})

		err = rootCmd.Execute()
		require.NoError(t, err)

		// Tenant 1
		models, mErr := data.NewModels(tenant1SchemaConnectionPool)
		require.NoError(t, mErr)

		// Validating assets
		assets, aErr := models.Assets.GetAll(ctx)
		require.NoError(t, aErr)

		assert.Len(t, assets, 2)
		assert.Equal(t, "USDC", assets[0].Code)
		assert.NotEqual(t, testnetUSDCIssuer, assets[0].Issuer)
		assert.Equal(t, services.DefaultAssetsNetworkMap[utils.PubnetNetworkType]["USDC"], assets[0].Issuer)
		assert.Equal(t, "XLM", assets[1].Code)
		assert.Empty(t, assets[1].Issuer)

		// Validating wallets
		wallets, wErr := models.Wallets.GetAll(ctx)
		require.NoError(t, wErr)

		assert.Len(t, wallets, 2)
		// assert.Equal(t, "Beans App", wallets[0].Name)
		// assert.Equal(t, "https://www.beansapp.com/disbursements", wallets[0].Homepage)
		// assert.Equal(t, "api.beansapp.com", wallets[0].SEP10ClientDomain)
		// assert.Equal(t, "https://www.beansapp.com/disbursements/registration?redirect=true", wallets[0].DeepLinkSchema)
		assert.Equal(t, "Vibrant Assist", wallets[0].Name)
		assert.Equal(t, "https://vibrantapp.com/vibrant-assist", wallets[0].Homepage)
		assert.Equal(t, "api.vibrantapp.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp", wallets[0].DeepLinkSchema)

		assert.Equal(t, "Vibrant Assist RC", wallets[1].Name)
		assert.Equal(t, "vibrantapp.com/vibrant-assist", wallets[1].Homepage)
		assert.Equal(t, "vibrantapp.com", wallets[1].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-rc", wallets[1].DeepLinkSchema)

		expectedLogs := []string{
			fmt.Sprintf("running for tenant ID %s", tnt1.ID),
			"updating/inserting assets for the 'pubnet' network",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", services.DefaultAssetsNetworkMap[utils.PubnetNetworkType]["USDC"]),
			"Code: XLM",
			"Issuer: ",
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://vibrantapp.com/sdp",
			"SEP-10 Client Domain: api.vibrantapp.com",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}

		// Tenant 2
		models, err = data.NewModels(tenant2SchemaConnectionPool)
		require.NoError(t, err)

		// Validating assets
		assets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, assets, 2)
		assert.Equal(t, "USDC", assets[0].Code)
		assert.NotEqual(t, testnetUSDCIssuer, assets[0].Issuer)
		assert.Equal(t, services.DefaultAssetsNetworkMap[utils.PubnetNetworkType]["USDC"], assets[0].Issuer)
		assert.Equal(t, "XLM", assets[1].Code)
		assert.Empty(t, assets[1].Issuer)

		// Validating wallets
		wallets, err = models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, wallets, 2)
		// assert.Equal(t, "Beans App", wallets[0].Name)
		// assert.Equal(t, "https://www.beansapp.com/disbursements", wallets[0].Homepage)
		// assert.Equal(t, "api.beansapp.com", wallets[0].SEP10ClientDomain)
		// assert.Equal(t, "https://www.beansapp.com/disbursements/registration?redirect=true", wallets[0].DeepLinkSchema)
		assert.Equal(t, "Vibrant Assist", wallets[0].Name)
		assert.Equal(t, "https://vibrantapp.com/vibrant-assist", wallets[0].Homepage)
		assert.Equal(t, "api.vibrantapp.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp", wallets[0].DeepLinkSchema)

		assert.Equal(t, "Vibrant Assist RC", wallets[1].Name)
		assert.Equal(t, "vibrantapp.com/vibrant-assist", wallets[1].Homepage)
		assert.Equal(t, "vibrantapp.com", wallets[1].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-rc", wallets[1].DeepLinkSchema)

		expectedLogs = []string{
			fmt.Sprintf("running for tenant ID %s", tnt2.ID),
			"updating/inserting assets for the 'pubnet' network",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", services.DefaultAssetsNetworkMap[utils.PubnetNetworkType]["USDC"]),
			"Code: XLM",
			"Issuer: ",
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://vibrantapp.com/sdp",
			"SEP-10 Client Domain: api.vibrantapp.com",
		}

		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})

	t.Run("run for a specific tenant --tenant-id", func(t *testing.T) {
		clearTenantTables(t, ctx, tenant1SchemaConnectionPool)
		clearTenantTables(t, ctx, tenant2SchemaConnectionPool)

		// Setup
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{
			"db",
			"setup-for-network",
			"--database-url",
			dbt.DSN,
			"--network-passphrase",
			network.PublicNetworkPassphrase,
			"--tenant-id",
			tnt2.ID,
		})

		err = rootCmd.Execute()
		require.NoError(t, err)

		// Tenant 1
		models, err := data.NewModels(tenant1SchemaConnectionPool)
		require.NoError(t, err)

		// Validating assets
		assets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, assets, 1)
		assert.Equal(t, "USDC", assets[0].Code)
		assert.Equal(t, testnetUSDCIssuer, assets[0].Issuer)

		// Validating wallets
		wallets, err := models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, wallets, 1)
		assert.Equal(t, "Vibrant Assist", wallets[0].Name)
		assert.Equal(t, "https://vibrantapp.com", wallets[0].Homepage)
		assert.Equal(t, "api-dev.vibrantapp.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-dev", wallets[0].DeepLinkSchema)

		logs := buf.String()
		assert.NotContains(t, logs, fmt.Sprintf("running on tenant ID %s", tnt1.ID))

		// Tenant 2
		models, err = data.NewModels(tenant2SchemaConnectionPool)
		require.NoError(t, err)

		// Validating assets
		assets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, assets, 2)
		assert.Equal(t, "USDC", assets[0].Code)
		assert.NotEqual(t, testnetUSDCIssuer, assets[0].Issuer)
		assert.Equal(t, services.DefaultAssetsNetworkMap[utils.PubnetNetworkType]["USDC"], assets[0].Issuer)
		assert.Equal(t, "XLM", assets[1].Code)
		assert.Empty(t, assets[1].Issuer)

		// Validating wallets
		wallets, err = models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, wallets, 2)
		// assert.Equal(t, "Beans App", wallets[0].Name)
		// assert.Equal(t, "https://www.beansapp.com/disbursements", wallets[0].Homepage)
		// assert.Equal(t, "api.beansapp.com", wallets[0].SEP10ClientDomain)
		// assert.Equal(t, "https://www.beansapp.com/disbursements/registration?redirect=true", wallets[0].DeepLinkSchema)
		assert.Equal(t, "Vibrant Assist", wallets[0].Name)
		assert.Equal(t, "https://vibrantapp.com/vibrant-assist", wallets[0].Homepage)
		assert.Equal(t, "api.vibrantapp.com", wallets[0].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp", wallets[0].DeepLinkSchema)

		assert.Equal(t, "Vibrant Assist RC", wallets[1].Name)
		assert.Equal(t, "vibrantapp.com/vibrant-assist", wallets[1].Homepage)
		assert.Equal(t, "vibrantapp.com", wallets[1].SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-rc", wallets[1].DeepLinkSchema)

		expectedLogs := []string{
			fmt.Sprintf("running for tenant ID %s", tnt2.ID),
			"updating/inserting assets for the 'pubnet' network",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", services.DefaultAssetsNetworkMap[utils.PubnetNetworkType]["USDC"]),
			"Code: XLM",
			"Issuer: ",
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://vibrantapp.com/sdp",
			"SEP-10 Client Domain: api.vibrantapp.com",
		}

		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})
}
