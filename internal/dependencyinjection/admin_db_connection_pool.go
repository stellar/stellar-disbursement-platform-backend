package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
)

const AdminDBConnectionPoolInstanceName = "admin_db_connection_pool_instance"

// NewAdminDBConnectionPool creates a new admin db connection pool instance, or retrives an instance that was already
// created before.
func NewAdminDBConnectionPool(ctx context.Context, opts DBConnectionPoolOptions) (db.DBConnectionPool, error) {
	// If there is already an instance of the service, we return the same instance
	instanceName := AdminDBConnectionPoolInstanceName
	if instance, ok := GetInstance(instanceName); ok {
		if dbConnectionPoolInstance, ok := instance.(db.DBConnectionPool); ok {
			return dbConnectionPoolInstance, nil
		}
		return nil, fmt.Errorf("trying to cast Admin DBConnectionPool for depencency injection")
	}

	log.Ctx(ctx).Infof("⚙️ Setting up Admin DBConnectionPool (withMetrics=%t)", opts.MonitorService != nil)
	adminDNS, err := router.GetDNSForAdmin(opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("getting Admin database DNS: %w", err)
	}

	dbConnectionPool, err := openDBConnectionPool(adminDNS, opts.MonitorService)
	if err != nil {
		return nil, fmt.Errorf("opening Admin DB connection pool: %w", err)
	}

	SetInstance(instanceName, dbConnectionPool)
	return dbConnectionPool, nil
}
