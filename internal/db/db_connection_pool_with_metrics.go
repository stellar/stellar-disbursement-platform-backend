package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

func NewDBConnectionPoolWithMetrics(dbConnectionPool DBConnectionPool, monitorServiceInterface monitor.MonitorServiceInterface) (*DBConnectionPoolWithMetrics, error) {
	sqlExec, err := NewSQLExecuterWithMetrics(dbConnectionPool, monitorServiceInterface)
	if err != nil {
		return nil, fmt.Errorf("error creating SQLExecuterWithMetrics: %w", err)
	}

	return &DBConnectionPoolWithMetrics{
		dbConnectionPool:       dbConnectionPool,
		SQLExecuterWithMetrics: *sqlExec,
	}, nil
}

// DBConnectionPoolWithMetrics is a wrapper around sqlx.DB that implements DBConnectionPool with the monitoring service.
type DBConnectionPoolWithMetrics struct {
	dbConnectionPool DBConnectionPool
	SQLExecuterWithMetrics
}

// make sure *DBConnectionPoolWithMetrics implements DBConnectionPool:
var _ DBConnectionPool = (*DBConnectionPoolWithMetrics)(nil)

func (dbc *DBConnectionPoolWithMetrics) BeginTxx(ctx context.Context, opts *sql.TxOptions) (DBTransaction, error) {
	dbTransaction, err := dbc.dbConnectionPool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error starting a new transaction: %w", err)
	}

	return NewDBTransactionWithMetrics(dbTransaction, dbc.monitorServiceInterface)
}

func (dbc *DBConnectionPoolWithMetrics) Close() error {
	return dbc.dbConnectionPool.Close()
}

func (dbc *DBConnectionPoolWithMetrics) Ping() error {
	return dbc.dbConnectionPool.Ping()
}

func (dbc *DBConnectionPoolWithMetrics) SqlDB() *sql.DB {
	return dbc.dbConnectionPool.SqlDB()
}

func (dbc *DBConnectionPoolWithMetrics) SqlxDB() *sqlx.DB {
	return dbc.dbConnectionPool.SqlxDB()
}
