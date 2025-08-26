package dependencyinjection

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

type DBConnectionPoolOptions struct {
	DatabaseURL    string
	MonitorService monitor.MonitorServiceInterface
}

// OpenDBConnectionPool opens a connection pool in different ways based on the options
func openDBConnectionPool(ctx context.Context, dsn string, metricsService monitor.MonitorServiceInterface) (db.DBConnectionPool, error) {
	if metricsService == nil {
		return db.OpenDBConnectionPool(dsn)
	}
	return db.OpenDBConnectionPoolWithMetrics(ctx, dsn, metricsService)
}
