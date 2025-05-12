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

type WalletFromSubmitterServiceInterface interface {
	SyncTransaction(ctx context.Context, tx *schemas.EventWalletCreationCompletedData) error
	SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error
}

type WalletFromSubmitterService struct {
	sdpModels *data.Models
	tssModel  *txSubStore.TransactionModel
}

var _ WalletFromSubmitterServiceInterface = new(WalletFromSubmitterService)

func (s WalletFromSubmitterService) SyncTransaction(ctx context.Context, tx *schemas.EventWalletCreationCompletedData) error {
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
		return fmt.Errorf("synchronizing wallet creation from submitter: %w", err)
	}

	return nil
}

func (s WalletFromSubmitterService) syncTransactions(ctx context.Context, sdpDBTx db.DBTransaction, tssDBTx db.DBTransaction, transactions []*txSubStore.Transaction) error {
	if s.sdpModels == nil || s.tssModel == nil {
		return fmt.Errorf("WalletFromSubmitterService sdpMoedls and tssModel cannot be nil")
	}

	if len(transactions) == 0 {
		log.Ctx(ctx).Debug("No wallet creation transactions to sync from submitter to SDP")
		return nil
	}

	transactionIDs := make([]string, len(transactions))
	for _, transaction := range transactions {
		if !transaction.StellarTransactionHash.Valid {
			return fmt.Errorf("expected transaction %s to have a stellar transaction hash", transaction.ID)
		}
		if transaction.Status != txSubStore.TransactionStatusSuccess && transaction.Status != txSubStore.TransactionStatusError {
			return fmt.Errorf("expected transaction %s to be in success or error state", transaction.ID)
		}

		err := s.syncWalletsWithTransaction(ctx, sdpDBTx, transaction)
		if err != nil {
			return fmt.Errorf("syncing wallets with transaction %s: %w", transaction.ID, err)
		}
		transactionIDs = append(transactionIDs, transaction.ID)
	}

	err := s.tssModel.UpdateSyncedTransactions(ctx, tssDBTx, transactionIDs)
	if err != nil {
		return fmt.Errorf("updating transactions as synced: %w", err)
	}
	log.Ctx(ctx).Infof("Updated %d transactions as synced", len(transactionIDs))

	return nil
}

func (s WalletFromSubmitterService) syncWalletsWithTransaction(ctx context.Context, sdpDBTx db.DBTransaction, transaction *txSubStore.Transaction) error {
	return nil
}

func (s WalletFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error {
	return nil
}
