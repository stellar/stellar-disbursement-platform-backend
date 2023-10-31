package tenant

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/lib/pq"
	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetTenantConfigFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, tenantID string) *Tenant {
	t.Helper()

	const q = `
		UPDATE tenants
		SET
			email_sender_type = DEFAULT, sms_sender_type = DEFAULT, sep10_signing_public_key = NULL,
			distribution_public_key = NULL, enable_mfa = DEFAULT, enable_recaptcha = DEFAULT,
			cors_allowed_origins = NULL, base_url = NULL, sdp_ui_base_url = NULL
		WHERE
			id = $1
		RETURNING *
	`

	var tnt Tenant
	err := dbConnectionPool.GetContext(ctx, &tnt, q, tenantID)
	require.NoError(t, err)

	return &tnt
}

func checkSearchPathExistsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, schemaName string) bool {
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

// tenantHasAllTablesFixture asserts if the new tenant database schema has all tables.
func tenantHasAllTablesFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, schemaName string, tableNames []string) {
	t.Helper()

	const q = `
		SELECT table_name FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name
	`

	var schemaTables []string
	err := dbConnectionPool.SelectContext(ctx, &schemaTables, q, schemaName)
	require.NoError(t, err)

	assert.ElementsMatch(t, tableNames, schemaTables)
}

func Test_Manager_ProvisionNewTenant(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))

	t.Run("provision a new tenant for the testnet", func(t *testing.T) {
		tenantName := "myorg-ukraine"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		tnt, err := m.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, string(utils.TestnetNetworkType))
		require.NoError(t, err)

		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		assert.Equal(t, tenantName, tnt.Name)
		assert.Equal(t, ProvisionedTenantStatus, tnt.Status)
		assert.True(t, checkSearchPathExistsFixture(t, ctx, dbConnectionPool, schemaName))

		// Connecting to the new schema
		dataSourceName := dbConnectionPool.DSN()
		u, err := url.Parse(dataSourceName)
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
		tenantHasAllTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)

		var registeredAssets []string
		queryRegisteredAssets := `
			SELECT CONCAT(code, ':', issuer) FROM assets
		`
		err = tenantSchemaConnectionPool.SelectContext(ctx, &registeredAssets, queryRegisteredAssets)
		require.NoError(t, err)
		expectedAssets := []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"}
		assert.ElementsMatch(t, expectedAssets, registeredAssets)

		var registeredWallets []string
		queryRegisteredWallets := `
			SELECT name FROM wallets
		`
		err = tenantSchemaConnectionPool.SelectContext(ctx, &registeredWallets, queryRegisteredWallets)
		require.NoError(t, err)
		expectedWallets := []string{"Demo Wallet", "Vibrant Assist"}
		assert.ElementsMatch(t, expectedWallets, registeredWallets)

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
		err = tenantSchemaConnectionPool.GetContext(ctx, &user, queryRegisteredUser, userEmail)
		require.NoError(t, err)
		assert.Equal(t, userFirstName, user.FirstName)
		assert.Equal(t, userLastName, user.LastName)
		assert.Equal(t, userEmail, user.Email)
		assert.Equal(t, pq.StringArray{"owner"}, user.Roles)
		assert.True(t, user.IsOwner)
	})

	t.Run("provision a new tenant for the pubnet", func(t *testing.T) {
		tenantName := "myorg-us"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		tnt, err := m.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, string(utils.PubnetNetworkType))
		require.NoError(t, err)

		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		assert.Equal(t, tenantName, tnt.Name)
		assert.Equal(t, ProvisionedTenantStatus, tnt.Status)
		assert.True(t, checkSearchPathExistsFixture(t, ctx, dbConnectionPool, schemaName))

		// Connecting to the new schema
		dataSourceName := dbConnectionPool.DSN()
		u, err := url.Parse(dataSourceName)
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
		tenantHasAllTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)

		var registeredAssets []string
		queryRegisteredAssets := `
			SELECT CONCAT(code, ':', issuer) FROM assets
		`
		err = tenantSchemaConnectionPool.SelectContext(ctx, &registeredAssets, queryRegisteredAssets)
		require.NoError(t, err)
		expectedAssets := []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"}
		assert.ElementsMatch(t, expectedAssets, registeredAssets)

		var registeredWallets []string
		queryRegisteredWallets := `
			SELECT name FROM wallets
		`
		err = tenantSchemaConnectionPool.SelectContext(ctx, &registeredWallets, queryRegisteredWallets)
		require.NoError(t, err)
		expectedWallets := []string{"Vibrant Assist", "Vibrant Assist RC"}
		assert.ElementsMatch(t, expectedWallets, registeredWallets)

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
		err = tenantSchemaConnectionPool.GetContext(ctx, &user, queryRegisteredUser, userEmail)
		require.NoError(t, err)
		assert.Equal(t, userFirstName, user.FirstName)
		assert.Equal(t, userLastName, user.LastName)
		assert.Equal(t, userEmail, user.Email)
		assert.Equal(t, pq.StringArray{"owner"}, user.Roles)
		assert.True(t, user.IsOwner)
	})
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
		tntDB = resetTenantConfigFixture(t, ctx, dbConnectionPool, tntDB.ID)
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
