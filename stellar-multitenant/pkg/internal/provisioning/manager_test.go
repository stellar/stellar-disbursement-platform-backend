package provisioning

import (
	"context"
	"fmt"
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
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_Manager_ProvisionNewTenant(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	messengerClientMock := message.MessengerClientMock{}
	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	distAccSigClientMock := sigMocks.NewMockSignatureClient(t)
	p := NewManager(
		WithDatabase(dbConnectionPool),
		WithMessengerClient(&messengerClientMock),
		WithTenantManager(tenantManager),
		WithDistributionAccountSignatureClient(distAccSigClientMock),
	)

	messengerClientMock.
		On("SendMessage", mock.AnythingOfType("message.Message")).
		Return(nil)

	distAcc := keypair.MustRandom()

	t.Run("provision a new tenant for the testnet", func(t *testing.T) {
		tenantName := "myorg-ukraine"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "My Org"
		uiBaseURL := "http://localhost:3000"

		distAccSigClientMock.
			On("BatchInsert", ctx, 1).Return([]string{distAcc.Address()}, nil).
			Once()

		tnt, err := p.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.TestnetNetworkType))
		require.NoError(t, err)

		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		assert.Equal(t, tenantName, tnt.Name)
		assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
		assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)
		assert.True(t, tenant.CheckSchemaExistsFixture(t, ctx, dbConnectionPool, schemaName))

		// Connecting to the new schema
		dsn, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		uq := u.Query()
		uq.Set("search_path", schemaName)
		u.RawQuery = uq.Encode()

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

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
		tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)

		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})

	t.Run("provision a new tenant for the pubnet", func(t *testing.T) {
		tenantName := "myorg-us"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "My Org"
		uiBaseURL := "http://localhost:3000"

		distAccSigClientMock.
			On("BatchInsert", ctx, 1).Return([]string{distAcc.Address()}, nil).
			Once()

		tnt, err := p.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.PubnetNetworkType))
		require.NoError(t, err)

		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		assert.Equal(t, tenantName, tnt.Name)
		assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
		assert.Equal(t, tenant.ProvisionedTenantStatus, tnt.Status)
		assert.True(t, tenant.CheckSchemaExistsFixture(t, ctx, dbConnectionPool, schemaName))

		// Connecting to the new schema
		dsn, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		uq := u.Query()
		uq.Set("search_path", schemaName)
		u.RawQuery = uq.Encode()

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

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
		tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)

		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Freedom Wallet", "Vibrant Assist RC", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})

	messengerClientMock.AssertExpectations(t)
	distAccSigClientMock.AssertExpectations(t)
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
