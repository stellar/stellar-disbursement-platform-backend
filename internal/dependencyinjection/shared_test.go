package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
)

func Test_openDBConnectionPool(t *testing.T) {
	ctx := context.Background()
	dbt := dbtest.Open(t)
	defer dbt.Close()

	t.Run("handle options without metrics", func(t *testing.T) {
		dbConnectionPool, err := openDBConnectionPool(ctx, dbt.DSN, DBConnectionPoolOptions{})
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		require.IsType(t, &db.DBConnectionPoolImplementation{}, dbConnectionPool)
	})

	t.Run("handle options with metrics", func(t *testing.T) {
		mMonitorService := monitorMocks.NewMockMonitorService(t)
		// Expect 8 RegisterFunctionMetric calls for database connection pool metrics
		mMonitorService.On("RegisterFunctionMetric", mock.Anything, mock.Anything).Times(8)
		dbConnectionPool, err := openDBConnectionPool(ctx, dbt.DSN, DBConnectionPoolOptions{MonitorService: mMonitorService})
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		require.IsType(t, &db.DBConnectionPoolWithMetrics{}, dbConnectionPool)
	})
}
