package transactionsubmission

import (
	"context"

	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type ReconcileSuccessType string

const (
	ReconcileSuccess      ReconcileSuccessType = "SUCCESS"
	ReconcileReprocessing ReconcileSuccessType = "PROCESSING"
)

// TransactionHandlerInterface defines the interface for handling different transaction types
type TransactionHandlerInterface interface {
	// BuildInnerTransaction builds the inner transaction for a given job
	BuildInnerTransaction(ctx context.Context, txJob *TxJob, sequenceNumber int64, distributionAccount string) (*txnbuild.Transaction, error)

	// BuildSuccessEvent builds an appropriate event message for a successful transaction
	BuildSuccessEvent(ctx context.Context, txJob *TxJob) (*events.Message, error)

	// BuildFailureEvent builds an appropriate event message for a failed transaction
	BuildFailureEvent(ctx context.Context, txJob *TxJob, hErr *utils.HorizonErrorWrapper) (*events.Message, error)

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
