package router

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var (
	ErrTenantNotFoundInContext = errors.New("tenant not found in context")
	ErrNoDataSourcesAvailable  = errors.New("no data sources are available")
)

type tenantContextKey struct{}

type MultiTenantDataSourceRouter struct {
	dataSources   sync.Map
	tenantManager *tenant.Manager
}

func NewMultiTenantDataSourceRouter(tenantManager *tenant.Manager) *MultiTenantDataSourceRouter {
	return &MultiTenantDataSourceRouter{
		tenantManager: tenantManager,
	}
}

func (m *MultiTenantDataSourceRouter) GetDataSource(ctx context.Context) (db.DBConnectionPool, error) {
	tenant, ok := GetTenantFromContext(ctx)
	if !ok {
		return nil, ErrTenantNotFoundInContext
	}

	return m.GetDataSourceForTenant(ctx, *tenant)
}

// GetDataSourceForTenant returns the database connection pool for the given tenant if it exists, otherwise create a new one.
func (m *MultiTenantDataSourceRouter) GetDataSourceForTenant(ctx context.Context, tenant tenant.Tenant) (db.DBConnectionPool, error) {
	value, exists := m.dataSources.Load(tenant.ID)
	if exists {
		return value.(db.DBConnectionPool), nil
	}

	u, err := m.tenantManager.GetDSNForTenant(ctx, tenant.Name)
	if err != nil || u == "" {
		return nil, fmt.Errorf("getting database DSN for tenant %s: %w", tenant.ID, err)
	}

	dbcp, err := db.OpenDBConnectionPool(u)
	if err != nil {
		return nil, fmt.Errorf("opening database connection pool for tenant %s: %w", tenant.ID, err)
	}

	// Store the new pool, but if another goroutine already stored a pool for this tenant,
	// then use the existing one and close the newly created one.
	actualValue, loaded := m.dataSources.LoadOrStore(tenant.ID, dbcp)
	if loaded {
		dbcp.Close() // Close the newly created pool if we're not using it
		return actualValue.(db.DBConnectionPool), nil
	}

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

// SetTenantInContext stores the tenant information in the context.
func SetTenantInContext(ctx context.Context, tenant *tenant.Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenant)
}

// GetTenantFromContext retrieves the tenant information from the context.
func GetTenantFromContext(ctx context.Context) (*tenant.Tenant, bool) {
	tenant, ok := ctx.Value(tenantContextKey{}).(*tenant.Tenant)
	return tenant, ok
}

// make sure *MultiTenantDataSourceRouter implements DataSourceRouter:
var _ db.DataSourceRouter = (*MultiTenantDataSourceRouter)(nil)
