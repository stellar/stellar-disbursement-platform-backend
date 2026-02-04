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

//go:generate mockery --name=SponsoredTransactionsToSubmitterServiceInterface --case=underscore --structname=MockSponsoredTransactionsToSubmitterService --filename=sponsored_transactions_to_submitter_service.go
type SponsoredTransactionsToSubmitterServiceInterface interface {
	SendBatchSponsoredTransactions(ctx context.Context, batchSize int) error
}

var _ SponsoredTransactionsToSubmitterServiceInterface = (*SponsoredTransactionsToSubmitterService)(nil)

type SponsoredTransactionsToSubmitterService struct {
	sdpModels *data.Models
	tssModel  *store.TransactionModel
}

type SponsoredTransactionsToSubmitterServiceOptions struct {
	Models              *data.Models
	TSSDBConnectionPool db.DBConnectionPool
}

func NewSponsoredTransactionsToSubmitterService(opts SponsoredTransactionsToSubmitterServiceOptions) (*SponsoredTransactionsToSubmitterService, error) {
	if opts.Models == nil {
		return nil, fmt.Errorf("models cannot be nil")
	}
	if opts.TSSDBConnectionPool == nil {
		return nil, fmt.Errorf("TSS DB connection pool cannot be nil")
	}

	return &SponsoredTransactionsToSubmitterService{
		sdpModels: opts.Models,
		tssModel:  store.NewTransactionModel(opts.TSSDBConnectionPool),
	}, nil
}

func (s *SponsoredTransactionsToSubmitterService) SendBatchSponsoredTransactions(ctx context.Context, batchSize int) error {
	if batchSize <= 0 {
		return fmt.Errorf("batch size must be greater than 0")
	}

	tenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context: %w", err)
	}

	return db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpTx db.DBTransaction) error {
		sponsored, err := s.sdpModels.SponsoredTransactions.GetPendingForSubmission(ctx, sdpTx, batchSize)
		if err != nil {
			return fmt.Errorf("getting pending sponsored transactions: %w", err)
		}
		if len(sponsored) == 0 {
			log.Ctx(ctx).Debug("no sponsored transactions to submit to TSS")
			return nil
		}

		transactions := make([]store.Transaction, 0, len(sponsored))
		for _, st := range sponsored {
			transactions = append(transactions, store.Transaction{
				ExternalID:      st.ID,
				TransactionType: store.TransactionTypeSponsored,
				TenantID:        tenant.ID,
				Sponsored: store.Sponsored{
					SponsoredAccount:      st.Account,
					SponsoredOperationXDR: st.OperationXDR,
				},
			})
		}

		var inserted []store.Transaction
		if err := db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssTx db.DBTransaction) error {
			var bulkErr error
			inserted, bulkErr = s.tssModel.BulkInsert(ctx, tssTx, transactions)
			if bulkErr != nil {
				return fmt.Errorf("creating sponsored transactions in TSS: %w", bulkErr)
			}
			return nil
		}); err != nil {
			return err
		}

		insertedTxs := make(map[string]struct{}, len(inserted))
		for _, tx := range inserted {
			insertedTxs[tx.ExternalID] = struct{}{}
		}

		for _, st := range sponsored {
			if _, ok := insertedTxs[st.ID]; !ok {
				log.Ctx(ctx).Warnf("no TSS transaction created for sponsored transaction %s, skipping status update", st.ID)
				continue
			}

			update := data.SponsoredTransactionUpdate{Status: data.ProcessingSponsoredTransactionStatus}
			if err := s.sdpModels.SponsoredTransactions.Update(ctx, sdpTx, st.ID, update); err != nil {
				return fmt.Errorf("updating sponsored transaction %s: %w", st.ID, err)
			}
		}

		return nil
	})
}
