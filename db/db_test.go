package db

import (
	"testing"

	"github.com/stellar/go/support/db/dbtest"
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
