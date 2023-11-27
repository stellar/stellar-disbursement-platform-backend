package tenant

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		assert.True(t, tntDB.EnableMFA)
		assert.True(t, tntDB.EnableReCAPTCHA)
		assert.Nil(t, tntDB.BaseURL)
		assert.Nil(t, tntDB.SDPUIBaseURL)
		assert.Empty(t, tntDB.CORSAllowedOrigins)

		// Partial Update
		tnt, err := m.UpdateTenantConfig(ctx, &TenantUpdate{
			ID:                 tntDB.ID,
			EmailSenderType:    &AWSEmailSenderType,
			EnableMFA:          &[]bool{false}[0],
			CORSAllowedOrigins: []string{"https://myorg.sdp.io", "https://myorg-dev.sdp.io"},
			SDPUIBaseURL:       &[]string{"https://myorg.frontend.io"}[0],
		})
		require.NoError(t, err)

		assert.Equal(t, tnt.EmailSenderType, AWSEmailSenderType)
		assert.Equal(t, tnt.SMSSenderType, DryRunSMSSenderType)
		assert.False(t, tnt.EnableMFA)
		assert.True(t, tnt.EnableReCAPTCHA)
		assert.Nil(t, tnt.BaseURL)
		assert.Equal(t, "https://myorg.frontend.io", *tnt.SDPUIBaseURL)
		assert.ElementsMatch(t, []string{"https://myorg.sdp.io", "https://myorg-dev.sdp.io"}, tnt.CORSAllowedOrigins)

		tnt, err = m.UpdateTenantConfig(ctx, &TenantUpdate{
			ID:              tntDB.ID,
			SMSSenderType:   &TwilioSMSSenderType,
			EnableReCAPTCHA: &[]bool{false}[0],
			BaseURL:         &[]string{"https://myorg.backend.io"}[0],
		})
		require.NoError(t, err)

		assert.Equal(t, tnt.EmailSenderType, AWSEmailSenderType)
		assert.Equal(t, tnt.SMSSenderType, TwilioSMSSenderType)
		assert.False(t, tnt.EnableMFA)
		assert.False(t, tnt.EnableReCAPTCHA)
		assert.Equal(t, "https://myorg.backend.io", *tnt.BaseURL)
		assert.Equal(t, "https://myorg.frontend.io", *tnt.SDPUIBaseURL)
		assert.ElementsMatch(t, []string{"https://myorg.sdp.io", "https://myorg-dev.sdp.io"}, tnt.CORSAllowedOrigins)
	})
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

func Test_Manager_GetTenantByID(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	_, err = m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)
	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)

	t.Run("gets tenant successfully", func(t *testing.T) {
		tntDB, err := m.GetTenantByID(ctx, tnt2.ID)
		require.NoError(t, err)
		assert.Equal(t, tnt2, tntDB)
	})

	t.Run("returns error when tenant is not found", func(t *testing.T) {
		tntDB, err := m.GetTenantByID(ctx, "unknown")
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})
}

func Test_Manager_GetTenantByName(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	_, err = m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)
	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)

	t.Run("gets tenant successfully", func(t *testing.T) {
		tntDB, err := m.GetTenantByName(ctx, "myorg2")
		require.NoError(t, err)
		assert.Equal(t, tnt2, tntDB)
	})

	t.Run("returns error when tenant is not found", func(t *testing.T) {
		tntDB, err := m.GetTenantByName(ctx, "unknown")
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})
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
