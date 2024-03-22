package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func getSDPMigrationsApplied(t *testing.T, ctx context.Context, db db.DBConnectionPool) []string {
	t.Helper()

	rows, err := db.QueryContext(ctx, "SELECT id FROM sdp_migrations")
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
		"admin             Admin migrations used to configure the multi-tenant module that manages the tenants.",
		"auth              Authentication's per-tenant schema migration helpers. Will execute the migrations of the `auth-migrations` folder on the desired tenant, according with the --all or --tenant-id configs. The migrations are tracked in the table `auth_migrations`.",
		"sdp               Stellar Disbursement Platform's per-tenant schema migration helpers.",
		"setup-for-network Set up the assets and wallets registered in the database based on the network passphrase.",
		"-h, --help   help for db",
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

func Test_DatabaseCommand_db_sdp_migrate(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	buf := new(strings.Builder)

	t.Run("migrate usage", func(t *testing.T) {
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs([]string{
			"db", "sdp", "migrate",
			"--database-url",
			dbt.DSN,
		})
		rootCmd.SetOut(buf)
		err = rootCmd.Execute()
		require.NoError(t, err)

		expectedContains := []string{
			"Schema migration helpers",
			"stellar-disbursement-platform db sdp migrate [flags]",
			"stellar-disbursement-platform db sdp migrate [command]",
			"down        Migrates database down [count] migrations",
			"up          Migrates database up [count] migrations",
			"-h, --help   help for migrate",
			`--all                         Apply the command to all tenants. Either --tenant-id or --all must be set, but the --all option will be ignored if --tenant-id is set.`,
			`--base-url string             The SDP backend server's base URL. (BASE_URL) (default "http://localhost:8000")`,
			`--database-url string         Postgres DB URL (DATABASE_URL) (default "postgres://localhost:5432/sdp?sslmode=disable")`,
			`--environment string          The environment where the application is running. Example: "development", "staging", "production". (ENVIRONMENT) (default "development")`,
			`--log-level string            The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")`,
			`--network-passphrase string   The Stellar network passphrase (NETWORK_PASSPHRASE) (default "Test SDF Network ; September 2015")`,
			`--sentry-dsn string           The DSN (client key) of the Sentry project. If not provided, Sentry will not be used. (SENTRY_DSN)`,
			`--tenant-id string            The tenant ID where the command will be applied. Either --tenant-id or --all must be set, but the --all option will be ignored if --tenant-id is set. (TENANT_ID)`,
		}

		output := buf.String()
		for _, expected := range expectedContains {
			assert.Contains(t, output, expected)
		}
	})

	t.Run("db sdp migrate up and down --all", func(t *testing.T) {
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
		rootCmd.SetArgs([]string{"db", "sdp", "migrate", "up", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--all"})
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
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations up.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"sdp_migrations"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getSDPMigrationsApplied(t, ctx, tenant2SchemaConnectionPool)
		assert.Equal(t, []string{"2023-01-20.0-initial.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations up.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"sdp_migrations"})

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "sdp", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--all"})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1
		ids = getSDPMigrationsApplied(t, context.Background(), tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations down.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"sdp_migrations"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getSDPMigrationsApplied(t, context.Background(), tenant2SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations down.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"sdp_migrations"})
	})

	t.Run("db sdp migrate up and down --tenant-id", func(t *testing.T) {
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
		rootCmd.SetArgs([]string{"db", "sdp", "migrate", "up", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--tenant-id", tnt1.ID})
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
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations up.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"sdp_migrations"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt2.ID))

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "sdp", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--tenant-id", tnt1.ID})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1 schema
		ids = getSDPMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations down.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"sdp_migrations"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt2.ID))
	})

	t.Run("db sdp migrate up and down auth migrations --all", func(t *testing.T) {
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
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations up.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations", "auth_users"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getAuthMigrationsApplied(t, ctx, tenant2SchemaConnectionPool)
		assert.Equal(t, []string{"2023-02-09.0.add-users-table.sql"}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations up.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"auth_migrations", "auth_users"})

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "auth", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--all"})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1
		ids = getAuthMigrationsApplied(t, context.Background(), tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations down.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations"})

		// Checking if the migrations were applied on the Tenant 2
		ids = getAuthMigrationsApplied(t, context.Background(), tenant2SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations down.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{"auth_migrations"})
	})

	t.Run("db sdp migrate up and down auth migrations --tenant-id", func(t *testing.T) {
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
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations up.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations", "auth_users"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt2.ID))

		buf.Reset()
		rootCmd.SetArgs([]string{"db", "auth", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE", "--tenant-id", tnt1.ID})
		err = rootCmd.Execute()
		require.NoError(t, err)

		// Checking if the migrations were applied on the Tenant 1 schema
		ids = getAuthMigrationsApplied(t, ctx, tenant1SchemaConnectionPool)
		assert.Equal(t, []string{}, ids)
		assert.Contains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt1.ID))
		assert.Contains(t, buf.String(), "Successfully applied 1 migrations down.")
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg1", []string{"auth_migrations"})

		// Checking if the migrations were not applied on the Tenant 2 schema
		tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, "sdp_myorg2", []string{})
		assert.NotContains(t, buf.String(), fmt.Sprintf("Applying migrations on tenant ID %s", tnt2.ID))
	})
}

func Test_DatabaseCommand_db_setup_for_network(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	// Creating Tenants
	tnt1, outerErr := m.AddTenant(ctx, "myorg1")
	require.NoError(t, outerErr)

	tnt2, outerErr := m.AddTenant(ctx, "myorg2")
	require.NoError(t, outerErr)

	_, outerErr = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg1")
	require.NoError(t, outerErr)
	_, outerErr = dbConnectionPool.ExecContext(ctx, "CREATE SCHEMA sdp_myorg2")
	require.NoError(t, outerErr)

	tenant.ApplyMigrationsForTenantFixture(t, ctx, dbConnectionPool, tnt1.Name)
	tenant.ApplyMigrationsForTenantFixture(t, ctx, dbConnectionPool, tnt2.Name)

	tnt1DSN, outerErr := m.GetDSNForTenant(ctx, tnt1.Name)
	require.NoError(t, outerErr)

	tnt2DSN, outerErr := m.GetDSNForTenant(ctx, tnt2.Name)
	require.NoError(t, outerErr)

	tenant1SchemaConnectionPool, outerErr := db.OpenDBConnectionPool(tnt1DSN)
	require.NoError(t, outerErr)
	defer tenant1SchemaConnectionPool.Close()

	tenant2SchemaConnectionPool, outerErr := db.OpenDBConnectionPool(tnt2DSN)
	require.NoError(t, outerErr)
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

		err := rootCmd.Execute()
		require.NoError(t, err)

		// Tenant 1
		models, mErr := data.NewModels(tenant1SchemaConnectionPool)
		require.NoError(t, mErr)

		// Validating assets
		actualAssets, aErr := models.Assets.GetAll(ctx)
		require.NoError(t, aErr)

		assert.Len(t, actualAssets, 2)
		assert.Equal(t, "USDC", actualAssets[0].Code)
		assert.NotEqual(t, testnetUSDCIssuer, actualAssets[0].Issuer)
		assert.Equal(t, assets.USDCAssetIssuerPubnet, actualAssets[0].Issuer)
		assert.Equal(t, "XLM", actualAssets[1].Code)
		assert.Empty(t, actualAssets[1].Issuer)

		// Validating wallets
		wallets, wErr := models.Wallets.GetAll(ctx)
		require.NoError(t, wErr)

		// Test only on Vibrant Assist and Vibrant Assist RC. This will help adding wallets without breaking tests.
		var vibrantAssist, vibrantAssistRC data.Wallet

		for _, w := range wallets {
			if w.Name == "Vibrant Assist" {
				vibrantAssist = w
			} else if w.Name == "Vibrant Assist RC" {
				vibrantAssistRC = w
			}
		}
		assert.Equal(t, "Vibrant Assist", vibrantAssist.Name)
		assert.Equal(t, "https://vibrantapp.com/vibrant-assist", vibrantAssist.Homepage)
		assert.Equal(t, "vibrantapp.com", vibrantAssist.SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp", vibrantAssist.DeepLinkSchema)

		assert.Equal(t, "Vibrant Assist RC", vibrantAssistRC.Name)
		assert.Equal(t, "vibrantapp.com/vibrant-assist", vibrantAssistRC.Homepage)
		assert.Equal(t, "vibrantapp.com", vibrantAssistRC.SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-rc", vibrantAssistRC.DeepLinkSchema)

		expectedLogs := []string{
			fmt.Sprintf("running for tenant ID %s", tnt1.ID),
			"updating/inserting assets for the 'pubnet' network",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", assets.USDCAssetIssuerPubnet),
			"Code: XLM",
			"Issuer: ",
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://vibrantapp.com/sdp",
			"SEP-10 Client Domain: vibrantapp.com",
		}

		logs := buf.String()
		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}

		// Tenant 2
		models, err = data.NewModels(tenant2SchemaConnectionPool)
		require.NoError(t, err)

		// Validating assets
		actualAssets, err = models.Assets.GetAll(ctx)
		require.NoError(t, err)

		require.Len(t, actualAssets, 2)
		require.Equal(t, assets.USDCAssetPubnet.Code, actualAssets[0].Code)
		require.Equal(t, assets.USDCAssetPubnet.Issuer, actualAssets[0].Issuer)
		require.Equal(t, assets.XLMAsset.Code, actualAssets[1].Code)
		require.Empty(t, assets.XLMAsset.Issuer)

		// Validating wallets
		wallets, err = models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		// Test only on Vibrant Assist and Vibrant Assist RC. This will help adding wallets without breaking tests.
		for _, w := range wallets {
			if w.Name == "Vibrant Assist" {
				vibrantAssist = w
			} else if w.Name == "Vibrant Assist RC" {
				vibrantAssistRC = w
			}
		}

		require.NotNil(t, vibrantAssist, "Vibrant Assist wallet not found")
		require.NotNil(t, vibrantAssistRC, "Vibrant Assist RC wallet not found")

		// Test the two wallets
		assert.Equal(t, "Vibrant Assist", vibrantAssist.Name)
		assert.Equal(t, "https://vibrantapp.com/vibrant-assist", vibrantAssist.Homepage)
		assert.Equal(t, "vibrantapp.com", vibrantAssist.SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp", vibrantAssist.DeepLinkSchema)

		assert.Equal(t, "Vibrant Assist RC", vibrantAssistRC.Name)
		assert.Equal(t, "vibrantapp.com/vibrant-assist", vibrantAssistRC.Homepage)
		assert.Equal(t, "vibrantapp.com", vibrantAssistRC.SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-rc", vibrantAssistRC.DeepLinkSchema)

		expectedLogs = []string{
			fmt.Sprintf("running for tenant ID %s", tnt2.ID),
			"updating/inserting assets for the 'pubnet' network",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", assets.USDCAssetIssuerPubnet),
			"Code: XLM",
			"Issuer: ",
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://vibrantapp.com/sdp",
			"SEP-10 Client Domain: vibrantapp.com",
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

		err := rootCmd.Execute()
		require.NoError(t, err)

		// Tenant 1
		models, err := data.NewModels(tenant1SchemaConnectionPool)
		require.NoError(t, err)

		// Validating assets
		currentAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, currentAssets, 1)
		assert.Equal(t, "USDC", currentAssets[0].Code)
		assert.Equal(t, testnetUSDCIssuer, currentAssets[0].Issuer)

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
		actualAssets, err := models.Assets.GetAll(ctx)
		require.NoError(t, err)

		assert.Len(t, actualAssets, 2)
		assert.Equal(t, "USDC", actualAssets[0].Code)
		assert.NotEqual(t, testnetUSDCIssuer, actualAssets[0].Issuer)
		assert.Equal(t, assets.USDCAssetIssuerPubnet, actualAssets[0].Issuer)
		assert.Equal(t, "XLM", actualAssets[1].Code)
		assert.Empty(t, actualAssets[1].Issuer)

		// Validating wallets
		wallets, err = models.Wallets.GetAll(ctx)
		require.NoError(t, err)

		// Test only on Vibrant Assist and Vibrant Assist RC. This will help adding wallets without breaking tests.
		var vibrantAssist, vibrantAssistRC data.Wallet
		for _, w := range wallets {
			if w.Name == "Vibrant Assist" {
				vibrantAssist = w
			} else if w.Name == "Vibrant Assist RC" {
				vibrantAssistRC = w
			}
		}
		assert.Equal(t, "Vibrant Assist", vibrantAssist.Name)
		assert.Equal(t, "https://vibrantapp.com/vibrant-assist", vibrantAssist.Homepage)
		assert.Equal(t, "vibrantapp.com", vibrantAssist.SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp", vibrantAssist.DeepLinkSchema)

		assert.Equal(t, "Vibrant Assist RC", vibrantAssistRC.Name)
		assert.Equal(t, "vibrantapp.com/vibrant-assist", vibrantAssistRC.Homepage)
		assert.Equal(t, "vibrantapp.com", vibrantAssistRC.SEP10ClientDomain)
		assert.Equal(t, "https://vibrantapp.com/sdp-rc", vibrantAssistRC.DeepLinkSchema)

		expectedLogs := []string{
			fmt.Sprintf("running for tenant ID %s", tnt2.ID),
			"updating/inserting assets for the 'pubnet' network",
			"Code: USDC",
			fmt.Sprintf("Issuer: %s", assets.USDCAssetPubnet.Issuer),
			"Code: XLM",
			"Issuer: ",
			"updating/inserting wallets for the 'pubnet' network",
			"Name: Vibrant Assist",
			"Homepage: https://vibrantapp.com/vibrant-assist",
			"Deep Link Schema: https://vibrantapp.com/sdp",
			"SEP-10 Client Domain: vibrantapp.com",
		}

		for _, expectedLog := range expectedLogs {
			assert.Contains(t, logs, expectedLog)
		}
	})
}
