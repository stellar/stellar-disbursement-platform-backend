package dependencyinjection

import (
	"context"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

type DBConnectionPoolOptions struct {
	DatabaseURL    string
	MonitorService monitor.MonitorServiceInterface
	// Optional pool settings; when zero-valued, defaults are used.
	MaxOpenConns           int
	MaxIdleConns           int
	ConnMaxIdleTimeSeconds int
	ConnMaxLifetimeSeconds int
}

// OpenDBConnectionPool opens a connection pool in different ways based on the options
func openDBConnectionPool(ctx context.Context, dsn string, opts DBConnectionPoolOptions) (db.DBConnectionPool, error) {
	cfg := db.DefaultDBPoolConfig
	if opts.MaxOpenConns > 0 {
		cfg.MaxOpenConns = opts.MaxOpenConns
	}
	if opts.MaxIdleConns > 0 { // leave default unless explicitly set >0
		cfg.MaxIdleConns = opts.MaxIdleConns
	}
	if opts.ConnMaxIdleTimeSeconds > 0 {
		cfg.ConnMaxIdleTime = time.Duration(opts.ConnMaxIdleTimeSeconds) * time.Second
	}
	if opts.ConnMaxLifetimeSeconds > 0 {
		cfg.ConnMaxLifetime = time.Duration(opts.ConnMaxLifetimeSeconds) * time.Second
	}

	if opts.MonitorService == nil {
		return db.OpenDBConnectionPoolWithConfig(dsn, cfg)
	}
	return db.OpenDBConnectionPoolWithMetricsAndConfig(ctx, dsn, opts.MonitorService, cfg)
}
