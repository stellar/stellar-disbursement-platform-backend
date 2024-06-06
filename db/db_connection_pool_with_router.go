package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ConnectionPoolWithRouter implements the DBConnectionPool interface
type ConnectionPoolWithRouter struct {
	SQLExecutorWithRouter
}

// NewConnectionPoolWithRouter creates a new ConnectionPoolWithRouter
func NewConnectionPoolWithRouter(dataSourceRouter DataSourceRouter) (*ConnectionPoolWithRouter, error) {
	sqlExecutor, err := NewSQLExecutorWithRouter(dataSourceRouter)
	if err != nil {
		return nil, fmt.Errorf("creating new sqlExecutor for connection pool with router: %w", err)
	}
	return &ConnectionPoolWithRouter{
		SQLExecutorWithRouter: *sqlExecutor,
	}, nil
}

func (m ConnectionPoolWithRouter) BeginTxx(ctx context.Context, opts *sql.TxOptions) (DBTransaction, error) {
	dbcpl, err := m.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting data source from context in BeginTxx: %w", err)
	}
	return dbcpl.BeginTxx(ctx, opts)
}

func (m ConnectionPoolWithRouter) Close() error {
	dbcpls, err := m.dataSourceRouter.GetAllDataSources()
	if err != nil {
		return fmt.Errorf("getting all data sources in Close: %w", err)
	}
	if len(dbcpls) == 0 {
		return fmt.Errorf("no data sources found in Close")
	}
	for _, dbcpl := range dbcpls {
		err = dbcpl.Close()
		if err != nil {
			return fmt.Errorf("closing data source in Close: %w", err)
		}
	}
	return nil
}

func (m ConnectionPoolWithRouter) Ping(ctx context.Context) error {
	dbcpl, err := m.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return fmt.Errorf("getting data source from context in Ping: %w", err)
	}
	return dbcpl.Ping(ctx)
}

func (m ConnectionPoolWithRouter) SqlDB(ctx context.Context) (*sql.DB, error) {
	dbcpl, err := m.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting data source from context in SqlDB: %w", err)
	}
	return dbcpl.SqlDB(ctx)
}

func (m ConnectionPoolWithRouter) SqlxDB(ctx context.Context) (*sqlx.DB, error) {
	dbcpl, err := m.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting data source from context in SqlxDB: %w", err)
	}
	return dbcpl.SqlxDB(ctx)
}

func (m ConnectionPoolWithRouter) DSN(ctx context.Context) (string, error) {
	dbcpl, err := m.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return "", fmt.Errorf("getting data source from context in DSN: %w", err)
	}
	return dbcpl.DSN(ctx)
}

// make sure *ConnectionPoolWithRouter implements DBConnectionPool:
var _ DBConnectionPool = (*ConnectionPoolWithRouter)(nil)
