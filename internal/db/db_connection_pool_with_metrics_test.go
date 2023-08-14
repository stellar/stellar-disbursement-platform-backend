package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBConnectionPoolWithMetrics_SqlxDB(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	sqlxDB := dbConnectionPoolWithMetrics.SqlxDB()

	assert.IsType(t, &sqlx.DB{}, sqlxDB)
}

func TestDBConnectionPoolWithMetrics_SqlDB(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	dbConnectionPoolWithMetrics, err := NewDBConnectionPoolWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	sqlDB := dbConnectionPoolWithMetrics.SqlDB()

	assert.IsType(t, &sql.DB{}, sqlDB)
}

func TestDBConnectionPoolWithMetrics_BeginTxx(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

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
