package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
)

func TestDBTransactionWithMetrics_Commit(t *testing.T) {
	t.Parallel()
	dbConnectionPool := openTestDBConnectionPool(t)

	mMonitorService := monitorMocks.NewMockMonitorService(t)

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
	t.Parallel()
	dbConnectionPool := openTestDBConnectionPool(t)

	mMonitorService := monitorMocks.NewMockMonitorService(t)

	ctx := context.Background()
	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)

	dbTransactionWithMetrics, err := NewDBTransactionWithMetrics(dbTx, mMonitorService)
	require.NoError(t, err)

	err = dbTransactionWithMetrics.Rollback()
	require.NoError(t, err)
}
