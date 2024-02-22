package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
)

const TSSDBConnectionPoolInstanceName = "tss_db_connection_pool_instance"

type TSSDBConnectionPoolOptions struct {
	DatabaseURL string
}

// NewTSSDBConnectionPool creates a new TSS db connection pool instance, or retrives a instance that
// was already created before.
func NewTSSDBConnectionPool(ctx context.Context, opts TSSDBConnectionPoolOptions) (db.DBConnectionPool, error) {
	instanceName := TSSDBConnectionPoolInstanceName

	if instance, ok := GetInstance(instanceName); ok {
		if dbConnectionPoolInstance, ok2 := instance.(db.DBConnectionPool); ok2 {
			return dbConnectionPoolInstance, nil
		}
		return nil, fmt.Errorf("trying to cast TSS DBConnectionPool client for depencency injection")
	}

	log.Ctx(ctx).Info("⚙️ Setting up TSS DBConnectionPool")
	tssDBConnectionPool, err := router.GetDBForTSSSchema(opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("getting TSS DBConnectionPool: %w", err)
	}

	SetInstance(instanceName, tssDBConnectionPool)
	return tssDBConnectionPool, nil
}
