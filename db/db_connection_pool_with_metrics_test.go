package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
)

func TestDBConnectionPoolWithMetrics_SqlxDB(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := monitorMocks.NewMockMonitorService(t)
	// We're registering 8 Function metrics for database connection pool metrics
	mMonitorService.On("RegisterFunctionMetric", mock.Anything, mock.Anything).Times(8)

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(ctx, dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	sqlxDB, err := dbConnectionPoolWithMetrics.SqlxDB(ctx)
	require.NoError(t, err)

	assert.IsType(t, &sqlx.DB{}, sqlxDB)
}

func TestDBConnectionPoolWithMetrics_SqlDB(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := monitorMocks.NewMockMonitorService(t)
	// We're registering 8 Function metrics for database connection pool metrics
	mMonitorService.On("RegisterFunctionMetric", mock.Anything, mock.Anything).Times(8)

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(ctx, dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	sqlDB, err := dbConnectionPoolWithMetrics.SqlDB(ctx)
	require.NoError(t, err)

	assert.IsType(t, &sql.DB{}, sqlDB)
}

func TestDBConnectionPoolWithMetrics_BeginTxx(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := monitorMocks.NewMockMonitorService(t)
	// We're registering 8 Function metrics for database connection pool metrics
	mMonitorService.On("RegisterFunctionMetric", mock.Anything, mock.Anything).Times(8)

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(ctx, dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	dbTxWithMetrics, err := dbConnectionPoolWithMetrics.BeginTxx(ctx, nil)

	// Defer a rollback in case anything fails.
	defer func() {
		err = dbTxWithMetrics.Rollback()
		require.Error(t, err, "not in transaction")
	}()
	require.NoError(t, err)

	assert.IsType(t, &DBTransactionWithMetrics{}, dbTxWithMetrics)

	err = dbTxWithMetrics.Commit()
	require.NoError(t, err)
}

func Test_NewDBConnectionPoolWithMetrics(t *testing.T) {
	ctx := context.Background()

	t.Run("successful creation", func(t *testing.T) {
		dbt := dbtest.Open(t)
		defer dbt.Close()
		dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		mMonitorService := monitorMocks.NewMockMonitorService(t)

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

		for _, tag := range expectedMetrics {
			mMonitorService.On("RegisterFunctionMetric",
				mock.AnythingOfType("monitor.FuncMetricType"),
				mock.MatchedBy(func(opts monitor.FuncMetricOptions) bool {
					return opts.Name == string(tag)
				})).Once()
		}

		dbPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(ctx, dbConnectionPool, mMonitorService)
		require.NoError(t, err)
		assert.NotNil(t, dbPoolWithMetrics)
	})

	t.Run("error in pool", func(t *testing.T) {
		mMonitorService := monitorMocks.NewMockMonitorService(t)

		_, err := NewDBConnectionPoolWithMetrics(ctx, nil, mMonitorService)
		assert.Error(t, err)
	})
}

func TestDBConnectionPoolWithMetrics_MetricsRegistration(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	expectedGaugeMetrics := []monitor.MetricTag{
		monitor.DBMaxOpenConnectionsTag,
		monitor.DBInUseConnectionsTag,
		monitor.DBIdleConnectionsTag,
	}

	expectedCounterMetrics := []monitor.MetricTag{
		monitor.DBWaitCountTotalTag,
		monitor.DBWaitDurationSecondsTotalTag,
		monitor.DBMaxIdleClosedTotalTag,
		monitor.DBMaxIdleTimeClosedTotalTag,
		monitor.DBMaxLifetimeClosedTotalTag,
	}

	// Expect gauge metrics
	for _, tag := range expectedGaugeMetrics {
		mMonitorService.On("RegisterFunctionMetric",
			monitor.FuncGaugeType,
			mock.MatchedBy(func(opts monitor.FuncMetricOptions) bool {
				return opts.Name == string(tag) &&
					opts.Namespace == monitor.DefaultNamespace &&
					opts.Labels["pool"] == "public" &&
					opts.Function != nil
			})).Once()
	}

	// Expect counter metrics
	for _, tag := range expectedCounterMetrics {
		mMonitorService.On("RegisterFunctionMetric",
			monitor.FuncCounterType,
			mock.MatchedBy(func(opts monitor.FuncMetricOptions) bool {
				return opts.Name == string(tag) &&
					opts.Namespace == monitor.DefaultNamespace &&
					opts.Labels["pool"] == "public" &&
					opts.Function != nil
			})).Once()
	}

	_, err = NewDBConnectionPoolWithMetrics(ctx, dbConnectionPool, mMonitorService)
	require.NoError(t, err)
}
