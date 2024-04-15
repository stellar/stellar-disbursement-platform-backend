package tenant

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

var ErrNoDataSourcesAvailable = errors.New("no data sources are available")

type MultiTenantDataSourceRouter struct {
	dataSources   sync.Map
	tenantManager ManagerInterface
	mu            sync.Mutex
}

func NewMultiTenantDataSourceRouter(tenantManager ManagerInterface) *MultiTenantDataSourceRouter {
	return &MultiTenantDataSourceRouter{
		tenantManager: tenantManager,
	}
}

func (m *MultiTenantDataSourceRouter) GetDataSource(ctx context.Context) (db.DBConnectionPool, error) {
	currentTenant, err := GetTenantFromContext(ctx)
	if err != nil {
		return nil, ErrTenantNotFoundInContext
	}

	return m.GetDataSourceForTenant(ctx, *currentTenant)
}

// GetDataSourceForTenant returns the database connection pool for the given tenant if it exists, otherwise create a new one.
func (m *MultiTenantDataSourceRouter) GetDataSourceForTenant(
	ctx context.Context,
	currentTenant Tenant,
) (db.DBConnectionPool, error) {
	value, exists := m.dataSources.Load(currentTenant.ID)
	if exists {
		return value.(db.DBConnectionPool), nil
	}

	return m.getOrCreateDataSourceForTenantWithLock(ctx, currentTenant)
}

func (m *MultiTenantDataSourceRouter) getOrCreateDataSourceForTenantWithLock(
	ctx context.Context,
	currentTenant Tenant,
) (db.DBConnectionPool, error) {
	// Acquire the lock only if the data source was not found.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Fetch in case the data source was created by another goroutine.
	value, exists := m.dataSources.Load(currentTenant.ID)
	if exists {
		return value.(db.DBConnectionPool), nil
	}

	// Create the connection pool for this tenant
	u, err := m.tenantManager.GetDSNForTenant(ctx, currentTenant.Name)
	if err != nil || u == "" {
		return nil, fmt.Errorf("getting database DSN for tenant %s: %w", currentTenant.ID, err)
	}

	dbcp, err := db.OpenDBConnectionPool(u)
	if err != nil {
		return nil, fmt.Errorf("opening database connection pool for tenant %s: %w", currentTenant.ID, err)
	}

	// Store the new connection pool in the map.
	m.dataSources.Store(currentTenant.ID, dbcp)

	return dbcp, nil
}

// GetAllDataSources returns all the database connection pools.
func (m *MultiTenantDataSourceRouter) GetAllDataSources() ([]db.DBConnectionPool, error) {
	var pools []db.DBConnectionPool
	m.dataSources.Range(func(_, value interface{}) bool {
		pools = append(pools, value.(db.DBConnectionPool))
		return true
	})
	return pools, nil
}

// AnyDataSource returns any database connection pool.
func (m *MultiTenantDataSourceRouter) AnyDataSource() (db.DBConnectionPool, error) {
	var anyDBCP db.DBConnectionPool
	var found bool
	m.dataSources.Range(func(_, value interface{}) bool {
		anyDBCP = value.(db.DBConnectionPool)
		found = true
		return false
	})
	if !found {
		return nil, ErrNoDataSourcesAvailable
	}
	return anyDBCP, nil
}

// make sure *MultiTenantDataSourceRouter implements DataSourceRouter:
var _ db.DataSourceRouter = (*MultiTenantDataSourceRouter)(nil)
