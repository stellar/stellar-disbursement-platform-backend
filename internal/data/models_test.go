package data

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/require"
)

func Test_NewModels(t *testing.T) {
	t.Run("returns error if the db connection pool is nil", func(t *testing.T) {
		models, err := NewModels(nil)
		require.Nil(t, models)
		require.EqualError(t, err, "dbConnectionPool is required for NewModels")
	})

	t.Run("returns model successfully 🎉", func(t *testing.T) {
		dbt := dbtest.Open(t)
		defer dbt.Close()

		dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		models, err := NewModels(dbConnectionPool)
		require.NoError(t, err)
		require.NotNil(t, models)
	})
}
