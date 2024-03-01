package dependencyinjection

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stretchr/testify/require"
)

func Test_openDBConnectionPool(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	mMonitorService := &monitor.MockMonitorService{}

	t.Run("handle options without metrics", func(t *testing.T) {
		dbConnectionPool, err := openDBConnectionPool(dbt.DSN, nil)
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		require.IsType(t, &db.DBConnectionPoolImplementation{}, dbConnectionPool)
	})

	t.Run("handle options with metrics", func(t *testing.T) {
		dbConnectionPool, err := openDBConnectionPool(dbt.DSN, mMonitorService)
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		require.IsType(t, &db.DBConnectionPoolWithMetrics{}, dbConnectionPool)
	})
}
