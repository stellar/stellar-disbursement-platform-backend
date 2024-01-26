package services

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type PaymentFromSubmitterServiceInterface interface {
	SyncTransaction(ctx context.Context, tx *schemas.EventPaymentCompletedData) error
}

// PaymentFromSubmitterService is a service that monitors TSS transactions that were complete and sync their completion
// state with the SDP payments.
type PaymentFromSubmitterService struct {
	sdpModels *data.Models
	tssModel  *txSubStore.TransactionModel
}

var _ PaymentFromSubmitterServiceInterface = new(PaymentFromSubmitterService)

// NewPaymentFromSubmitterService is a PaymentFromSubmitterService constructor.
func NewPaymentFromSubmitterService(models *data.Models, tssDBConnectionPool db.DBConnectionPool) *PaymentFromSubmitterService {
	return &PaymentFromSubmitterService{
		sdpModels: models,
		tssModel:  txSubStore.NewTransactionModel(tssDBConnectionPool),
	}
}

// SyncTransaction syncs the completed TSS transaction with the SDP's payment.
func (s PaymentFromSubmitterService) SyncTransaction(ctx context.Context, tx *schemas.EventPaymentCompletedData) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		return s.syncTransaction(ctx, dbTx, tx)
	})
	if err != nil {
		return fmt.Errorf("synching payment from submitter: %w", err)
	}

	return nil
}

// MonitorTransactions monitors TSS transactions that were complete and sync their completion state with the SDP payments. It
// should be called within a DB transaction.
func (s PaymentFromSubmitterService) syncTransaction(ctx context.Context, dbTx db.DBTransaction, tx *schemas.EventPaymentCompletedData) error {
	if s.sdpModels == nil {
		return fmt.Errorf("PaymentFromSubmitterService.sdpModels cannot be nil")
	}

	// 1. Get transaction passed by parameter which is in a final state (status=SUCCESS or status=ERROR)
	//     this operation will lock the row.
	transaction, err := s.tssModel.GetTransactionPendingUpdateByID(ctx, s.tssModel.DBConnectionPool, tx.TransactionID)
	if err != nil {
		return fmt.Errorf("getting transaction ID %s for update: %w", tx.TransactionID, err)
	}

	if !transaction.StellarTransactionHash.Valid {
		return fmt.Errorf("expected transaction %s to have a stellar transaction hash", transaction.ID)
	}

	if transaction.Status != txSubStore.TransactionStatusSuccess && transaction.Status != txSubStore.TransactionStatusError {
		return fmt.Errorf("transaction id %s is in an unexpected status: %s", transaction.ID, transaction.Status)
	}

	// 3. Update payments based on the transaction status
	err = s.syncPaymentWithTransaction(ctx, dbTx, transaction)
	if err != nil {
		return fmt.Errorf("synching payments for transaction ID %s: %w", transaction.ID, err)
	}

	// 4. Set synced_at for the synced transaction
	err = db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
		return s.tssModel.UpdateSyncedTransactions(ctx, tssDBTx, []string{transaction.ID})
	})
	if err != nil {
		return fmt.Errorf("updating transaction ID %s as synced: %w", transaction.ID, err)
	}
	log.Ctx(ctx).Infof("Updated transaction ID %s as synced", transaction.ID)

	return nil
}

// syncPaymentWithTransaction updates the status of the payments based on the status of the transactions.
func (s PaymentFromSubmitterService) syncPaymentWithTransaction(ctx context.Context, dbTx db.DBTransaction, transaction *txSubStore.Transaction) error {
	payments, err := s.sdpModels.Payment.GetByIDs(ctx, dbTx, []string{transaction.ExternalID})
	if err != nil {
		return fmt.Errorf("getting payments by IDs: %w", err)
	}

	if len(payments) != 1 {
		return fmt.Errorf("expected exactly 1 payment for the transaction ID %s but found %d", transaction.ID, len(payments))
	}
	payment := payments[0]

	var toStatus data.PaymentStatus
	if transaction.Status == store.TransactionStatusSuccess {
		toStatus = data.SuccessPaymentStatus
	} else if transaction.Status == store.TransactionStatusError {
		toStatus = data.FailedPaymentStatus
	} else {
		return fmt.Errorf("invalid transaction status %s. Expected only %s or %s", transaction.Status, store.TransactionStatusSuccess, store.TransactionStatusError)
	}

	// Update payment status for the transaction to SUCCESS or FAILURE
	paymentUpdate := &data.PaymentUpdate{
		Status:               toStatus,
		StatusMessage:        transaction.StatusMessage.String,
		StellarTransactionID: transaction.StellarTransactionHash.String,
	}
	err = s.sdpModels.Payment.Update(ctx, dbTx, payment, paymentUpdate)
	if err != nil {
		return fmt.Errorf("updating payment ID %s for transaction ID %s: %w", payment.ID, transaction.ID, err)
	}

	// Update the disbursement to complete if it has all payments in the end state.
	err = s.sdpModels.Disbursements.CompleteDisbursements(ctx, dbTx, []string{payment.Disbursement.ID})
	if err != nil {
		return fmt.Errorf("completing disbursement: %w", err)
	}

	return nil
}
