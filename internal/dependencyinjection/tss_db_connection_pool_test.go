package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

func Test_dependencyinjection_NewTSSDBConnectionPool(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	ctx := context.Background()

	t.Run("should create and return the same instance on the second call (without Metrics)", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := DBConnectionPoolOptions{DatabaseURL: dbt.DSN}

		gotDependency, err := NewTSSDBConnectionPool(ctx, opts)
		require.NoError(t, err)
		defer gotDependency.Close()
		assert.IsType(t, &db.DBConnectionPoolImplementation{}, gotDependency)

		gotDependencyDuplicate, err := NewTSSDBConnectionPool(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)

		tssDatabaseDSN, err := gotDependency.DSN(context.Background())
		require.NoError(t, err)
		assert.Contains(t, tssDatabaseDSN, "search_path="+router.TSSSchemaName)
	})

	t.Run("should create and return the same instance on the second call (with Metrics)", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		mMonitorService := &monitor.MockMonitorService{}
		opts := DBConnectionPoolOptions{DatabaseURL: dbt.DSN, MonitorService: mMonitorService}

		gotDependency, err := NewTSSDBConnectionPool(ctx, opts)
		require.NoError(t, err)
		defer gotDependency.Close()
		assert.IsType(t, &db.DBConnectionPoolWithMetrics{}, gotDependency)

		gotDependencyDuplicate, err := NewTSSDBConnectionPool(ctx, opts)
		require.NoError(t, err)

		assert.Equal(t, &gotDependency, &gotDependencyDuplicate)

		tssDatabaseDSN, err := gotDependency.DSN(context.Background())
		require.NoError(t, err)
		assert.Contains(t, tssDatabaseDSN, "search_path="+router.TSSSchemaName)
	})

	t.Run("should return an error on a invalid option", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		opts := DBConnectionPoolOptions{}
		gotDependency, err := NewTSSDBConnectionPool(ctx, opts)
		assert.Nil(t, gotDependency)
		assert.ErrorContains(t, err, "opening TSS DB connection pool: error pinging app DB connection pool")
	})

	t.Run("should return an error if there's an invalid instance pre-stored", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		SetInstance(TSSDBConnectionPoolInstanceName, false)

		opts := DBConnectionPoolOptions{DatabaseURL: dbt.DSN}
		gotDependency, err := NewTSSDBConnectionPool(ctx, opts)
		assert.Nil(t, gotDependency)
		assert.EqualError(t, err, "trying to cast TSS DBConnectionPool client for depencency injection")
	})
}
