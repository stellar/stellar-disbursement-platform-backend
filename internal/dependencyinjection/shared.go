package dependencyinjection

import (
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

type DBConnectionPoolOptions struct {
	DatabaseURL    string
	MonitorService monitor.MonitorServiceInterface
}

func openDBConnectionPool(dns string, metricsService monitor.MonitorServiceInterface) (db.DBConnectionPool, error) {
	if metricsService == nil {
		return db.OpenDBConnectionPool(dns)
	}
	return db.OpenDBConnectionPoolWithMetrics(dns, metricsService)
}
