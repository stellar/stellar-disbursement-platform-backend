package router

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/require"
)

func TestMultiTenantDataSourceRouter_GetDataSource(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	router := NewMultiTenantDataSourceRouter(tenantManager)

	ctx := context.Background()

	t.Run("error tenant not found in context", func(t *testing.T) {
		dbcp, err := router.GetDataSource(ctx)
		require.Nil(t, dbcp)
		require.EqualError(t, err, ErrTenantNotFoundInContext.Error())
	})

	t.Run("successfully getting data source", func(t *testing.T) {
		// Create a new context with tenant information
		tenantInfo := &tenant.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e44", Name: "aid-org-1"}
		ctx = SetTenantInContext(context.Background(), tenantInfo)

		dbcp, err := router.GetDataSource(ctx)
		require.NotNil(t, dbcp)
		require.NoError(t, err)
		defer dbcp.Close()

		dsn, err := dbcp.DSN(ctx)
		require.NoError(t, err)
		require.Contains(t, dsn, tenantInfo.Name)
	})
}

func TestMultiTenantDataSourceRouter_GetAllDataSources(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	router := NewMultiTenantDataSourceRouter(tenantManager)

	t.Run("empty data sources", func(t *testing.T) {
		dbcps, err := router.GetAllDataSources()
		require.NoError(t, err)
		require.Nil(t, dbcps)
		require.Empty(t, dbcps)
	})

	t.Run("successfully getting data sources", func(t *testing.T) {
		// Store DB Connection Pool for aid-org-1
		tenantInfo := &tenant.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e44", Name: "aid-org-1"}
		ctx := SetTenantInContext(context.Background(), tenantInfo)
		dbcp1, err := router.GetDataSource(ctx)
		require.NoError(t, err)
		require.NotNil(t, dbcp1)
		defer dbcp1.Close()

		// Store DB Connection Pool for aid-org-2
		tenantInfo = &tenant.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e45", Name: "aid-org-2"}
		ctx = SetTenantInContext(context.Background(), tenantInfo)
		dbcp2, err := router.GetDataSource(ctx)
		require.NoError(t, err)
		require.NotNil(t, dbcp2)
		defer dbcp2.Close()

		dbcps, err := router.GetAllDataSources()
		require.NotNil(t, dbcps)
		require.NoError(t, err)

		require.Equal(t, 2, len(dbcps))
		require.Contains(t, dbcps, dbcp1)
		require.Contains(t, dbcps, dbcp2)
	})
}

func TestMultiTenantDataSourceRouter_AnyDataSource(t *testing.T) {
	dbt := dbtest.OpenWithTenantMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	router := NewMultiTenantDataSourceRouter(tenantManager)

	t.Run("no data sources available", func(t *testing.T) {
		dbcp, err := router.AnyDataSource()
		require.Nil(t, dbcp)
		require.EqualError(t, err, ErrNoDataSourcesAvailable.Error())
	})

	t.Run("successfully getting data source", func(t *testing.T) {
		// Store DB Connection Pool for aid-org-1
		tenantInfo := &tenant.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e44", Name: "aid-org-1"}
		ctx := SetTenantInContext(context.Background(), tenantInfo)
		dbcp1, err := router.GetDataSource(ctx)
		require.NoError(t, err)
		require.NotNil(t, dbcp1)
		defer dbcp1.Close()

		dbcp, err := router.AnyDataSource()
		require.NotNil(t, dbcp)
		require.NoError(t, err)
		require.Equal(t, dbcp1, dbcp)
	})
}
