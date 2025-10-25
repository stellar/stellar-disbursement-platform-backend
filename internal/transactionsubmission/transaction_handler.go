package transactionsubmission

import (
	"context"

	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type ReconcileSuccessType string

const (
	ReconcileSuccess      ReconcileSuccessType = "SUCCESS"
	ReconcileReprocessing ReconcileSuccessType = "PROCESSING"
)

// TransactionHandlerInterface defines the interface for handling different transaction types
//
//go:generate mockery --name=TransactionHandlerInterface --case=underscore --structname=MockTransactionHandler --filename=transaction_handler_mock.go --inpackage
type TransactionHandlerInterface interface {
	// BuildInnerTransaction builds the inner transaction for a given job
	BuildInnerTransaction(ctx context.Context, txJob *TxJob, sequenceNumber int64, distributionAccount string) (*txnbuild.Transaction, error)

	// RequiresRebuildOnRetry returns true if this transaction type needs to be rebuilt
	// when retried
	RequiresRebuildOnRetry() bool

	// AddContextLoggerFields adds handler-specific fields to the logger context
	AddContextLoggerFields(transaction *store.Transaction) map[string]interface{}

	// MonitorTransactionProcessingStarted logs and monitors the start of transaction processing
	MonitorTransactionProcessingStarted(ctx context.Context, txJob *TxJob, jobUUID string)

	// MonitorTransactionProcessingSuccess logs and monitors when a transaction is successfully processed
	MonitorTransactionProcessingSuccess(ctx context.Context, txJob *TxJob, jobUUID string)

	// MonitorTransactionProcessingFailed logs and monitors when a transaction processing fails
	MonitorTransactionProcessingFailed(ctx context.Context, txJob *TxJob, jobUUID string, isRetryable bool, errStack string)

	// MonitorTransactionReconciliationSuccess logs and monitors successful transaction reconciliation
	MonitorTransactionReconciliationSuccess(ctx context.Context, txJob *TxJob, jobUUID string, successType ReconcileSuccessType)

	// MonitorTransactionReconciliationFailure logs and monitors failed transaction reconciliation
	MonitorTransactionReconciliationFailure(ctx context.Context, txJob *TxJob, jobUUID string, isHorizonErr bool, errStack string)
}

// TransactionHandlerFactoryInterface creates appropriate transaction handlers based on transaction type
type TransactionHandlerFactoryInterface interface {
	GetTransactionHandler(tx *store.Transaction) (TransactionHandlerInterface, error)
}
