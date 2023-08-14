package db

import (
	"testing"

	"github.com/stellar/go/support/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_OpenDBConnectionPool(t *testing.T) {
	db := dbtest.Postgres(t)
	defer db.Close()

	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	assert.Equal(t, "postgres", dbConnectionPool.DriverName())

	err = dbConnectionPool.Ping()
	require.NoError(t, err)
}

func TestOpen_OpenDBConnectionPoolWithMetrics(t *testing.T) {
	db := dbtest.Postgres(t)
	defer db.Close()

	mMonitorService := &monitor.MockMonitorService{}

	dbConnectionPoolWithMetrics, err := OpenDBConnectionPoolWithMetrics(db.DSN, mMonitorService)
	require.NoError(t, err)
	defer dbConnectionPoolWithMetrics.Close()

	assert.Equal(t, "postgres", dbConnectionPoolWithMetrics.DriverName())

	err = dbConnectionPoolWithMetrics.Ping()
	require.NoError(t, err)
}
