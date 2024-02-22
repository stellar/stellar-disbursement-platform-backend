package provisioning

import (
	"context"
	"fmt"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"net/url"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_Manager_ProvisionNewTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	messengerClientMock := message.MessengerClientMock{}
	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	messengerClientMock.
		On("SendMessage", mock.AnythingOfType("message.Message")).
		Return(nil)

	distAcc := keypair.MustRandom()

	t.Run("provision a new tenant for the testnet", func(t *testing.T) {
		tenantName1 := "myorg-ukraine"
		tenantName2 := "myorg-poland"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "My Org"
		uiBaseURL := "http://localhost:3000"

		schemaName1 := fmt.Sprintf("sdp_%s", tenantName1)
		schemaName2 := fmt.Sprintf("sdp_%s", tenantName2)

		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"countries",
			"disbursements",
			"sdp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"wallets",
			"wallets_assets",
		}

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_ENV", func(t *testing.T) {
			distAccSigClient, err := signing.NewSignatureClient(signing.DistributionAccountEnvSignatureClientType, signing.SignatureClientOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: distAcc.Seed(),
			})
			require.NoError(t, err)

			getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
			p := NewManager(
				WithDatabase(dbConnectionPool),
				WithMessengerClient(&messengerClientMock),
				WithTenantManager(tenantManager),
				WithDistributionAccountSignatureClient(distAccSigClient),
			)

			tnt, err := p.ProvisionNewTenant(ctx, tenantName1, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.TestnetNetworkType))
			require.NoError(t, err)

			entries := getEntries()
			require.Len(t, entries, 2)
			assert.Contains(t, entries[0].Message, "Account provisioning not needed for distribution account signature client type")

			assert.Equal(t, tenantName1, tnt.Name)
			assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
			assert.Equal(t, distAcc.Address(), *tnt.DistributionAccount)
			assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)

			// Connecting to the new schema
			dsn, err := dbConnectionPool.DSN(ctx)
			require.NoError(t, err)
			u, err := url.Parse(dsn)
			require.NoError(t, err)

			uq := u.Query()
			uq.Set("search_path", schemaName1)
			u.RawQuery = uq.Encode()

			assert.True(t, tenant.CheckSchemaExistsFixture(t, ctx, dbConnectionPool, schemaName1))

			tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
			require.NoError(t, err)
			defer tenantSchemaConnectionPool.Close()

			tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName1, expectedTablesAfterMigrationsApplied)

			tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
			tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
			tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
		})

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_DB", func(t *testing.T) {
			distAccSigClient, err := signing.NewSignatureClient(signing.DistributionAccountDBSignatureClientType, signing.SignatureClientOptions{
				NetworkPassphrase:           network.TestNetworkPassphrase,
				DistAccEncryptionPassphrase: keypair.MustRandom().Seed(),
				DBConnectionPool:            dbConnectionPool,
			})
			require.NoError(t, err)

			p := NewManager(
				WithDatabase(dbConnectionPool),
				WithMessengerClient(&messengerClientMock),
				WithTenantManager(tenantManager),
				WithDistributionAccountSignatureClient(distAccSigClient),
			)

			tnt, err := p.ProvisionNewTenant(ctx, tenantName2, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.TestnetNetworkType))
			require.NoError(t, err)

			assert.Equal(t, tenantName2, tnt.Name)
			assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
			assert.NotNil(t, *tnt.DistributionAccount)
			assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)

			// Connecting to the new schema
			dsn, err := dbConnectionPool.DSN(ctx)
			require.NoError(t, err)
			u, err := url.Parse(dsn)
			require.NoError(t, err)

			uq := u.Query()
			uq.Set("search_path", schemaName2)
			u.RawQuery = uq.Encode()

			assert.True(t, tenant.CheckSchemaExistsFixture(t, ctx, dbConnectionPool, schemaName2))

			tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
			require.NoError(t, err)
			defer tenantSchemaConnectionPool.Close()

			tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName2, expectedTablesAfterMigrationsApplied)

			tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
			tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
			tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
		})
	})

	t.Run("provision a new tenant for the pubnet", func(t *testing.T) {
		tenantName1 := "myorg-us"
		tenantName2 := "myorg-canada"

		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "My Org"
		uiBaseURL := "http://localhost:3000"

		schemaName1 := fmt.Sprintf("sdp_%s", tenantName1)
		schemaName2 := fmt.Sprintf("sdp_%s", tenantName2)

		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"countries",
			"disbursements",
			"sdp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"wallets",
			"wallets_assets",
		}

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_ENV", func(t *testing.T) {
			distAccSigClient, err := signing.NewSignatureClient(signing.DistributionAccountEnvSignatureClientType, signing.SignatureClientOptions{
				NetworkPassphrase:      network.PublicNetworkPassphrase,
				DistributionPrivateKey: distAcc.Seed(),
			})
			require.NoError(t, err)

			getEntries := log.DefaultLogger.StartTest(log.WarnLevel)
			p := NewManager(
				WithDatabase(dbConnectionPool),
				WithMessengerClient(&messengerClientMock),
				WithTenantManager(tenantManager),
				WithDistributionAccountSignatureClient(distAccSigClient),
			)

			tnt, err := p.ProvisionNewTenant(ctx, tenantName1, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.PubnetNetworkType))
			require.NoError(t, err)

			entries := getEntries()
			require.Len(t, entries, 2)
			assert.Contains(t, entries[0].Message, "Account provisioning not needed for distribution account signature client type")

			assert.Equal(t, tenantName1, tnt.Name)
			assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
			assert.Equal(t, distAcc.Address(), *tnt.DistributionAccount)
			assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)

			// Connecting to the new schema
			dsn, err := dbConnectionPool.DSN(ctx)
			require.NoError(t, err)
			u, err := url.Parse(dsn)
			require.NoError(t, err)
			uq := u.Query()
			uq.Set("search_path", schemaName1)
			u.RawQuery = uq.Encode()

			tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
			require.NoError(t, err)
			defer tenantSchemaConnectionPool.Close()

			tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName1, expectedTablesAfterMigrationsApplied)

			tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"})
			tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Freedom Wallet", "Vibrant Assist RC", "Vibrant Assist"})
			tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
		})

		t.Run("provision key using type DISTRIBUTION_ACCOUNT_DB", func(t *testing.T) {
			distAccSigClient, err := signing.NewSignatureClient(signing.DistributionAccountDBSignatureClientType, signing.SignatureClientOptions{
				NetworkPassphrase:           network.PublicNetworkPassphrase,
				DistAccEncryptionPassphrase: keypair.MustRandom().Seed(),
				DBConnectionPool:            dbConnectionPool,
			})
			require.NoError(t, err)

			p := NewManager(
				WithDatabase(dbConnectionPool),
				WithMessengerClient(&messengerClientMock),
				WithTenantManager(tenantManager),
				WithDistributionAccountSignatureClient(distAccSigClient),
			)

			tnt, err := p.ProvisionNewTenant(ctx, tenantName2, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.PubnetNetworkType))
			require.NoError(t, err)

			assert.Equal(t, tenantName2, tnt.Name)
			assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
			assert.NotNil(t, *tnt.DistributionAccount)
			assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)

			// Connecting to the new schema
			dsn, err := dbConnectionPool.DSN(ctx)
			require.NoError(t, err)
			u, err := url.Parse(dsn)
			require.NoError(t, err)

			uq := u.Query()
			uq.Set("search_path", schemaName2)
			u.RawQuery = uq.Encode()

			assert.True(t, tenant.CheckSchemaExistsFixture(t, ctx, dbConnectionPool, schemaName2))

			tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
			require.NoError(t, err)
			defer tenantSchemaConnectionPool.Close()

			tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName2, expectedTablesAfterMigrationsApplied)

			tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"})
			tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Freedom Wallet", "Vibrant Assist RC", "Vibrant Assist"})
			tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
		})
	})

	messengerClientMock.AssertExpectations(t)
}

func Test_Manager_RunMigrationsForTenant(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	tnt1, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)
	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)

	tnt1SchemaName := fmt.Sprintf("sdp_%s", tnt1.Name)
	tnt2SchemaName := fmt.Sprintf("sdp_%s", tnt2.Name)

	// Creating DB Schemas
	_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", tnt1SchemaName))
	require.NoError(t, err)
	_, err = dbConnectionPool.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", tnt2SchemaName))
	require.NoError(t, err)

	dsn, err := dbConnectionPool.DSN(ctx)
	require.NoError(t, err)
	u, err := url.Parse(dsn)
	require.NoError(t, err)

	// Tenant 1 DB connection
	tnt1Q := u.Query()
	tnt1Q.Set("search_path", tnt1SchemaName)
	u.RawQuery = tnt1Q.Encode()
	tnt1SchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
	require.NoError(t, err)
	defer tnt1SchemaConnectionPool.Close()

	// Tenant 2 DB connection
	tnt2Q := u.Query()
	tnt2Q.Set("search_path", tnt2SchemaName)
	u.RawQuery = tnt2Q.Encode()
	tnt2SchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
	require.NoError(t, err)
	defer tnt2SchemaConnectionPool.Close()

	// Apply migrations for Tenant 1
	tnt1DSN, err := tnt1SchemaConnectionPool.DSN(ctx)
	require.NoError(t, err)

	p := NewManager(WithDatabase(dbConnectionPool))
	err = p.RunMigrationsForTenant(ctx, tnt1, tnt1DSN, migrate.Up, 0, sdpmigrations.FS, db.StellarPerTenantSDPMigrationsTableName)
	require.NoError(t, err)
	err = p.RunMigrationsForTenant(ctx, tnt1, tnt1DSN, migrate.Up, 0, authmigrations.FS, db.StellarPerTenantAuthMigrationsTableName)
	require.NoError(t, err)

	expectedTablesAfterMigrationsApplied := []string{
		"assets",
		"auth_migrations",
		"auth_user_mfa_codes",
		"auth_user_password_reset",
		"auth_users",
		"countries",
		"disbursements",
		"sdp_migrations",
		"messages",
		"organizations",
		"payments",
		"receiver_verifications",
		"receiver_wallets",
		"receivers",
		"wallets",
		"wallets_assets",
	}
	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt1SchemaName, expectedTablesAfterMigrationsApplied)

	// Asserting if the Tenant 2 DB Schema wasn't affected by Tenant 1 schema migrations
	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt2SchemaName, []string{})

	// Apply migrations for Tenant 2
	tnt2DSN, err := tnt2SchemaConnectionPool.DSN(ctx)
	require.NoError(t, err)
	err = p.RunMigrationsForTenant(ctx, tnt2, tnt2DSN, migrate.Up, 0, sdpmigrations.FS, db.StellarPerTenantSDPMigrationsTableName)
	require.NoError(t, err)
	err = p.RunMigrationsForTenant(ctx, tnt2, tnt2DSN, migrate.Up, 0, authmigrations.FS, db.StellarPerTenantAuthMigrationsTableName)
	require.NoError(t, err)

	tenant.TenantSchemaMatchTablesFixture(t, ctx, dbConnectionPool, tnt2SchemaName, expectedTablesAfterMigrationsApplied)
}
