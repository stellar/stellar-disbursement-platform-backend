package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

//go:generate mockery --name=WalletCreationFromSubmitterServiceInterface --case=snake --structname=MockWalletCreationFromSubmitterService

type WalletCreationFromSubmitterServiceInterface interface {
	SyncTransaction(ctx context.Context, transactionID string) error
	SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error
}

// WalletCreationFromSubmitterService is a service that monitors TSS wallet creation transactions that were complete and sync their completion
// state with the SDP embedded wallets.
type WalletCreationFromSubmitterService struct {
	sdpModels         *data.Models
	tssModel          *store.TransactionModel
	networkPassphrase string
}

var _ WalletCreationFromSubmitterServiceInterface = (*WalletCreationFromSubmitterService)(nil)

// NewWalletCreationFromSubmitterService creates a new instance of WalletCreationFromSubmitterService.
func NewWalletCreationFromSubmitterService(
	models *data.Models,
	tssDBConnectionPool db.DBConnectionPool,
	networkPassphrase string,
) *WalletCreationFromSubmitterService {
	return &WalletCreationFromSubmitterService{
		sdpModels:         models,
		tssModel:          store.NewTransactionModel(tssDBConnectionPool),
		networkPassphrase: networkPassphrase,
	}
}

// SyncTransaction syncs a single completed TSS wallet creation transaction with the embedded wallet table
func (s *WalletCreationFromSubmitterService) SyncTransaction(ctx context.Context, transactionID string) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			transaction, err := s.tssModel.GetTransactionPendingUpdateByID(ctx, tssDBTx, transactionID, store.TransactionTypeWalletCreation)
			if err != nil {
				if errors.Is(err, store.ErrRecordNotFound) {
					return fmt.Errorf("wallet creation transaction %s not found or wrong type", transactionID)
				}
				return fmt.Errorf("getting wallet creation transaction %s: %w", transactionID, err)
			}

			return s.syncTransactions(ctx, sdpDBTx, tssDBTx, []*store.Transaction{transaction})
		})
	})
	if err != nil {
		return fmt.Errorf("synchronizing wallet creation from submitter: %w", err)
	}

	return nil
}

// SyncBatchTransactions syncs a batch of completed TSS wallet creation transactions with the embedded wallet table
func (s *WalletCreationFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			transactions, err := s.tssModel.GetTransactionBatchForUpdate(ctx, tssDBTx, batchSize, tenantID, store.TransactionTypeWalletCreation)
			if err != nil {
				return fmt.Errorf("getting wallet creation transactions for update: %w", err)
			}
			return s.syncTransactions(ctx, sdpDBTx, tssDBTx, transactions)
		})
	})
	if err != nil {
		return fmt.Errorf("synchronizing wallet creations from submitter: %w", err)
	}

	return nil
}

// syncTransactions synchronizes TSS wallet creation transactions with the embedded wallet table.
// It should be called within a DB transaction. This method processes multiple transactions efficiently.
func (s *WalletCreationFromSubmitterService) syncTransactions(ctx context.Context, sdpDBTx, tssDBTx db.DBTransaction, transactions []*store.Transaction) error {
	if s.sdpModels == nil || s.tssModel == nil {
		return fmt.Errorf("WalletCreationFromSubmitterService sdpModels and tssModel cannot be nil")
	}

	if len(transactions) == 0 {
		log.Ctx(ctx).Debug("No wallet creation transactions to sync from submitter to SDP")
		return nil
	}

	// 1. Validate all transactions
	for _, transaction := range transactions {
		if transaction.Status == store.TransactionStatusSuccess && !transaction.StellarTransactionHash.Valid {
			return fmt.Errorf("expected successful transaction %s to have a stellar transaction hash", transaction.ID)
		}
		if !transaction.DistributionAccount.Valid {
			return fmt.Errorf("expected transaction %s to have a distribution account", transaction.ID)
		}
		if transaction.WalletCreation.PublicKey == "" {
			return fmt.Errorf("expected transaction %s to have a public key in wallet creation", transaction.ID)
		}
		if transaction.Status != store.TransactionStatusSuccess && transaction.Status != store.TransactionStatusError {
			return fmt.Errorf("transaction id %s is in an unexpected status %s", transaction.ID, transaction.Status)
		}
	}

	// 2. Sync embedded wallets with transactions
	transactionIDs := make([]string, 0, len(transactions))
	for _, transaction := range transactions {
		err := s.syncEmbeddedWalletWithTransaction(ctx, sdpDBTx, transaction)
		if err != nil {
			return fmt.Errorf("syncing embedded wallet for transaction ID %s: %w", transaction.ID, err)
		}
		transactionIDs = append(transactionIDs, transaction.ID)
	}

	// 3. Set synced_at for all synced wallet creation transactions
	err := s.tssModel.UpdateSyncedTransactions(ctx, tssDBTx, transactionIDs)
	if err != nil {
		return fmt.Errorf("updating transactions as synced: %w", err)
	}
	log.Ctx(ctx).Infof("Updated %d wallet creation transactions as synced", len(transactions))

	return nil
}

// syncEmbeddedWalletWithTransaction updates the embedded wallet based on the transaction status.
func (s *WalletCreationFromSubmitterService) syncEmbeddedWalletWithTransaction(ctx context.Context, sdpDBTx db.DBTransaction, transaction *store.Transaction) error {
	embeddedWallet, err := s.sdpModels.EmbeddedWallets.GetByToken(ctx, sdpDBTx, transaction.ExternalID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return fmt.Errorf("embedded wallet with token %s not found", transaction.ExternalID)
		}
		return fmt.Errorf("getting embedded wallet with token %s: %w", transaction.ExternalID, err)
	}

	update := data.EmbeddedWalletUpdate{}

	switch transaction.Status {
	case store.TransactionStatusSuccess:
		if !transaction.DistributionAccount.Valid {
			return fmt.Errorf("distribution account is not set for transaction %s", transaction.ID)
		}

		contractAddress, calcErr := utils.CalculateContractAddress(
			transaction.DistributionAccount.String,
			transaction.WalletCreation.Salt,
			s.networkPassphrase,
		)
		if calcErr != nil {
			return fmt.Errorf("calculating contract address for transaction %s: %w", transaction.ID, calcErr)
		}

		update.ContractAddress = contractAddress
		update.WalletStatus = data.SuccessWalletStatus
	case store.TransactionStatusError:
		update.WalletStatus = data.FailedWalletStatus
	default:
		return fmt.Errorf("transaction %s is not in a terminal state (status: %s)", transaction.ID, transaction.Status)
	}

	err = s.sdpModels.EmbeddedWallets.Update(ctx, sdpDBTx, embeddedWallet.Token, update)
	if err != nil {
		return fmt.Errorf("updating embedded wallet with token %s: %w", embeddedWallet.Token, err)
	}

	return nil
}
