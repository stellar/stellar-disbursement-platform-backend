package tenant

import (
	"context"
	"fmt"
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
		baseURL := "https://myorg.backend.io"
		tnt, err := m.UpdateTenantConfig(ctx, &TenantUpdate{ID: "abc", BaseURL: &baseURL})
		assert.Equal(t, ErrTenantDoesNotExist, err)
		assert.Nil(t, tnt)
	})

	t.Run("updates tenant config successfully", func(t *testing.T) {
		tntDB = ResetTenantConfigFixture(t, ctx, dbConnectionPool, tntDB.ID)
		assert.Nil(t, tntDB.BaseURL)
		assert.Nil(t, tntDB.SDPUIBaseURL)

		// Partial Update
		tnt, err := m.UpdateTenantConfig(ctx, &TenantUpdate{
			ID:           tntDB.ID,
			SDPUIBaseURL: &[]string{"https://myorg.frontend.io"}[0],
		})
		require.NoError(t, err)

		assert.Nil(t, tnt.BaseURL)
		assert.Equal(t, "https://myorg.frontend.io", *tnt.SDPUIBaseURL)

		tnt, err = m.UpdateTenantConfig(ctx, &TenantUpdate{
			ID:      tntDB.ID,
			BaseURL: &[]string{"https://myorg.backend.io"}[0],
		})
		require.NoError(t, err)

		assert.Equal(t, "https://myorg.backend.io", *tnt.BaseURL)
		assert.Equal(t, "https://myorg.frontend.io", *tnt.SDPUIBaseURL)
	})
}

func Test_Manager_GetAllTenants(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
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

	tenants, err := m.GetAllTenants(ctx, nil)
	require.NoError(t, err)

	assert.ElementsMatch(t, tenants, []Tenant{*tnt1, *tnt2})

	deactivateTenant(t, ctx, m, tnt1)
	tenants, err = m.GetAllTenants(ctx, nil)
	require.NoError(t, err)

	assert.ElementsMatch(t, tenants, []Tenant{*tnt2})
}

func Test_Manager_GetTenant(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
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

	dbTnt1, err := m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyID: tnt1.ID}})
	require.NoError(t, err)
	assert.Equal(t, *tnt1, *dbTnt1)

	dbTnt1, err = m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyName: tnt1.Name}})
	require.NoError(t, err)
	assert.Equal(t, *tnt1, *dbTnt1)

	dbTnt2, err := m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyID: tnt2.ID}})
	require.NoError(t, err)
	assert.Equal(t, *tnt2, *dbTnt2)

	dbTnt2, err = m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyName: tnt2.Name}})
	require.NoError(t, err)
	assert.Equal(t, *tnt2, *dbTnt2)
}

func Test_Manager_GetTenantByID(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
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

	t.Run("returns error when tenant is deactivated", func(t *testing.T) {
		deactivateTenant(t, ctx, m, tnt2)
		tntDB, err := m.GetTenantByID(ctx, tnt2.ID)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})

	t.Run("returns error when tenant is not found", func(t *testing.T) {
		tntDB, err := m.GetTenantByID(ctx, "unknown")
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})
}

func Test_Manager_GetTenantByName(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
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

	t.Run("returns error when tenant is deactivated", func(t *testing.T) {
		deactivateTenant(t, ctx, m, tnt2)
		tntDB, err := m.GetTenantByName(ctx, tnt2.ID)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})

	t.Run("returns error when tenant is not found", func(t *testing.T) {
		tntDB, err := m.GetTenantByName(ctx, "unknown")
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})
}

func Test_Manager_GetTenantByIDOrName(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
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

	t.Run("returns error when tenant is deactivated", func(t *testing.T) {
		deactivateTenant(t, ctx, m, tnt2)
		tntDB, err := m.GetTenantByIDOrName(ctx, tnt2.ID)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)

		tntDB, err = m.GetTenantByIDOrName(ctx, tnt2.Name)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})

	t.Run("returns error when tenant is not found", func(t *testing.T) {
		tntDB, err := m.GetTenantByIDOrName(ctx, "unknown")
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})
}

func activateTenant(t *testing.T, ctx context.Context, m *Manager, tnt *Tenant) {
	activatedStatus := ActivatedTenantStatus
	_, err := m.UpdateTenantConfig(ctx, &TenantUpdate{ID: tnt.ID, Status: &activatedStatus})
	require.NoError(t, err)
}

func deactivateTenant(t *testing.T, ctx context.Context, m *Manager, tnt *Tenant) {
	deactivatedStatus := DeactivatedTenantStatus
	_, err := m.UpdateTenantConfig(ctx, &TenantUpdate{ID: tnt.ID, Status: &deactivatedStatus})
	require.NoError(t, err)
}

func updateTenantIsDefault(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, tenantID string, isDefault bool) {
	const q = "UPDATE public.tenants SET is_default = $1 WHERE id = $2"
	_, err := dbConnectionPool.ExecContext(ctx, q, isDefault, tenantID)
	require.NoError(t, err)
}

func Test_Manager_GetDefault(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
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

	t.Run("returns error when there's no default tenant", func(t *testing.T) {
		defaultTnt, err := m.GetDefault(ctx)
		assert.EqualError(t, err, ErrTenantDoesNotExist.Error())
		assert.Nil(t, defaultTnt)
	})

	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt1.ID, true)
	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt2.ID, true)

	t.Run("returns error when there's multiple default tenants", func(t *testing.T) {
		defaultTnt, err := m.GetDefault(ctx)
		assert.EqualError(t, err, ErrTooManyDefaultTenants.Error())
		assert.Nil(t, defaultTnt)
	})

	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt1.ID, false)

	t.Run("returns error when default tenant is inactive", func(t *testing.T) {
		deactivateTenant(t, ctx, m, tnt2)
		defaultTnt, err := m.GetDefault(ctx)
		assert.EqualError(t, err, ErrTenantDoesNotExist.Error())
		assert.Nil(t, defaultTnt)
	})

	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt2.ID, true)
	activateTenant(t, ctx, m, tnt2)

	t.Run("gets the default tenant successfully", func(t *testing.T) {
		tntDB, err := m.GetDefault(ctx)
		require.NoError(t, err)
		assert.Equal(t, tnt2.ID, tntDB.ID)
		assert.Equal(t, tnt2.Name, tntDB.Name)
		assert.True(t, tntDB.IsDefault)
	})
}

func Test_Manager_SetDefault(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))

	t.Run("returns error when tenant does not exist", func(t *testing.T) {
		tnt, dErr := m.SetDefault(ctx, dbConnectionPool, "some-id")
		assert.EqualError(t, dErr, ErrTenantDoesNotExist.Error())
		assert.Nil(t, tnt)
	})

	tnt1, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)
	tnt2, err := m.AddTenant(ctx, "myorg2")
	require.NoError(t, err)
	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt1.ID, true)

	t.Run("ensures the default tenant is not changed when an error occurs", func(t *testing.T) {
		tnt, err := db.RunInTransactionWithResult(ctx, dbConnectionPool, nil, func(dbTx db.DBTransaction) (*Tenant, error) {
			dTnt, innerErr := m.SetDefault(ctx, dbTx, "some-id")
			return dTnt, innerErr
		})
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tnt)

		tnt1DB, err := m.GetTenantByID(ctx, tnt1.ID)
		require.NoError(t, err)
		assert.True(t, tnt1DB.IsDefault)
	})

	t.Run("returns error when attempting to set deactivated tenant to default", func(t *testing.T) {
		tnt3, err := m.AddTenant(ctx, "myorg3")
		require.NoError(t, err)
		deactivateTenant(t, ctx, m, tnt3)

		tnt, err := m.SetDefault(ctx, dbConnectionPool, tnt3.Name)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
		assert.Nil(t, tnt)

		tnt3DB, err := m.GetTenant(ctx, &QueryParams{
			Filters: map[FilterKey]interface{}{FilterKeyID: tnt3.ID},
		})
		require.NoError(t, err)
		assert.False(t, tnt3DB.IsDefault)
	})

	t.Run("updates default tenant", func(t *testing.T) {
		tnt2DB, err := m.SetDefault(ctx, dbConnectionPool, tnt2.ID)
		require.NoError(t, err)

		assert.Equal(t, tnt2.ID, tnt2DB.ID)
		assert.True(t, tnt2DB.IsDefault)

		tnt1DB, err := m.GetTenantByID(ctx, tnt1.ID)
		require.NoError(t, err)
		assert.Equal(t, tnt1.ID, tnt1DB.ID)
		assert.False(t, tnt1DB.IsDefault)
	})
}

func Test_Manager_DeleteTenantByName(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	tnt, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)

	t.Run("deletes tenant successfully", func(t *testing.T) {
		err := m.DeleteTenantByName(ctx, tnt.Name)
		require.NoError(t, err)

		_, err = m.GetTenantByName(ctx, tnt.Name)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
	})

	t.Run("returns error when tenant name is empty", func(t *testing.T) {
		err := m.DeleteTenantByName(ctx, "")
		assert.ErrorIs(t, err, ErrEmptyTenantName)
	})
}

func Test_Manager_DropTenantSchema(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	orgName := "myorg1"
	require.NoError(t, err)

	err = m.DropTenantSchema(ctx, orgName)
	require.NoError(t, err)

	query := "SELECT exists(select schema_name FROM information_schema.schemata WHERE schema_name = $1)"
	var exists bool
	err = dbConnectionPool.GetContext(ctx, &exists, query, fmt.Sprintf("sdp_%s", orgName))
	require.NoError(t, err)
	assert.False(t, exists)
}

func Test_Manager_CreateTenantSchema(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	orgName := "myorg1"
	require.NoError(t, err)

	err = m.CreateTenantSchema(ctx, orgName)
	require.NoError(t, err)

	query := "SELECT exists(select schema_name FROM information_schema.schemata WHERE schema_name = $1)"
	var exists bool
	err = dbConnectionPool.GetContext(ctx, &exists, query, fmt.Sprintf("sdp_%s", orgName))
	require.NoError(t, err)
	assert.True(t, exists)

	// attempt to create the same schema again, which fails
	err = m.CreateTenantSchema(ctx, orgName)
	require.ErrorContains(t, err, fmt.Sprintf("creating schema for tenant sdp_%s: pq: schema \"sdp_%s\" already exists", orgName, orgName))
}
