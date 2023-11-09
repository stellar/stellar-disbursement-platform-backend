package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

const (
	MaxDBConnIdleTime = 10 * time.Second
	MaxOpenDBConns    = 30
)

// DBConnectionPoolFromSqlDB returns a new DBConnectionPool wrapper for a PRE-EXISTING *sql.DB. The driverName of the
// original database is required for named query support. ATTENTION: this will not start a new connection pool, just
// create a wrap aroung the pre-existing connection pool.
func DBConnectionPoolFromSqlDB(sqlDB *sql.DB, driverName string) DBConnectionPool {
	return &DBConnectionPoolImplementation{DB: sqlx.NewDb(sqlDB, driverName)}
}

// DBConnectionPool is an interface that wraps the sqlx.DB structs methods and includes the RunInTransaction helper.
type DBConnectionPool interface {
	SQLExecuter
	BeginTxx(ctx context.Context, opts *sql.TxOptions) (DBTransaction, error)
	Close() error
	Ping(ctx context.Context) error
	SqlDB(ctx context.Context) (*sql.DB, error)
	SqlxDB(ctx context.Context) (*sqlx.DB, error)
	DSN(ctx context.Context) (string, error)
}

// DBConnectionPoolImplementation is a wrapper around sqlx.DB that implements DBConnectionPool.
type DBConnectionPoolImplementation struct {
	*sqlx.DB
	dataSourceName string
}

func (db *DBConnectionPoolImplementation) BeginTxx(ctx context.Context, opts *sql.TxOptions) (DBTransaction, error) {
	return db.DB.BeginTxx(ctx, opts)
}

func (db *DBConnectionPoolImplementation) Ping(ctx context.Context) error {
	return db.DB.PingContext(ctx)
}

func (db *DBConnectionPoolImplementation) SqlDB(ctx context.Context) (*sql.DB, error) {
	if db.DB == nil || db.DB.DB == nil {
		return nil, fmt.Errorf("sql.DB is not initialized")
	}
	return db.DB.DB, nil
}

func (db *DBConnectionPoolImplementation) SqlxDB(ctx context.Context) (*sqlx.DB, error) {
	if db.DB == nil {
		return nil, fmt.Errorf("sqlx.DB is not initialized")
	}
	return db.DB, nil
}

func (db *DBConnectionPoolImplementation) DSN(ctx context.Context) (string, error) {
	return db.dataSourceName, nil
}

// RunInTransactionWithResult runs the given atomic function in an atomic database transaction and returns a result and
// an error. Boilerplate code for database transactions.
func RunInTransactionWithResult[T any](ctx context.Context, dbConnectionPool DBConnectionPool, opts *sql.TxOptions, atomicFunction func(dbTx DBTransaction) (T, error)) (result T, err error) {
	dbTx, err := dbConnectionPool.BeginTxx(ctx, opts)
	if err != nil {
		return *new(T), fmt.Errorf("creating db transaction for RunInTransactionWithResult: %w", err)
	}

	defer func() {
		DBTxRollback(ctx, dbTx, err, "rolling back transaction due to error")
	}()

	result, err = atomicFunction(dbTx)
	if err != nil {
		return *new(T), fmt.Errorf("running atomic function in RunInTransactionWithResult: %w", err)
	}

	err = dbTx.Commit()
	if err != nil {
		return *new(T), fmt.Errorf("committing transaction in RunInTransactionWithResult: %w", err)
	}

	return result, nil
}

// RunInTransaction runs the given atomic function in an atomic database transaction and returns an error. Boilerplate
// code for database transactions.
func RunInTransaction(ctx context.Context, dbConnectionPool DBConnectionPool, opts *sql.TxOptions, atomicFunction func(dbTx DBTransaction) error) error {
	// wrap the atomic function with a function that returns nil and an error so we can call RunInTransactionWithResult
	wrappedFunction := func(dbTx DBTransaction) (interface{}, error) {
		return nil, atomicFunction(dbTx)
	}

	_, err := RunInTransactionWithResult(ctx, dbConnectionPool, opts, wrappedFunction)
	return err
}

// make sure *DBConnectionPoolImplementation implements DBConnectionPool:
var _ DBConnectionPool = (*DBConnectionPoolImplementation)(nil)

// DBTransaction is an interface that wraps the sqlx.Tx structs methods.
type DBTransaction interface {
	SQLExecuter
	Rollback() error
	Commit() error
}

// make sure *sqlx.Tx implements DBTransaction:
var _ DBTransaction = (*sqlx.Tx)(nil)

// SQLExecuter is an interface that wraps the *sqlx.DB and *sqlx.Tx structs methods.
type SQLExecuter interface {
	DriverName() string
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	sqlx.PreparerContext
	sqlx.QueryerContext
	Rebind(query string) string
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

// make sure *sqlx.DB implements SQLExecuter:
var _ SQLExecuter = (*sqlx.DB)(nil)

// make sure DBConnectionPool implements SQLExecuter:
var _ SQLExecuter = (DBConnectionPool)(nil)

// make sure *sqlx.Tx implements SQLExecuter:
var _ SQLExecuter = (*sqlx.Tx)(nil)

// make sure DBTransaction implements SQLExecuter:
var _ SQLExecuter = (DBTransaction)(nil)

// DBTxRollback rolls back the transaction if there is an error.
func DBTxRollback(ctx context.Context, dbTx DBTransaction, err error, logMessage string) {
	if err != nil {
		log.Ctx(ctx).Errorf("%s: %s", logMessage, err.Error())
		errRollBack := dbTx.Rollback()
		if errRollBack != nil {
			log.Ctx(ctx).Errorf("error in database transaction rollback: %s", errRollBack.Error())
		}
	}
}

// OpenDBConnectionPool opens a new database connection pool. It returns an error if it can't connect to the database.
func OpenDBConnectionPool(dataSourceName string) (DBConnectionPool, error) {
	sqlxDB, err := sqlx.Open("postgres", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("error creating app DB connection pool: %w", err)
	}
	sqlxDB.SetConnMaxIdleTime(MaxDBConnIdleTime)
	sqlxDB.SetMaxOpenConns(MaxOpenDBConns)

	err = sqlxDB.Ping()
	if err != nil {
		return nil, fmt.Errorf("error pinging app DB connection pool: %w", err)
	}

	return &DBConnectionPoolImplementation{DB: sqlxDB, dataSourceName: dataSourceName}, nil
}

// OpenDBConnectionPoolWithMetrics opens a new database connection pool with the monitor service. It returns an error if it can't connect to the database.
func OpenDBConnectionPoolWithMetrics(dataSourceName string, monitorService monitor.MonitorServiceInterface) (DBConnectionPool, error) {
	dbConnectionPool, err := OpenDBConnectionPool(dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("error opening a new db connection pool: %w", err)
	}

	return NewDBConnectionPoolWithMetrics(dbConnectionPool, monitorService)
}

// CloseRows closes the given rows and logs an error if it can't close them.
func CloseRows(ctx context.Context, rows *sqlx.Rows) {
	if err := rows.Close(); err != nil {
		log.Ctx(ctx).Errorf("Failed to close rows: %v", err)
	}
}
