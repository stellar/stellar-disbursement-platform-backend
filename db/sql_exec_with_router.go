package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type DataSourceRouter interface {
	GetDataSource(ctx context.Context) (DBConnectionPool, error)
	GetAllDataSources() ([]DBConnectionPool, error)
	AnyDataSource() (DBConnectionPool, error)
}

type SQLExecutorWithRouter struct {
	dataSourceRouter DataSourceRouter
}

func NewSQLExecutorWithRouter(router DataSourceRouter) (*SQLExecutorWithRouter, error) {
	if router == nil {
		return nil, fmt.Errorf("router is nil in NewSQLExecutorWithRouter")
	}
	return &SQLExecutorWithRouter{
		dataSourceRouter: router,
	}, nil
}

func (s SQLExecutorWithRouter) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	dbcpl, err := s.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return fmt.Errorf("getting data source from context in GetContext: %w", err)
	}
	return dbcpl.GetContext(ctx, dest, query, args...)
}

func (s SQLExecutorWithRouter) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	dbcpl, err := s.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return fmt.Errorf("getting data source from context in SelectContext: %w", err)
	}

	return dbcpl.SelectContext(ctx, dest, query, args...)
}

func (s SQLExecutorWithRouter) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	dbcpl, err := s.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting data source from context in ExecContext: %w", err)
	}

	return dbcpl.ExecContext(ctx, query, args...)
}

func (s SQLExecutorWithRouter) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	dbcpl, err := s.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting data source from context in QueryContext: %w", err)
	}

	return dbcpl.QueryContext(ctx, query, args...)
}

func (s SQLExecutorWithRouter) QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	dbcpl, err := s.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting data source from context in QueryxContext: %w", err)
	}

	return dbcpl.QueryxContext(ctx, query, args...)
}

func (s SQLExecutorWithRouter) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	dbcpl, err := s.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting data source from context in PrepareContext: %w", err)
	}

	return dbcpl.PrepareContext(ctx, query)
}

func (s SQLExecutorWithRouter) QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row {
	dbcpl, err := s.dataSourceRouter.GetDataSource(ctx)
	if err != nil {
		return nil
	}

	return dbcpl.QueryRowxContext(ctx, query, args...)
}

func (s SQLExecutorWithRouter) Rebind(query string) string {
	dbcp, err := s.dataSourceRouter.AnyDataSource()
	if err != nil {
		return sqlx.Rebind(sqlx.DOLLAR, query)
	}
	return dbcp.Rebind(query)
}

func (m SQLExecutorWithRouter) DriverName() string {
	dbcp, err := m.dataSourceRouter.AnyDataSource()
	if err != nil {
		return ""
	}
	return dbcp.DriverName()
}

// make sure *SQLExecutorWithRouter implements SQLExecuter:
var _ SQLExecuter = (*SQLExecutorWithRouter)(nil)
