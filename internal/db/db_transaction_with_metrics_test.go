package db

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stretchr/testify/require"
)

func TestDBTransactionWithMetrics_Commit(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	ctx := context.Background()
	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	// Defer a rollback in case anything fails.
	defer func() {
		err = dbTx.Rollback()
		require.Error(t, err, "not in transaction")
	}()

	dbTransactionWithMetrics, err := NewDBTransactionWithMetrics(dbTx, mMonitorService)
	require.NoError(t, err)

	err = dbTransactionWithMetrics.Commit()
	require.NoError(t, err)
}

func TestDBTransactionWithMetrics_Rollback(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	ctx := context.Background()
	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)

	dbTransactionWithMetrics, err := NewDBTransactionWithMetrics(dbTx, mMonitorService)
	require.NoError(t, err)

	err = dbTransactionWithMetrics.Rollback()
	require.NoError(t, err)
}
