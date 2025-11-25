package db

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/support/db/dbtest"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
)

func TestOpen_OpenDBConnectionPool(t *testing.T) {
	db := dbtest.Postgres(t)
	defer db.Close()

	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	assert.Equal(t, "postgres", dbConnectionPool.DriverName())

	ctx := context.Background()
	err = dbConnectionPool.Ping(ctx)
	require.NoError(t, err)
}

func TestOpen_OpenDBConnectionPoolWithMetrics(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Postgres(t)
	defer db.Close()

	mMonitorService := monitorMocks.NewMockMonitorService(t)
	// We're registering 8 Function metrics for database connection pool metrics
	mMonitorService.On("RegisterFunctionMetric", mock.Anything, mock.Anything).Times(8)

	dbConnectionPool, err := OpenDBConnectionPoolWithMetrics(ctx, db.DSN, mMonitorService)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	dbConnectionPoolWithMetrics, ok := dbConnectionPool.(*DBConnectionPoolWithMetrics)
	require.True(t, ok)
	innerDBConnectionPool := dbConnectionPoolWithMetrics.dbConnectionPool
	assert.IsType(t, &DBConnectionPoolImplementation{}, innerDBConnectionPool)
	assert.Equal(t, innerDBConnectionPool, dbConnectionPoolWithMetrics.SQLExecuterWithMetrics.SQLExecuter)
	assert.Equal(t, mMonitorService, dbConnectionPoolWithMetrics.SQLExecuterWithMetrics.monitorServiceInterface)

	assert.Equal(t, "postgres", dbConnectionPool.DriverName())
	err = dbConnectionPool.Ping(context.Background())
	require.NoError(t, err)
}

func Test_CloseConnectionPoolIfNeeded(t *testing.T) {
	db := dbtest.Postgres(t)
	defer db.Close()
	ctx := context.Background()

	t.Run("Logs NO-OP if the dbConnectionPool is nil", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := CloseConnectionPoolIfNeeded(ctx, nil)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(t, "NO-OP: attempting to close a DB connection pool but the object is nil", entries[0].Message)
	})

	t.Run("Logs NO-OP if the dbConnectionPool is already closed", func(t *testing.T) {
		dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
		require.NoError(t, err)
		err = dbConnectionPool.Close()
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err = CloseConnectionPoolIfNeeded(ctx, dbConnectionPool)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(t, "NO-OP: attempting to close a DB connection pool that was already closed", entries[0].Message)
	})
}

func Test_OpenDBConnectionPoolWithMetrics_MetricsRegistered(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Postgres(t)
	defer db.Close()

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	// Track all the metrics that should be registered
	var registeredMetrics []monitor.MetricTag
	mMonitorService.On("RegisterFunctionMetric",
		mock.AnythingOfType("monitor.FuncMetricType"),
		mock.MatchedBy(func(opts monitor.FuncMetricOptions) bool {
			// Capture the metric name for verification
			registeredMetrics = append(registeredMetrics, monitor.MetricTag(opts.Name))
			return opts.Function != nil && opts.Namespace == monitor.DefaultNamespace
		})).Times(8)

	dbConnectionPool, err := OpenDBConnectionPoolWithMetrics(ctx, db.DSN, mMonitorService)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Verify we got all expected metrics
	expectedMetrics := []monitor.MetricTag{
		monitor.DBMaxOpenConnectionsTag,
		monitor.DBInUseConnectionsTag,
		monitor.DBIdleConnectionsTag,
		monitor.DBWaitCountTotalTag,
		monitor.DBWaitDurationSecondsTotalTag,
		monitor.DBMaxIdleClosedTotalTag,
		monitor.DBMaxIdleTimeClosedTotalTag,
		monitor.DBMaxLifetimeClosedTotalTag,
	}

	for _, expectedMetric := range expectedMetrics {
		assert.Contains(t, registeredMetrics, expectedMetric,
			"Should have registered metric %s", expectedMetric)
	}
}

func Test_detectSchemaFromDBCP(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name           string
		datasourceName string
		expectedSchema string
	}{
		{"sdp schema", "postgres://user:password@somehost:5432/test?search_path=sdp_marwen-org&otherParam=false", "sdp_marwen-org"},
		{"tss schema", "postgres://user:password@somehost:5432/test?otherParam=false&search_path=tss", "tss"},
		{"admin schema", "postgres://user:password@somehost:5432/test?search_path=admin&otherParam=false", "admin"},
		{"unknown schema", "postgres://user:password@somehost:5432/test?search_path=unknown&otherParam=false", "unknown"},
		{"public schema", "postgres://user:password@somehost:5432/test?otherParam=false", "public"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbcp := &DBConnectionPoolImplementation{
				dataSourceName: tc.datasourceName,
			}
			result := detectSchemaFromDBCP(ctx, dbcp)
			assert.Equal(t, tc.expectedSchema, result)
		})
	}
}
