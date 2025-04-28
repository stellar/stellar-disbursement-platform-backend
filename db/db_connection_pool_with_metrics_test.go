package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
)

func TestDBConnectionPoolWithMetrics_SqlxDB(t *testing.T) {
	t.Parallel()
	dbConnectionPool := openTestDBConnectionPool(t)

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()
	sqlxDB, err := dbConnectionPoolWithMetrics.SqlxDB(ctx)
	require.NoError(t, err)

	assert.IsType(t, &sqlx.DB{}, sqlxDB)
}

func TestDBConnectionPoolWithMetrics_SqlDB(t *testing.T) {
	t.Parallel()
	dbConnectionPool := openTestDBConnectionPool(t)

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()
	sqlDB, err := dbConnectionPoolWithMetrics.SqlDB(ctx)
	require.NoError(t, err)

	assert.IsType(t, &sql.DB{}, sqlDB)
}

func TestDBConnectionPoolWithMetrics_BeginTxx(t *testing.T) {
	t.Parallel()
	dbConnectionPool := openTestDBConnectionPool(t)

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()
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
