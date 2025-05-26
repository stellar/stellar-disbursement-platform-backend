package transactionsubmission

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type TransactionHandlerFactory struct {
	engine        *engine.SubmitterEngine
	txModel       store.TransactionStore
	eventProducer events.Producer
	monitorSvc    tssMonitor.TSSMonitorService
}

var _ TransactionHandlerFactoryInterface = &TransactionHandlerFactory{}

func NewTransactionHandlerFactory(
	engine *engine.SubmitterEngine,
	txModel store.TransactionStore,
	eventProducer events.Producer,
	monitorSvc tssMonitor.TSSMonitorService,
) *TransactionHandlerFactory {
	return &TransactionHandlerFactory{
		engine:        engine,
		txModel:       txModel,
		eventProducer: eventProducer,
		monitorSvc:    monitorSvc,
	}
}

func (f *TransactionHandlerFactory) GetTransactionHandler(tx *store.Transaction) (TransactionHandlerInterface, error) {
	paymentWorker, err := NewPaymentTransactionHandler(f.engine, f.monitorSvc)
	if err != nil {
		return nil, err
	}

	return paymentWorker, nil
}
