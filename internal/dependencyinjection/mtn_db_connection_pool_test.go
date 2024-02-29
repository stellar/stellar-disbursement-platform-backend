package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dependencyinjection_NewMtnDBConnectionPool(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()
	t.Run("should create and return the same instance on the second call", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		// adminDBConnectionPool is retrieved here because it's a dependency of the MtnDBConnectionPool that needs to be closed.
		adminDBConnectionPool, err := NewAdminDBConnectionPool(ctx, DBConnectionPoolOptions{DatabaseURL: dbt.DSN})
		require.NoError(t, err)
		defer adminDBConnectionPool.Close()

		gotDependency, err := NewMtnDBConnectionPool(ctx, DBConnectionPoolOptions{DatabaseURL: dbt.DSN})
		require.NoError(t, err)
		defer gotDependency.Close()
		assert.IsType(t, &db.ConnectionPoolWithRouter{}, gotDependency)

		gotDependencyDuplicate, err := NewMtnDBConnectionPool(ctx, DBConnectionPoolOptions{DatabaseURL: dbt.DSN})
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)

		// Checks that the search_path is set.
		tenantInfo := &tenant.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e44", Name: "aid-org-1"}
		ctxWithTenant := tenant.SaveTenantInContext(ctx, tenantInfo)
		mtnDatabaseDSN, err := gotDependency.DSN(ctxWithTenant)
		require.NoError(t, err)
		assert.Contains(t, mtnDatabaseDSN, "search_path")
		assert.Contains(t, mtnDatabaseDSN, tenantInfo.Name)
	})

	t.Run("should return an error on a invalid option", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		gotDependency, err := NewMtnDBConnectionPool(ctx, DBConnectionPoolOptions{})
		assert.Nil(t, gotDependency)
		assert.ErrorContains(t, err, "opening Admin DB connection pool from NewMtnDBConnectionPool")
		assert.ErrorContains(t, err, "error pinging app DB connection pool")
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		SetInstance(MtnDBConnectionPoolInstanceName, false)

		gotDependency, err := NewMtnDBConnectionPool(ctx, DBConnectionPoolOptions{DatabaseURL: dbt.DSN})
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "trying to cast multitenant DBConnectionPool for depencency injection")
	})
}
