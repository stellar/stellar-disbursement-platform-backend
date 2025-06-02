package transactionsubmission

import (
	"context"

	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stretchr/testify/mock"
)

type MockTransactionHandler struct {
	mock.Mock
}

func (m *MockTransactionHandler) BuildInnerTransaction(ctx context.Context, txJob *TxJob, sequenceNumber int64, distributionAccount string) (*txnbuild.Transaction, error) {
	args := m.Called(ctx, txJob, sequenceNumber, distributionAccount)
	if tx := args.Get(0); tx != nil {
		return tx.(*txnbuild.Transaction), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTransactionHandler) BuildSuccessEvent(ctx context.Context, txJob *TxJob) (*events.Message, error) {
	args := m.Called(ctx, txJob)
	if msg := args.Get(0); msg != nil {
		return msg.(*events.Message), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTransactionHandler) BuildFailureEvent(ctx context.Context, txJob *TxJob, hErr *utils.HorizonErrorWrapper) (*events.Message, error) {
	args := m.Called(ctx, txJob, hErr)
	if msg := args.Get(0); msg != nil {
		return msg.(*events.Message), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTransactionHandler) AddContextLoggerFields(transaction *store.Transaction) map[string]interface{} {
	args := m.Called(transaction)
	if fields := args.Get(0); fields != nil {
		return fields.(map[string]interface{})
	}
	return nil
}

func (m *MockTransactionHandler) MonitorTransactionProcessingStarted(ctx context.Context, txJob *TxJob, jobUUID string) {
	m.Called(ctx, txJob, jobUUID)
}

func (m *MockTransactionHandler) MonitorTransactionProcessingSuccess(ctx context.Context, txJob *TxJob, jobUUID string) {
	m.Called(ctx, txJob, jobUUID)
}

func (m *MockTransactionHandler) MonitorTransactionProcessingFailed(ctx context.Context, txJob *TxJob, jobUUID string, isRetryable bool, errStack string) {
	m.Called(ctx, txJob, jobUUID, isRetryable, errStack)
}

func (m *MockTransactionHandler) MonitorTransactionReconciliationSuccess(ctx context.Context, txJob *TxJob, jobUUID string, successType ReconcileSuccessType) {
	m.Called(ctx, txJob, jobUUID, successType)
}

func (m *MockTransactionHandler) MonitorTransactionReconciliationFailure(ctx context.Context, txJob *TxJob, jobUUID string, isHorizonErr bool, errStack string) {
	m.Called(ctx, txJob, jobUUID, isHorizonErr, errStack)
}
