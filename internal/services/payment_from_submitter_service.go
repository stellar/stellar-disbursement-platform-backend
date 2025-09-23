package services

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type PaymentFromSubmitterServiceInterface interface {
	SyncTransaction(ctx context.Context, tx *schemas.EventPaymentCompletedData) error
	SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error
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

// SyncBatchTransactions monitors TSS transactions that were complete and sync their completion state with the SDP payments.
func (s PaymentFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			transactions, err := s.tssModel.GetTransactionBatchForUpdate(ctx, tssDBTx, batchSize, tenantID)
			if err != nil {
				return fmt.Errorf("getting transactions for update: %w", err)
			}
			return s.syncTransactions(ctx, sdpDBTx, tssDBTx, transactions)
		})
	})
	if err != nil {
		return fmt.Errorf("synchronizing payments from submitter: %w", err)
	}

	return nil
}

// SyncTransaction syncs the completed TSS transaction with the SDP's payment.
func (s PaymentFromSubmitterService) SyncTransaction(ctx context.Context, tx *schemas.EventPaymentCompletedData) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			transaction, err := s.tssModel.GetTransactionPendingUpdateByID(ctx, tssDBTx, tx.TransactionID)
			if err != nil {
				return fmt.Errorf("getting transaction ID %s for update: %w", tx.TransactionID, err)
			}
			return s.syncTransactions(ctx, sdpDBTx, tssDBTx, []*txSubStore.Transaction{transaction})
		})
	})
	if err != nil {
		return fmt.Errorf("synchronizing payment from submitter: %w", err)
	}

	return nil
}

// syncTransactions synchronizes TSS transactions that were completed with the SDP payments. It
// should be called within a DB transaction.
func (s PaymentFromSubmitterService) syncTransactions(ctx context.Context, sdpDBTx, tssDBTx db.DBTransaction, transactions []*txSubStore.Transaction) error {
	if s.sdpModels == nil || s.tssModel == nil {
		return fmt.Errorf("PaymentFromSubmitterService sdpModels and tssModel cannot be nil")
	}

	if len(transactions) == 0 {
		log.Ctx(ctx).Debug("No transactions to sync from submitter to SDP")
		return nil
	}

	// 1. Sync payments with Transactions
	transactionIDs := make([]string, 0, len(transactions))
	for _, transaction := range transactions {
		if !transaction.StellarTransactionHash.Valid {
			return fmt.Errorf("expected transaction %s to have a stellar transaction hash", transaction.ID)
		}
		if transaction.Status != txSubStore.TransactionStatusSuccess && transaction.Status != txSubStore.TransactionStatusError {
			return fmt.Errorf("transaction id %s is in an unexpected status %s", transaction.ID, transaction.Status)
		}

		errPayments := s.syncPaymentWithTransaction(ctx, sdpDBTx, transaction)
		if errPayments != nil {
			return fmt.Errorf("syncing payments for transaction ID %s: %w", transaction.ID, errPayments)
		}
		transactionIDs = append(transactionIDs, transaction.ID)
	}

	// 2. Set synced_at for all synced transactions
	err := s.tssModel.UpdateSyncedTransactions(ctx, tssDBTx, transactionIDs)
	if err != nil {
		return fmt.Errorf("updating transactions as synced: %w", err)
	}
	log.Ctx(ctx).Infof("Updated %d transactions as synced", len(transactions))

	return nil
}

// syncPaymentWithTransaction updates the status of the payment based on the status of the transaction.
func (s PaymentFromSubmitterService) syncPaymentWithTransaction(ctx context.Context, sdpDBTx db.DBTransaction, transaction *txSubStore.Transaction) error {
	payments, err := s.sdpModels.Payment.GetByIDs(ctx, sdpDBTx, []string{transaction.ExternalID})
	if err != nil {
		return fmt.Errorf("getting payments by IDs: %w", err)
	}

	if len(payments) != 1 {
		return fmt.Errorf("expected exactly 1 payment for the transaction ID %s but found %d", transaction.ID, len(payments))
	}
	payment := payments[0]

	var toStatus data.PaymentStatus
	switch transaction.Status {
	case txSubStore.TransactionStatusSuccess:
		toStatus = data.SuccessPaymentStatus
	case txSubStore.TransactionStatusError:
		toStatus = data.FailedPaymentStatus
	default:
		return fmt.Errorf("invalid transaction status %s. Expected only %s or %s", transaction.Status, txSubStore.TransactionStatusSuccess, txSubStore.TransactionStatusError)
	}

	// Update payment status for the transaction to SUCCESS or FAILURE
	paymentUpdate := &data.PaymentUpdate{
		Status:               toStatus,
		StatusMessage:        transaction.StatusMessage.String,
		StellarTransactionID: transaction.StellarTransactionHash.String,
	}
	err = s.sdpModels.Payment.Update(ctx, sdpDBTx, &payment, paymentUpdate)
	if err != nil {
		return fmt.Errorf("updating payment ID %s for transaction ID %s: %w", payment.ID, transaction.ID, err)
	}

	if payment.Type == data.PaymentTypeDisbursement {
		// Update the disbursement to complete if it has all payments in the end state.
		err = s.sdpModels.Disbursements.CompleteDisbursements(ctx, sdpDBTx, []string{payment.Disbursement.ID})
		if err != nil {
			return fmt.Errorf("completing disbursement: %w", err)
		}
	}

	return nil
}
