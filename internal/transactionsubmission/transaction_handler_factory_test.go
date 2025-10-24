package transactionsubmission

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sdpMonitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

func TestTransactionHandlerFactory_GetTransactionHandler(t *testing.T) {
	engine := &engine.SubmitterEngine{}
	txModel := &store.TransactionModel{}
	monitorSvc := tssMonitor.TSSMonitorService{
		Client: &sdpMonitorMocks.MockMonitorClient{},
	}
	rpcClient := &mocks.MockRPCClient{}

	factory := NewTransactionHandlerFactory(engine, txModel, monitorSvc, rpcClient)

	testCases := []struct {
		name          string
		transaction   *store.Transaction
		expectedType  string
		expectedError string
	}{
		{
			name: "returns payment handler for payment transaction",
			transaction: &store.Transaction{
				TransactionType: store.TransactionTypePayment,
			},
			expectedType: "*transactionsubmission.PaymentTransactionHandler",
		},
		{
			name: "returns wallet creation handler for wallet creation transaction",
			transaction: &store.Transaction{
				TransactionType: store.TransactionTypeWalletCreation,
			},
			expectedType: "*transactionsubmission.WalletCreationTransactionHandler",
		},
		{
			name: "returns sponsored transaction handler for sponsored transaction",
			transaction: &store.Transaction{
				TransactionType: store.TransactionTypeSponsored,
			},
			expectedType: "*transactionsubmission.SponsoredTransactionHandler",
		},
		{
			name: "returns error for unsupported transaction type",
			transaction: &store.Transaction{
				TransactionType: "UNSUPPORTED_TYPE",
			},
			expectedError: "unsupported transaction type: UNSUPPORTED_TYPE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := factory.GetTransactionHandler(tc.transaction)

			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, handler)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handler)
				assert.Equal(t, tc.expectedType, getTypeName(handler))
			}
		})
	}
}

func getTypeName(obj interface{}) string {
	switch obj.(type) {
	case *PaymentTransactionHandler:
		return "*transactionsubmission.PaymentTransactionHandler"
	case *WalletCreationTransactionHandler:
		return "*transactionsubmission.WalletCreationTransactionHandler"
	case *SponsoredTransactionHandler:
		return "*transactionsubmission.SponsoredTransactionHandler"
	default:
		return "unknown"
	}
}
