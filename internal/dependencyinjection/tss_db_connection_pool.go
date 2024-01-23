package dependencyinjection

import (
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
)

const tssDBConnectionPoolInstanceName = "tss_db_connection_pool_instance"

type TSSDBConnectionPoolOptions struct {
	DatabaseURL string
}

// NewTSSDBConnectionPool creates a new TSS db connection pool instance, or retrives a instance that
// was already created before.
func NewTSSDBConnectionPool(opts TSSDBConnectionPoolOptions) (db.DBConnectionPool, error) {
	// If there is already an instance of the service, we return the same instance
	instanceName := tssDBConnectionPoolInstanceName
	if instance, ok := dependenciesStoreMap[instanceName]; ok {
		if dbConnectionPoolInstance, ok := instance.(db.DBConnectionPool); ok {
			return dbConnectionPoolInstance, nil
		}
		return nil, fmt.Errorf("trying to cast TSS DBConnectionPool client for depencency injection")
	}

	log.Info("⚙️ Setting up TSS DBConnectionPool")
	tssDNS, err := router.GetDNSForTSS(opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("getting TSS database DNS: %w", err)
	}
	tssDBConnectionPool, err := db.OpenDBConnectionPool(tssDNS)
	if err != nil {
		return nil, fmt.Errorf("openong TSS DB connection pool: %w", err)
	}

	setInstance(instanceName, tssDBConnectionPool)
	return tssDBConnectionPool, nil
}
