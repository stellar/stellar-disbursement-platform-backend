package tenant

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
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
	})

	t.Run("returns error when tenant name is duplicated", func(t *testing.T) {
		tnt, err := m.AddTenant(ctx, "myorg")
		require.NoError(t, err)
		assert.NotNil(t, tnt)
		assert.NotEmpty(t, tnt.ID)
		assert.Equal(t, "myorg", tnt.Name)

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
