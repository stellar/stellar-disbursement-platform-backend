package tenant

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
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

func pointerTo[T any](v T) *T {
	return &v
}

func Test_Manager_UpdateTenantConfig(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := NewManager(WithDatabase(dbConnectionPool))

	testCases := []struct {
		name                   string
		tenantUpdateFn         func(tnt Tenant) *TenantUpdate
		expectedErrorContains  string
		expectedFieldsToAssert map[string]interface{}
	}{
		{
			name: "returns error when tenant update is nil",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return nil
			},
			expectedErrorContains: "tenant update cannot be nil",
		},
		{
			name: "returns error when no field has changed",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: tnt.ID}
			},
			expectedErrorContains: "provide at least one field to be updated",
		},
		{
			name: "returns error when the tenant ID does not exist",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: "abc", BaseURL: pointerTo("https://myorg.test.com")}
			},
			expectedErrorContains: ErrTenantDoesNotExist.Error(),
		},
		{
			name: "🎉 successfully updates the tenant [BaseURL]",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: tnt.ID, BaseURL: pointerTo("https://myorg.test.com")}
			},
			expectedFieldsToAssert: map[string]interface{}{
				"base_url": "https://myorg.test.com",
			},
		},
		{
			name: "🎉 successfully updates the tenant [SDPUIBaseURL]",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: tnt.ID, SDPUIBaseURL: pointerTo("https://ui.myorg.test.com")}
			},
			expectedFieldsToAssert: map[string]interface{}{
				"sdp_ui_base_url": "https://ui.myorg.test.com",
			},
		},
		{
			name: "🎉 successfully updates the tenant [Status]",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: tnt.ID, Status: pointerTo(DeactivatedTenantStatus)}
			},
			expectedFieldsToAssert: map[string]interface{}{
				"status": string(DeactivatedTenantStatus),
			},
		},
		{
			name: "🎉 successfully updates the tenant [DistributionAccountAddress]",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: tnt.ID, DistributionAccountAddress: "GCK6GPKFTIGJJM7OHSQH7O7ORSKTUK37ZUDEUXZRFMIQNBUBZDEPU5KS"}
			},
			expectedFieldsToAssert: map[string]interface{}{
				"distribution_account_address": "GCK6GPKFTIGJJM7OHSQH7O7ORSKTUK37ZUDEUXZRFMIQNBUBZDEPU5KS",
			},
		},
		{
			name: "🎉 successfully updates the tenant [DistributionAccountType]",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: tnt.ID, DistributionAccountType: schema.DistributionAccountStellarEnv}
			},
			expectedFieldsToAssert: map[string]interface{}{
				"distribution_account_type": string(schema.DistributionAccountStellarEnv),
			},
		},
		{
			name: "🎉 successfully updates the tenant [DistributionAccountStatus]",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{ID: tnt.ID, DistributionAccountStatus: schema.AccountStatusPendingUserActivation}
			},
			expectedFieldsToAssert: map[string]interface{}{
				"distribution_account_status": string(schema.AccountStatusPendingUserActivation),
			},
		},
		{
			name: "🎉 successfully updates the tenant (ALL FIELDS)",
			tenantUpdateFn: func(tnt Tenant) *TenantUpdate {
				return &TenantUpdate{
					ID:                         tnt.ID,
					BaseURL:                    pointerTo("https://myorg.test.com"),
					SDPUIBaseURL:               pointerTo("https://ui.myorg.test.com"),
					Status:                     pointerTo(DeactivatedTenantStatus),
					DistributionAccountAddress: "GCK6GPKFTIGJJM7OHSQH7O7ORSKTUK37ZUDEUXZRFMIQNBUBZDEPU5KS",
					DistributionAccountType:    schema.DistributionAccountStellarEnv,
					DistributionAccountStatus:  schema.AccountStatusPendingUserActivation,
				}
			},
			expectedFieldsToAssert: map[string]interface{}{
				"base_url":                     "https://myorg.test.com",
				"sdp_ui_base_url":              "https://ui.myorg.test.com",
				"status":                       string(DeactivatedTenantStatus),
				"distribution_account_address": "GCK6GPKFTIGJJM7OHSQH7O7ORSKTUK37ZUDEUXZRFMIQNBUBZDEPU5KS",
				"distribution_account_type":    string(schema.DistributionAccountStellarEnv),
				"distribution_account_status":  string(schema.AccountStatusPendingUserActivation),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer DeleteAllTenantsFixture(t, ctx, dbConnectionPool)
			tnt, err := m.AddTenant(ctx, "myorg")
			require.NoError(t, err)

			var tenantUpdate *TenantUpdate
			if tc.tenantUpdateFn != nil {
				tenantUpdate = tc.tenantUpdateFn(*tnt)
			}

			updatedTnt, err := m.UpdateTenantConfig(ctx, tenantUpdate)
			if tc.expectedErrorContains != "" {
				t.Log(err)
				assert.ErrorContains(t, err, tc.expectedErrorContains)
				assert.Nil(t, updatedTnt)
			} else {
				require.NoError(t, err)

				// assert that the updated value is the same as the DB one:
				require.NotNil(t, tc.expectedFieldsToAssert)
				queryParams := &QueryParams{Filters: map[FilterKey]interface{}{
					FilterKeyID: tnt.ID,
				}}
				dbTnt, err := m.GetTenant(ctx, queryParams)
				require.NoError(t, err)
				assert.Equal(t, dbTnt, updatedTnt)

				// parse to map, so we can easily assert a subset of fields:
				tntBytes, err := json.Marshal(dbTnt)
				require.NoError(t, err)
				tntMap := map[string]interface{}{}
				err = json.Unmarshal(tntBytes, &tntMap)
				require.NoError(t, err)
				assert.Subset(t, tntMap, tc.expectedFieldsToAssert)
			}
		})
	}
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
		tntDB, dbErr := m.GetTenantByIDOrName(ctx, tnt1.ID)
		require.NoError(t, dbErr)
		assert.Equal(t, tnt1, tntDB)
	})

	t.Run("gets tenant by name successfully", func(t *testing.T) {
		tntDB, dbErr := m.GetTenantByIDOrName(ctx, tnt2.Name)
		require.NoError(t, dbErr)
		assert.Equal(t, tnt2, tntDB)
	})

	t.Run("returns error when tenant is deactivated", func(t *testing.T) {
		deactivateTenant(t, ctx, m, tnt2)
		tntDB, dbErr := m.GetTenantByIDOrName(ctx, tnt2.ID)
		assert.ErrorIs(t, dbErr, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)

		tntDB, dbErr = m.GetTenantByIDOrName(ctx, tnt2.Name)
		assert.ErrorIs(t, dbErr, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})

	t.Run("returns error when tenant is not found", func(t *testing.T) {
		tntDB, dbErr := m.GetTenantByIDOrName(ctx, "unknown")
		assert.ErrorIs(t, dbErr, ErrTenantDoesNotExist)
		assert.Nil(t, tntDB)
	})
}

func activateTenant(t *testing.T, ctx context.Context, m *Manager, tnt *Tenant) {
	_, err := m.UpdateTenantConfig(ctx, &TenantUpdate{ID: tnt.ID, Status: pointerTo(ActivatedTenantStatus)})
	require.NoError(t, err)
}

func deactivateTenant(t *testing.T, ctx context.Context, m *Manager, tnt *Tenant) {
	_, err := m.UpdateTenantConfig(ctx, &TenantUpdate{ID: tnt.ID, Status: pointerTo(DeactivatedTenantStatus)})
	require.NoError(t, err)
}

func updateTenantIsDefault(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, tenantID string, isDefault bool) {
	const q = "UPDATE tenants SET is_default = $1 WHERE id = $2"
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
		defaultTnt, dbErr := m.GetDefault(ctx)
		assert.EqualError(t, dbErr, ErrTenantDoesNotExist.Error())
		assert.Nil(t, defaultTnt)
	})

	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt1.ID, true)
	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt2.ID, true)

	t.Run("returns error when there's multiple default tenants", func(t *testing.T) {
		defaultTnt, dbErr := m.GetDefault(ctx)
		assert.EqualError(t, dbErr, ErrTooManyDefaultTenants.Error())
		assert.Nil(t, defaultTnt)
	})

	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt1.ID, false)

	t.Run("returns error when default tenant is inactive", func(t *testing.T) {
		deactivateTenant(t, ctx, m, tnt2)
		defaultTnt, dbErr := m.GetDefault(ctx)
		assert.EqualError(t, dbErr, ErrTenantDoesNotExist.Error())
		assert.Nil(t, defaultTnt)
	})

	updateTenantIsDefault(t, ctx, dbConnectionPool, tnt2.ID, true)
	activateTenant(t, ctx, m, tnt2)

	t.Run("gets the default tenant successfully", func(t *testing.T) {
		tntDB, dbErr := m.GetDefault(ctx)
		require.NoError(t, dbErr)
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
		tnt, dbErr := db.RunInTransactionWithResult(ctx, dbConnectionPool, nil, func(dbTx db.DBTransaction) (*Tenant, error) {
			dTnt, innerErr := m.SetDefault(ctx, dbTx, "some-id")
			return dTnt, innerErr
		})
		assert.ErrorIs(t, dbErr, ErrTenantDoesNotExist)
		assert.Nil(t, tnt)

		tnt1DB, dbErr := m.GetTenantByID(ctx, tnt1.ID)
		require.NoError(t, dbErr)
		assert.True(t, tnt1DB.IsDefault)
	})

	t.Run("returns error when attempting to set deactivated tenant to default", func(t *testing.T) {
		tnt3, dbErr := m.AddTenant(ctx, "myorg3")
		require.NoError(t, dbErr)
		deactivateTenant(t, ctx, m, tnt3)

		tnt, dbErr := m.SetDefault(ctx, dbConnectionPool, tnt3.Name)
		assert.ErrorIs(t, dbErr, ErrTenantDoesNotExist)
		assert.Nil(t, tnt)

		tnt3DB, dbErr := m.GetTenant(ctx, &QueryParams{
			Filters: map[FilterKey]interface{}{FilterKeyID: tnt3.ID},
		})
		require.NoError(t, dbErr)
		assert.False(t, tnt3DB.IsDefault)
	})

	t.Run("updates default tenant", func(t *testing.T) {
		tnt2DB, dbErr := m.SetDefault(ctx, dbConnectionPool, tnt2.ID)
		require.NoError(t, dbErr)

		assert.Equal(t, tnt2.ID, tnt2DB.ID)
		assert.True(t, tnt2DB.IsDefault)

		tnt1DB, dbErr := m.GetTenantByID(ctx, tnt1.ID)
		require.NoError(t, dbErr)
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
		err = m.DeleteTenantByName(ctx, tnt.Name)
		require.NoError(t, err)

		_, err = m.GetTenantByName(ctx, tnt.Name)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
	})

	t.Run("returns error when tenant name is empty", func(t *testing.T) {
		err = m.DeleteTenantByName(ctx, "")
		assert.ErrorIs(t, err, ErrEmptyTenantName)
	})
}

func Test_Manager_SoftDeleteTenantByID(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	tnt, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)

	t.Run("returns error when tenant does not exist", func(t *testing.T) {
		_, err = m.SoftDeleteTenantByID(ctx, "invalid-tnt")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
	})

	t.Run("returns error when tenant is not deactivated", func(t *testing.T) {
		_, err = m.SoftDeleteTenantByID(ctx, tnt.ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTenantDoesNotExist)
	})

	t.Run("successfully soft deletes tenant", func(t *testing.T) {
		deactivateTenant(t, ctx, m, tnt)
		dbTnt, dbErr := m.SoftDeleteTenantByID(ctx, tnt.ID)
		require.NoError(t, dbErr)
		require.NotNil(t, dbTnt.DeletedAt)

		dbTnt, dbErr = m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyID: tnt.ID}})
		require.NoError(t, dbErr)
		assert.NotNil(t, dbTnt.DeletedAt)
	})
}

func TestManager_DeactivateTenantDistributionAccount(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := NewManager(WithDatabase(dbConnectionPool))
	tnt, err := m.AddTenant(ctx, "myorg1")
	require.NoError(t, err)

	t.Run("does not deactivate distribution account if not managed by Circle", func(t *testing.T) {
		err = m.DeactivateTenantDistributionAccount(ctx, tnt.ID)
		require.NoError(t, err)

		dbTnt, dbErr := m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyID: tnt.ID}})
		require.NoError(t, dbErr)
		assert.NotEqual(t, schema.AccountStatusPendingUserActivation, dbTnt.DistributionAccountStatus)
	})

	t.Run("does not deactivate distribution account if tenant is deactivated", func(t *testing.T) {
		tnt, err = m.UpdateTenantConfig(
			ctx, &TenantUpdate{
				ID:                      tnt.ID,
				DistributionAccountType: schema.DistributionAccountCircleDBVault,
				Status:                  pointerTo(DeactivatedTenantStatus),
			})
		require.NoError(t, err)

		err = m.DeactivateTenantDistributionAccount(ctx, tnt.ID)
		require.NoError(t, err)

		dbTnt, dbErr := m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyID: tnt.ID}})
		require.NoError(t, dbErr)
		assert.NotEqual(t, schema.AccountStatusPendingUserActivation, dbTnt.DistributionAccountStatus)
	})

	t.Run("operation is idempotent if distribution account is already deactivated", func(t *testing.T) {
		tnt, err = m.UpdateTenantConfig(
			ctx, &TenantUpdate{
				ID:                        tnt.ID,
				DistributionAccountType:   schema.DistributionAccountCircleDBVault,
				DistributionAccountStatus: schema.AccountStatusPendingUserActivation,
			})
		require.NoError(t, err)

		err = m.DeactivateTenantDistributionAccount(ctx, tnt.ID)
		require.NoError(t, err)

		dbTnt, dbErr := m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyID: tnt.ID}})
		require.NoError(t, dbErr)
		assert.Equal(t, schema.AccountStatusPendingUserActivation, dbTnt.DistributionAccountStatus)
	})

	t.Run("successfully deactivates tenant distribution account", func(t *testing.T) {
		tnt, err = m.UpdateTenantConfig(
			ctx, &TenantUpdate{
				ID:                      tnt.ID,
				DistributionAccountType: schema.DistributionAccountCircleDBVault,
				Status:                  pointerTo(ActivatedTenantStatus),
			})
		require.NoError(t, err)
		err = m.DeactivateTenantDistributionAccount(ctx, tnt.ID)
		require.NoError(t, err)

		dbTnt, dbErr := m.GetTenant(ctx, &QueryParams{Filters: map[FilterKey]interface{}{FilterKeyID: tnt.ID}})
		require.NoError(t, dbErr)
		assert.Equal(t, schema.AccountStatusPendingUserActivation, dbTnt.DistributionAccountStatus)
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
