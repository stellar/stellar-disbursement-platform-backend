package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
)

const TSSDBConnectionPoolInstanceName = "tss_db_connection_pool_instance"

// NewTSSDBConnectionPool creates a new TSS db connection pool instance, or retrives a instance that
// was already created before.
func NewTSSDBConnectionPool(ctx context.Context, opts DBConnectionPoolOptions) (db.DBConnectionPool, error) {
	instanceName := TSSDBConnectionPoolInstanceName

	if instance, ok := GetInstance(instanceName); ok {
		if dbConnectionPoolInstance, ok2 := instance.(db.DBConnectionPool); ok2 {
			return dbConnectionPoolInstance, nil
		}
		return nil, fmt.Errorf("trying to cast TSS DBConnectionPool client for depencency injection")
	}

	log.Ctx(ctx).Infof("⚙️ Setting up TSS DBConnectionPool (withMetrics=%t)", opts.MonitorService != nil)
	tssDSN, err := router.GetDSNForTSS(opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("getting TSS database DSN: %w", err)
	}

	tssDBConnectionPool, err := openDBConnectionPool(ctx, tssDSN, opts.MonitorService)
	if err != nil {
		return nil, fmt.Errorf("opening TSS DB connection pool: %w", err)
	}

	SetInstance(instanceName, tssDBConnectionPool)
	return tssDBConnectionPool, nil
}
