package data

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func SetupModels(t *testing.T) *Models {
	t.Helper()

	pool := SetupDBCP(t)

	models, err := NewModels(pool)
	require.NoError(t, err)

	return models
}

func SetupDBCP(t *testing.T) db.DBConnectionPool {
	t.Helper()

	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	return pool
}
