package db

import (
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

func NewDBTransactionWithMetrics(dbTransaction DBTransaction, monitorServiceInterface monitor.MonitorServiceInterface) (*DBTransactionWithMetrics, error) {
	sqlExec, err := NewSQLExecuterWithMetrics(dbTransaction, monitorServiceInterface)
	if err != nil {
		return nil, fmt.Errorf("error creating SQLExecuterWithMetrics: %w", err)
	}

	return &DBTransactionWithMetrics{
		dbTransaction:          dbTransaction,
		SQLExecuterWithMetrics: *sqlExec,
	}, nil
}

// DBTransactionWithMetrics is an interface that implements DBTransaction with the monitoring service.
type DBTransactionWithMetrics struct {
	dbTransaction DBTransaction
	SQLExecuterWithMetrics
}

// make sure DBTransactionWithMetrics implements DBTransaction:
var _ DBTransaction = (*DBTransactionWithMetrics)(nil)

func (dbTx *DBTransactionWithMetrics) Commit() error {
	return dbTx.dbTransaction.Commit()
}

func (dbTx *DBTransactionWithMetrics) Rollback() error {
	return dbTx.dbTransaction.Rollback()
}
