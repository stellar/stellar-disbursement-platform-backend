package db

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"

	"github.com/stretchr/testify/require"
)

func openTestDBConnectionPool(t *testing.T) DBConnectionPool {
	t.Helper()

	dbt := dbtest.Open(t)
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)

	t.Cleanup(func() {
		dbConnectionPool.Close()
	})

	return dbConnectionPool
}
