package db

import (
	"context"
	"testing"

	"github.com/stellar/go/support/db/dbtest"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
)

func TestOpen_OpenDBConnectionPool(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	db := dbtest.Postgres(t)
	defer db.Close()

	mMonitorService := monitorMocks.NewMockMonitorService(t)
	dbConnectionPool, err := OpenDBConnectionPoolWithMetrics(db.DSN, mMonitorService)
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
	t.Parallel()
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
