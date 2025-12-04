package dependencyinjection

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const MtnDBConnectionPoolInstanceName = "mtn_db_connection_pool_instance"

// NewMtnDBConnectionPool creates a new multitenant db connection pool instance, or retrives an instance that was
// already created before. The multitenant db connection pool is used to connect to the tenant's databases based on the
// tenant found in the context.
func NewMtnDBConnectionPool(ctx context.Context, opts DBConnectionPoolOptions) (db.DBConnectionPool, error) {
	// If there is already an instance of the service, we return the same instance
	instanceName := MtnDBConnectionPoolInstanceName
	if instance, ok := GetInstance(instanceName); ok {
		if dbConnectionPoolInstance, ok2 := instance.(db.DBConnectionPool); ok2 {
			return dbConnectionPoolInstance, nil
		}
		return nil, fmt.Errorf("trying to cast multitenant DBConnectionPool for depencency injection")
	}

	adminDBConnectionPool, err := NewAdminDBConnectionPool(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("opening Admin DB connection pool from NewMtnDBConnectionPool: %w", err)
	}

	log.Ctx(ctx).Info("⚙️ Setting up Multi-tenant DBConnectionPool")
	tm := tenant.NewManager(tenant.WithDatabase(adminDBConnectionPool))

	// Build pool config for tenant pools
	cfg := db.DefaultDBPoolConfig
	if opts.MaxOpenConns > 0 {
		cfg.MaxOpenConns = opts.MaxOpenConns
	}
	if opts.MaxIdleConns >= 0 {
		cfg.MaxIdleConns = opts.MaxIdleConns
	}
	if opts.ConnMaxIdleTimeSeconds > 0 {
		cfg.ConnMaxIdleTime = time.Duration(opts.ConnMaxIdleTimeSeconds) * time.Second
	}
	if opts.ConnMaxLifetimeSeconds > 0 {
		cfg.ConnMaxLifetime = time.Duration(opts.ConnMaxLifetimeSeconds) * time.Second
	}

	tr := tenant.NewMultiTenantDataSourceRouter(tm).WithMonitoring(opts.MonitorService).WithPoolConfig(cfg)
	mtnDBConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
	if err != nil {
		return nil, fmt.Errorf("opening Mtn DB connection pool: %w", err)
	}

	SetInstance(instanceName, mtnDBConnectionPool)
	return mtnDBConnectionPool, nil
}
