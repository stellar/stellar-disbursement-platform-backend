package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

const dbConnectionPoolInstanceName = "db_connection_pool_instance"

type DBConnectionPoolOptions struct {
	DatabaseURL string
}

// NewDBConnectionPool creates a new db connection pool instance, or retrives an instance that was already created
// before.
func NewDBConnectionPool(ctx context.Context, opts DBConnectionPoolOptions) (db.DBConnectionPool, error) {
	// If there is already an instance of the service, we return the same instance
	instanceName := dbConnectionPoolInstanceName
	if instance, ok := dependenciesStoreMap[instanceName]; ok {
		if dbConnectionPoolInstance, ok := instance.(db.DBConnectionPool); ok {
			return dbConnectionPoolInstance, nil
		}
		return nil, fmt.Errorf("trying to cast DBConnectionPool client for depencency injection")
	}

	log.Ctx(ctx).Info("⚙️ Setting up DBConnectionPool")
	dbConnectionPool, err := db.OpenDBConnectionPool(opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening DB connection pool: %w", err)
	}

	SetInstance(instanceName, dbConnectionPool)
	return dbConnectionPool, nil
}
