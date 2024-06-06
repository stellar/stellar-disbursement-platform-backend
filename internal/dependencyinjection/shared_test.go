package dependencyinjection

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
)

func Test_openDBConnectionPool(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	t.Run("handle options without metrics", func(t *testing.T) {
		dbConnectionPool, err := openDBConnectionPool(dbt.DSN, nil)
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		require.IsType(t, &db.DBConnectionPoolImplementation{}, dbConnectionPool)
	})

	t.Run("handle options with metrics", func(t *testing.T) {
		mMonitorService := monitorMocks.NewMockMonitorService(t)
		dbConnectionPool, err := openDBConnectionPool(dbt.DSN, mMonitorService)
		require.NoError(t, err)
		defer dbConnectionPool.Close()

		require.IsType(t, &db.DBConnectionPoolWithMetrics{}, dbConnectionPool)
	})
}
