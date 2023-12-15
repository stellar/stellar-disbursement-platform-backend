package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getMigrationsApplied(t *testing.T, ctx context.Context, db db.DBConnectionPool) []string {
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

func Test_DatabaseCommand_db_migrate(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	buf := new(strings.Builder)

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
	log.DefaultLogger.SetOutput(buf)
	rootCmd = SetupCLI("x.y.z", "1234567890abcdef")
	rootCmd.SetArgs([]string{"db", "migrate", "up", "1", "--database-url", dbt.DSN, "--log-level", "TRACE"})
	err = rootCmd.Execute()
	require.NoError(t, err)

	ids := getMigrationsApplied(t, context.Background(), dbConnectionPool)
	assert.Equal(t, []string{"2023-01-20.0-initial.sql"}, ids)

	assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")

	buf.Reset()
	rootCmd = SetupCLI("x.y.z", "1234567890abcdef")
	rootCmd.SetArgs([]string{"db", "migrate", "down", "1", "--database-url", dbt.DSN, "--log-level", "TRACE"})
	err = rootCmd.Execute()
	require.NoError(t, err)

	ids = getMigrationsApplied(t, context.Background(), dbConnectionPool)
	assert.Equal(t, []string{}, ids)

	assert.Contains(t, buf.String(), "Successfully applied 1 migrations.")
}

func Test_DatabaseCommand_db_setup_for_network(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	// Assets
	testnetUSDCIssuer := keypair.MustRandom().Address()
	data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", testnetUSDCIssuer)

	actualAssets, err := models.Assets.GetAll(ctx)
	require.NoError(t, err)

	assert.Len(t, actualAssets, 1)
	assert.Equal(t, "USDC", actualAssets[0].Code)
	assert.Equal(t, testnetUSDCIssuer, actualAssets[0].Issuer)

	// Wallets
	data.CreateWalletFixture(t, ctx, dbConnectionPool, "Vibrant Assist", "https://vibrantapp.com", "api-dev.vibrantapp.com", "https://vibrantapp.com/sdp-dev")

	wallets, err := models.Wallets.GetAll(ctx)
	require.NoError(t, err)

	assert.Len(t, wallets, 1)
	assert.Equal(t, "Vibrant Assist", wallets[0].Name)
	assert.Equal(t, "https://vibrantapp.com", wallets[0].Homepage)
	assert.Equal(t, "api-dev.vibrantapp.com", wallets[0].SEP10ClientDomain)
	assert.Equal(t, "https://vibrantapp.com/sdp-dev", wallets[0].DeepLinkSchema)

	buf := new(strings.Builder)
	log.DefaultLogger.SetLevel(log.InfoLevel)
	log.DefaultLogger.SetOutput(buf)

	// Setup
	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	rootCmd.SetArgs([]string{
		"db",
		"setup-for-network",
		"--database-url",
		dbt.DSN,
		"--network-passphrase",
		network.PublicNetworkPassphrase,
	})

	err = rootCmd.Execute()
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
	var vibrantAssist, vibrantAssistRC data.Wallet

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

	expectedLogs := []string{
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

	logs := buf.String()
	for _, expectedLog := range expectedLogs {
		assert.Contains(t, logs, expectedLog)
	}
}
