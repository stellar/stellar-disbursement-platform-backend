package services

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

//go:generate mockery --name=WalletCreationToSubmitterServiceInterface --case=underscore --structname=MockWalletCreationToSubmitterService --filename=wallet_creation_to_submitter_service.go
type WalletCreationToSubmitterServiceInterface interface {
	SendBatchWalletCreations(ctx context.Context, batchSize int) error
}

var _ WalletCreationToSubmitterServiceInterface = (*WalletCreationToSubmitterService)(nil)

type WalletCreationToSubmitterService struct {
	sdpModels *data.Models
	tssModel  *store.TransactionModel
}

type WalletCreationToSubmitterServiceOptions struct {
	Models              *data.Models
	TSSDBConnectionPool db.DBConnectionPool
}

func NewWalletCreationToSubmitterService(opts WalletCreationToSubmitterServiceOptions) (*WalletCreationToSubmitterService, error) {
	if opts.Models == nil {
		return nil, fmt.Errorf("models cannot be nil")
	}
	if opts.TSSDBConnectionPool == nil {
		return nil, fmt.Errorf("TSS DB connection pool cannot be nil")
	}

	return &WalletCreationToSubmitterService{
		sdpModels: opts.Models,
		tssModel:  store.NewTransactionModel(opts.TSSDBConnectionPool),
	}, nil
}

func (s *WalletCreationToSubmitterService) SendBatchWalletCreations(ctx context.Context, batchSize int) error {
	if batchSize <= 0 {
		return fmt.Errorf("batch size must be greater than 0")
	}

	tenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context: %w", err)
	}

	return db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpTx db.DBTransaction) error {
		wallets, err := s.sdpModels.EmbeddedWallets.GetPendingForSubmission(ctx, sdpTx, batchSize)
		if err != nil {
			return fmt.Errorf("getting pending embedded wallets: %w", err)
		}
		if len(wallets) == 0 {
			log.Ctx(ctx).Debug("no embedded wallets to submit to TSS")
			return nil
		}

		transactions := make([]store.Transaction, 0, len(wallets))
		for _, wallet := range wallets {
			if wallet.PublicKey == "" || wallet.WasmHash == "" {
				log.Ctx(ctx).Warnf("embedded wallet %s is missing required data, skipping", wallet.Token)
				continue
			}

			transactions = append(transactions, store.Transaction{
				ExternalID:      wallet.Token,
				TransactionType: store.TransactionTypeWalletCreation,
				TenantID:        tenant.ID,
				WalletCreation: store.WalletCreation{
					PublicKey: wallet.PublicKey,
					WasmHash:  wallet.WasmHash,
				},
			})
		}

		if len(transactions) == 0 {
			return nil
		}

		var inserted []store.Transaction
		if err := db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssTx db.DBTransaction) error {
			var bulkErr error
			inserted, bulkErr = s.tssModel.BulkInsert(ctx, tssTx, transactions)
			if bulkErr != nil {
				return fmt.Errorf("creating wallet transactions in TSS: %w", bulkErr)
			}
			return nil
		}); err != nil {
			return err
		}

		insertedTxs := make(map[string]struct{}, len(inserted))
		for _, tx := range inserted {
			insertedTxs[tx.ExternalID] = struct{}{}
		}

		for _, wallet := range wallets {
			if _, ok := insertedTxs[wallet.Token]; !ok {
				log.Ctx(ctx).Warnf("no TSS transaction created for wallet %s, skipping status update", wallet.Token)
				continue
			}

			update := data.EmbeddedWalletUpdate{WalletStatus: data.ProcessingWalletStatus}
			if err := s.sdpModels.EmbeddedWallets.Update(ctx, sdpTx, wallet.Token, update); err != nil {
				return fmt.Errorf("updating embedded wallet %s: %w", wallet.Token, err)
			}
		}

		return nil
	})
}
