package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

//go:generate mockery --name=SponsoredTransactionFromSubmitterServiceInterface --case=snake --structname=MockSponsoredTransactionFromSubmitterService

type SponsoredTransactionFromSubmitterServiceInterface interface {
	SyncTransaction(ctx context.Context, transactionID string) error
	SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error
}

// SponsoredTransactionFromSubmitterService is a service that monitors TSS sponsored transactions that were completed and syncs their completion
// state with the SDP sponsored_transactions table.
type SponsoredTransactionFromSubmitterService struct {
	sdpModels *data.Models
	tssModel  *store.TransactionModel
}

var _ SponsoredTransactionFromSubmitterServiceInterface = (*SponsoredTransactionFromSubmitterService)(nil)

// NewSponsoredTransactionFromSubmitterService creates a new instance of SponsoredTransactionFromSubmitterService.
func NewSponsoredTransactionFromSubmitterService(
	models *data.Models,
	tssDBConnectionPool db.DBConnectionPool,
) *SponsoredTransactionFromSubmitterService {
	return &SponsoredTransactionFromSubmitterService{
		sdpModels: models,
		tssModel:  store.NewTransactionModel(tssDBConnectionPool),
	}
}

// SyncTransaction syncs a single completed TSS sponsored transaction with the sponsored_transactions table
func (s *SponsoredTransactionFromSubmitterService) SyncTransaction(ctx context.Context, transactionID string) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			transaction, err := s.tssModel.GetTransactionPendingUpdateByID(ctx, tssDBTx, transactionID, store.TransactionTypeSponsored)
			if err != nil {
				if errors.Is(err, store.ErrRecordNotFound) {
					return fmt.Errorf("sponsored transaction %s not found or wrong type", transactionID)
				}
				return fmt.Errorf("getting sponsored transaction %s: %w", transactionID, err)
			}

			return s.syncTransactions(ctx, sdpDBTx, tssDBTx, []*store.Transaction{transaction})
		})
	})
	if err != nil {
		return fmt.Errorf("synchronizing sponsored transaction from submitter: %w", err)
	}

	return nil
}

// SyncBatchTransactions syncs a batch of completed TSS sponsored transactions with the sponsored_transactions table
func (s *SponsoredTransactionFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			transactions, err := s.tssModel.GetTransactionBatchForUpdate(ctx, tssDBTx, batchSize, tenantID, store.TransactionTypeSponsored)
			if err != nil {
				return fmt.Errorf("getting sponsored transactions for update: %w", err)
			}
			return s.syncTransactions(ctx, sdpDBTx, tssDBTx, transactions)
		})
	})
	if err != nil {
		return fmt.Errorf("synchronizing sponsored transactions from submitter: %w", err)
	}

	return nil
}

// syncTransactions synchronizes TSS sponsored transactions with the sponsored_transactions table.
// It should be called within a DB transaction. This method processes multiple transactions efficiently.
func (s *SponsoredTransactionFromSubmitterService) syncTransactions(ctx context.Context, sdpDBTx, tssDBTx db.DBTransaction, transactions []*store.Transaction) error {
	if s.sdpModels == nil || s.tssModel == nil {
		return fmt.Errorf("SponsoredTransactionFromSubmitterService sdpModels and tssModel cannot be nil")
	}

	if len(transactions) == 0 {
		log.Ctx(ctx).Debug("No sponsored transactions to sync from submitter to SDP")
		return nil
	}

	// 1. Validate all transactions
	for _, transaction := range transactions {
		if transaction.Status == store.TransactionStatusSuccess {
			if !transaction.StellarTransactionHash.Valid {
				return fmt.Errorf("expected successful transaction %s to have a stellar transaction hash", transaction.ID)
			}
			if !transaction.DistributionAccount.Valid {
				return fmt.Errorf("expected successful transaction %s to have a distribution account", transaction.ID)
			}
		}
		if transaction.Sponsored.SponsoredAccount == "" {
			return fmt.Errorf("expected transaction %s to have a sponsored account", transaction.ID)
		}
		if transaction.Status != store.TransactionStatusSuccess && transaction.Status != store.TransactionStatusError {
			return fmt.Errorf("transaction id %s is in an unexpected status %s", transaction.ID, transaction.Status)
		}
	}

	// 2. Sync sponsored transactions with TSS transactions
	transactionIDs := make([]string, 0, len(transactions))
	for _, transaction := range transactions {
		err := s.syncSponsoredTransactionWithTSSTransaction(ctx, sdpDBTx, transaction)
		if err != nil {
			return fmt.Errorf("syncing sponsored transaction for TSS transaction ID %s: %w", transaction.ID, err)
		}
		transactionIDs = append(transactionIDs, transaction.ID)
	}

	// 3. Set synced_at for all synced sponsored transactions
	err := s.tssModel.UpdateSyncedTransactions(ctx, tssDBTx, transactionIDs)
	if err != nil {
		return fmt.Errorf("updating transactions as synced: %w", err)
	}
	log.Ctx(ctx).Infof("Updated %d sponsored transactions as synced", len(transactions))

	return nil
}

// syncSponsoredTransactionWithTSSTransaction updates the sponsored transaction based on the TSS transaction status.
func (s *SponsoredTransactionFromSubmitterService) syncSponsoredTransactionWithTSSTransaction(ctx context.Context, sdpDBTx db.DBTransaction, transaction *store.Transaction) error {
	sponsoredTransaction, err := s.sdpModels.SponsoredTransactions.GetByID(ctx, sdpDBTx, transaction.ExternalID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return fmt.Errorf("sponsored transaction with ID %s not found", transaction.ExternalID)
		}
		return fmt.Errorf("getting sponsored transaction with ID %s: %w", transaction.ExternalID, err)
	}

	update := data.SponsoredTransactionUpdate{}

	switch transaction.Status {
	case store.TransactionStatusSuccess:
		if !transaction.StellarTransactionHash.Valid {
			return fmt.Errorf("stellar transaction hash is not set for transaction %s", transaction.ID)
		}

		update.TransactionHash = transaction.StellarTransactionHash.String
		update.Status = data.SuccessSponsoredTransactionStatus
	case store.TransactionStatusError:
		update.Status = data.FailedSponsoredTransactionStatus
	default:
		return fmt.Errorf("transaction %s is not in a terminal state (status: %s)", transaction.ID, transaction.Status)
	}

	err = s.sdpModels.SponsoredTransactions.Update(ctx, sdpDBTx, sponsoredTransaction.ID, update)
	if err != nil {
		return fmt.Errorf("updating sponsored transaction with ID %s: %w", sponsoredTransaction.ID, err)
	}

	return nil
}
