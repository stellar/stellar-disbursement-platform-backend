package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

func NewDBConnectionPoolWithMetrics(ctx context.Context, dbConnectionPool DBConnectionPool, monitorServiceInterface monitor.MonitorServiceInterface) (*DBConnectionPoolWithMetrics, error) {
	sqlExec, err := NewSQLExecuterWithMetrics(dbConnectionPool, monitorServiceInterface)
	if err != nil {
		return nil, fmt.Errorf("error creating SQLExecuterWithMetrics: %w", err)
	}

	registerMetrics(ctx, dbConnectionPool, monitorServiceInterface)

	return &DBConnectionPoolWithMetrics{
		dbConnectionPool:       dbConnectionPool,
		SQLExecuterWithMetrics: *sqlExec,
	}, nil
}

func registerMetrics(ctx context.Context, dbConnectionPool DBConnectionPool, monitorServiceInterface monitor.MonitorServiceInterface) {
	labels := map[string]string{
		"pool": detectSchemaFromDBCP(ctx, dbConnectionPool),
	}

	db, err := dbConnectionPool.SqlDB(ctx)
	if err != nil {
		log.Ctx(ctx).Errorf("Error getting SQL DB for MaxOpenDBConns metric: %s", err)
		return
	}

	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncGaugeType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBMaxOpenConnectionsTag),
			Help:   "Maximum number of open connections to the database",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().MaxOpenConnections)
			},
		})

	// Pool Status
	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncGaugeType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBInUseConnectionsTag),
			Help:   "The number of established connections both in use and idle",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().InUse)
			},
		})

	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncGaugeType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBIdleConnectionsTag),
			Help:   "The number of idle connections",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().Idle)
			},
		})

	// Counters
	// WaitCount
	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncCounterType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBWaitCountTotalTag),
			Help:   "The total number of connections waited for",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().WaitCount)
			},
		})
	// WaitDuration
	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncCounterType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBWaitDurationSecondsTotalTag),
			Help:   "The total time blocked waiting for a new connection",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().WaitDuration.Seconds())
			},
		})

	// MaxIdleClosed
	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncCounterType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBMaxIdleClosedTotalTag),
			Help:   "The total number of connections closed due to SetMaxIdleConns",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().MaxIdleClosed)
			},
		})

	// MaxIdleTimeClosed
	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncCounterType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBMaxIdleTimeClosedTotalTag),
			Help:   "The total number of connections closed due to SetConnMaxIdleTime",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().MaxIdleTimeClosed)
			},
		})

	// MaxLifetimeClosed
	monitorServiceInterface.RegisterFunctionMetric(
		monitor.FuncCounterType,
		monitor.FuncMetricOptions{
			Namespace: monitor.DefaultNamespace, Subservice: string(monitor.DBSubservice), Name: string(monitor.DBMaxLifetimeClosedTotalTag),
			Help:   "The total number of connections closed due to SetConnMaxLifetime",
			Labels: labels,
			Function: func() float64 {
				return float64(db.Stats().MaxLifetimeClosed)
			},
		})
}

// DBConnectionPoolWithMetrics is a wrapper around sqlx.DB that implements DBConnectionPool with the monitoring service.
type DBConnectionPoolWithMetrics struct {
	dbConnectionPool DBConnectionPool
	SQLExecuterWithMetrics
}

// make sure *DBConnectionPoolWithMetrics implements DBConnectionPool:
var _ DBConnectionPool = (*DBConnectionPoolWithMetrics)(nil)

func (dbc *DBConnectionPoolWithMetrics) BeginTxx(ctx context.Context, opts *sql.TxOptions) (DBTransaction, error) {
	dbTransaction, err := dbc.dbConnectionPool.BeginTxx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("error starting a new transaction: %w", err)
	}

	return NewDBTransactionWithMetrics(dbTransaction, dbc.monitorServiceInterface)
}

func (dbc *DBConnectionPoolWithMetrics) Close() error {
	return dbc.dbConnectionPool.Close()
}

func (dbc *DBConnectionPoolWithMetrics) Ping(ctx context.Context) error {
	return dbc.dbConnectionPool.Ping(ctx)
}

func (dbc *DBConnectionPoolWithMetrics) SqlDB(ctx context.Context) (*sql.DB, error) {
	return dbc.dbConnectionPool.SqlDB(ctx)
}

func (dbc *DBConnectionPoolWithMetrics) SqlxDB(ctx context.Context) (*sqlx.DB, error) {
	return dbc.dbConnectionPool.SqlxDB(ctx)
}

func (dbc *DBConnectionPoolWithMetrics) DSN(ctx context.Context) (string, error) {
	return dbc.dbConnectionPool.DSN(ctx)
}
