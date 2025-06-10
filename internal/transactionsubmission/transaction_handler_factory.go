package transactionsubmission

import (
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type TransactionHandlerFactory struct {
	engine        *engine.SubmitterEngine
	txModel       store.TransactionStore
	eventProducer events.Producer
	monitorSvc    tssMonitor.TSSMonitorService
	rpcClient     stellar.RPCClient
}

var _ TransactionHandlerFactoryInterface = &TransactionHandlerFactory{}

func NewTransactionHandlerFactory(
	engine *engine.SubmitterEngine,
	txModel store.TransactionStore,
	eventProducer events.Producer,
	monitorSvc tssMonitor.TSSMonitorService,
	rpcClient stellar.RPCClient,
) *TransactionHandlerFactory {
	return &TransactionHandlerFactory{
		engine:        engine,
		txModel:       txModel,
		eventProducer: eventProducer,
		monitorSvc:    monitorSvc,
		rpcClient:     rpcClient,
	}
}

func (f *TransactionHandlerFactory) GetTransactionHandler(tx *store.Transaction) (TransactionHandlerInterface, error) {
	switch tx.TransactionType {
	case store.TransactionTypePayment:
		paymentHandler, err := NewPaymentTransactionHandler(f.engine, f.monitorSvc)
		if err != nil {
			return nil, fmt.Errorf("creating payment transaction handler: %w", err)
		}
		return paymentHandler, nil
	case store.TransactionTypeWalletCreation:
		if f.rpcClient == nil {
			return nil, fmt.Errorf("rpc client is required for wallet creation transaction handler")
		}
		walletCreationHandler, err := NewWalletCreationTransactionHandler(f.engine, f.rpcClient, f.monitorSvc)
		if err != nil {
			return nil, fmt.Errorf("creating wallet creation transaction handler: %w", err)
		}
		return walletCreationHandler, nil
	default:
		return nil, fmt.Errorf("unsupported transaction type: %s", tx.TransactionType)
	}
}
