package tenant

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_Manager_ProvisionNewTenant(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	messengerClientMock := message.MessengerClientMock{}
	m := NewManager(WithDatabase(dbConnectionPool), WithMessengerClient(&messengerClientMock))

	messengerClientMock.
		On("SendMessage", mock.AnythingOfType("message.Message")).
		Return(nil)

	t.Run("provision a new tenant for the testnet", func(t *testing.T) {
		tenantName := "myorg-ukraine"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "My Org"
		uiBaseURL := "http://localhost:3000"

		tnt, err := m.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.TestnetNetworkType))
		require.NoError(t, err)

		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		assert.Equal(t, tenantName, tnt.Name)
		assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
		assert.Equal(t, ProvisionedTenantStatus, tnt.Status)
		assert.True(t, CheckSchemaExistsFixture(t, ctx, dbConnectionPool, schemaName))

		// Connecting to the new schema
		u, err := url.Parse(dbConnectionPool.DSN())
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
			"channel_accounts",
			"countries",
			"disbursements",
			"gorp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"submitter_transactions",
			"wallets",
			"wallets_assets",
		}
		TenantSchemaHasTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)

		AssertRegisteredAssets(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
		AssertRegisteredWallets(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
		AssertRegisteredUser(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})

	t.Run("provision a new tenant for the pubnet", func(t *testing.T) {
		tenantName := "myorg-us"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "My Org"
		uiBaseURL := "http://localhost:3000"

		tnt, err := m.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, organizationName, uiBaseURL, string(utils.PubnetNetworkType))
		require.NoError(t, err)

		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		assert.Equal(t, tenantName, tnt.Name)
		assert.Equal(t, uiBaseURL, *tnt.SDPUIBaseURL)
		assert.Equal(t, ProvisionedTenantStatus, tnt.Status)
		assert.True(t, CheckSchemaExistsFixture(t, ctx, dbConnectionPool, schemaName))

		// Connecting to the new schema
		u, err := url.Parse(dbConnectionPool.DSN())
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
			"channel_accounts",
			"countries",
			"disbursements",
			"gorp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"submitter_transactions",
			"wallets",
			"wallets_assets",
		}
		TenantSchemaHasTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)

		AssertRegisteredAssets(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"})
		AssertRegisteredWallets(t, ctx, tenantSchemaConnectionPool, []string{"Vibrant Assist RC", "Vibrant Assist"})
		AssertRegisteredUser(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})

	messengerClientMock.AssertExpectations(t)
}

func Test_Manager_AddTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))

	t.Run("returns error when tenant name is empty", func(t *testing.T) {
		tnt, err := m.AddTenant(ctx, "")
		assert.Equal(t, ErrEmptyTenantName, err)
		assert.Nil(t, tnt)
	})

	t.Run("inserts a new tenant successfully", func(t *testing.T) {
		tnt, err := m.AddTenant(ctx, "myorg-ukraine")
		require.NoError(t, err)
		assert.NotNil(t, tnt)
		assert.NotEmpty(t, tnt.ID)
		assert.Equal(t, "myorg-ukraine", tnt.Name)
		assert.Equal(t, CreatedTenantStatus, tnt.Status)
	})

	t.Run("returns error when tenant name is duplicated", func(t *testing.T) {
		tnt, err := m.AddTenant(ctx, "myorg")
		require.NoError(t, err)
		assert.NotNil(t, tnt)
		assert.NotEmpty(t, tnt.ID)
		assert.Equal(t, "myorg", tnt.Name)
		assert.Equal(t, CreatedTenantStatus, tnt.Status)

		tnt, err = m.AddTenant(ctx, "MyOrg")
		assert.Equal(t, ErrDuplicatedTenantName, err)
		assert.Nil(t, tnt)
	})
}

func Test_Manager_UpdateTenantConfig(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	tntDB, err := m.AddTenant(ctx, "myorg")
	require.NoError(t, err)

	t.Run("returns error when tenant update is nil", func(t *testing.T) {
		tnt, err := m.UpdateTenantConfig(ctx, nil)
		assert.EqualError(t, err, "tenant update cannot be nil")
		assert.Nil(t, tnt)
	})

	t.Run("returns error when no field has changed", func(t *testing.T) {
		tnt, err := m.UpdateTenantConfig(ctx, &TenantUpdate{ID: tntDB.ID})
		assert.EqualError(t, err, "provide at least one field to be updated")
		assert.Nil(t, tnt)
	})

	t.Run("returns error when the tenant ID does not exist", func(t *testing.T) {
		tnt, err := m.UpdateTenantConfig(ctx, &TenantUpdate{ID: "abc", EmailSenderType: &AWSEmailSenderType})
		assert.Equal(t, ErrTenantDoesNotExist, err)
		assert.Nil(t, tnt)
	})

	t.Run("updates tenant config successfully", func(t *testing.T) {
		tntDB = ResetTenantConfigFixture(t, ctx, dbConnectionPool, tntDB.ID)
		assert.Equal(t, tntDB.EmailSenderType, DryRunEmailSenderType)
		assert.Equal(t, tntDB.SMSSenderType, DryRunSMSSenderType)
		assert.Nil(t, tntDB.SEP10SigningPublicKey)
		assert.Nil(t, tntDB.DistributionPublicKey)
		assert.True(t, tntDB.EnableMFA)
		assert.True(t, tntDB.EnableReCAPTCHA)
		assert.Nil(t, tntDB.BaseURL)
		assert.Nil(t, tntDB.SDPUIBaseURL)
		assert.Empty(t, tntDB.CORSAllowedOrigins)

		// Partial Update
		addr := keypair.MustRandom().Address()
		tnt, err := m.UpdateTenantConfig(ctx, &TenantUpdate{
			ID:                    tntDB.ID,
			EmailSenderType:       &AWSEmailSenderType,
			SEP10SigningPublicKey: &addr,
			EnableMFA:             &[]bool{false}[0],
			CORSAllowedOrigins:    []string{"https://myorg.sdp.io", "https://myorg-dev.sdp.io"},
			SDPUIBaseURL:          &[]string{"https://myorg.frontend.io"}[0],
		})
		require.NoError(t, err)

		assert.Equal(t, tnt.EmailSenderType, AWSEmailSenderType)
		assert.Equal(t, tnt.SMSSenderType, DryRunSMSSenderType)
		assert.Equal(t, addr, *tnt.SEP10SigningPublicKey)
		assert.Nil(t, tnt.DistributionPublicKey)
		assert.False(t, tnt.EnableMFA)
		assert.True(t, tnt.EnableReCAPTCHA)
		assert.Nil(t, tnt.BaseURL)
		assert.Equal(t, "https://myorg.frontend.io", *tnt.SDPUIBaseURL)
		assert.ElementsMatch(t, []string{"https://myorg.sdp.io", "https://myorg-dev.sdp.io"}, tnt.CORSAllowedOrigins)

		tnt, err = m.UpdateTenantConfig(ctx, &TenantUpdate{
			ID:                    tntDB.ID,
			SMSSenderType:         &TwilioSMSSenderType,
			DistributionPublicKey: &addr,
			EnableReCAPTCHA:       &[]bool{false}[0],
			BaseURL:               &[]string{"https://myorg.backend.io"}[0],
		})
		require.NoError(t, err)

		assert.Equal(t, tnt.EmailSenderType, AWSEmailSenderType)
		assert.Equal(t, tnt.SMSSenderType, TwilioSMSSenderType)
		assert.Equal(t, addr, *tnt.SEP10SigningPublicKey)
		assert.Equal(t, addr, *tnt.DistributionPublicKey)
		assert.False(t, tnt.EnableMFA)
		assert.False(t, tnt.EnableReCAPTCHA)
		assert.Equal(t, "https://myorg.backend.io", *tnt.BaseURL)
		assert.Equal(t, "https://myorg.frontend.io", *tnt.SDPUIBaseURL)
		assert.ElementsMatch(t, []string{"https://myorg.sdp.io", "https://myorg-dev.sdp.io"}, tnt.CORSAllowedOrigins)
	})
}

func Test_Manager_RunMigrationsForTenant(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
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

	u, err := url.Parse(dbConnectionPool.DSN())
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
	err = m.RunMigrationsForTenant(ctx, tnt1, tnt1SchemaConnectionPool.DSN(), migrate.Up, 0, sdpmigrations.FS, db.StellarSDPMigrationsTableName)
	require.NoError(t, err)
	err = m.RunMigrationsForTenant(ctx, tnt1, tnt1SchemaConnectionPool.DSN(), migrate.Up, 0, authmigrations.FS, db.StellarAuthMigrationsTableName)
	require.NoError(t, err)

	expectedTablesAfterMigrationsApplied := []string{
		"assets",
		"auth_migrations",
		"auth_user_mfa_codes",
		"auth_user_password_reset",
		"auth_users",
		"channel_accounts",
		"countries",
		"disbursements",
		"gorp_migrations",
		"messages",
		"organizations",
		"payments",
		"receiver_verifications",
		"receiver_wallets",
		"receivers",
		"submitter_transactions",
		"wallets",
		"wallets_assets",
	}
	TenantSchemaHasTablesFixture(t, ctx, dbConnectionPool, tnt1SchemaName, expectedTablesAfterMigrationsApplied)

	// Asserting if the Tenant 2 DB Schema wasn't affected by Tenant 1 schema migrations
	TenantSchemaHasTablesFixture(t, ctx, dbConnectionPool, tnt2SchemaName, []string{})

	// Apply migrations for Tenant 2
	err = m.RunMigrationsForTenant(ctx, tnt2, tnt2SchemaConnectionPool.DSN(), migrate.Up, 0, sdpmigrations.FS, db.StellarSDPMigrationsTableName)
	require.NoError(t, err)
	err = m.RunMigrationsForTenant(ctx, tnt2, tnt2SchemaConnectionPool.DSN(), migrate.Up, 0, authmigrations.FS, db.StellarAuthMigrationsTableName)
	require.NoError(t, err)

	TenantSchemaHasTablesFixture(t, ctx, dbConnectionPool, tnt2SchemaName, expectedTablesAfterMigrationsApplied)
}

func Test_Manager_GetAllTenants(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	tnt1, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)
	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)

	tenants, err := m.GetAllTenants(ctx)
	require.NoError(t, err)

	assert.ElementsMatch(t, tenants, []Tenant{*tnt1, *tnt2})
}

func Test_Manager_GetTenantByIDOrName(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	tnt1, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)
	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)

	t.Run("gets tenant by ID successfully", func(t *testing.T) {
		tntDB, err := m.GetTenantByIDOrName(ctx, tnt1.ID)
		require.NoError(t, err)
		assert.Equal(t, tnt1, tntDB)
	})

	t.Run("gets tenant by name successfully", func(t *testing.T) {
		tntDB, err := m.GetTenantByIDOrName(ctx, tnt2.Name)
		require.NoError(t, err)
		assert.Equal(t, tnt2, tntDB)
	})

	t.Run("returns error when tenant is not found", func(t *testing.T) {
		tntDB, err := m.GetTenantByIDOrName(ctx, "unknown")
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})
}
