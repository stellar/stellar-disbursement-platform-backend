package paymentdispatchers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type StellarPaymentDispatcher struct {
	sdpModels           *data.Models
	tssModel            *txSubStore.TransactionModel
	distAccountResolver signing.DistributionAccountResolver
	memoResolver        MemoResolverInterface
}

func NewStellarPaymentDispatcher(sdpModels *data.Models, tssModel *txSubStore.TransactionModel, distAccountResolver signing.DistributionAccountResolver) *StellarPaymentDispatcher {
	return &StellarPaymentDispatcher{
		sdpModels:           sdpModels,
		tssModel:            tssModel,
		distAccountResolver: distAccountResolver,
		memoResolver:        &MemoResolver{Organizations: sdpModels.Organizations},
	}
}

func (s *StellarPaymentDispatcher) DispatchPayments(ctx context.Context, sdpDBTx db.DBTransaction, tenantID string, paymentsToDispatch []*data.Payment) error {
	if len(paymentsToDispatch) == 0 {
		return nil
	}

	distAccount, err := s.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsStellar() {
		return fmt.Errorf("distribution account is not a Stellar account for tenant %s", tenantID)
	}

	return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
		return s.sendPaymentsToTSS(ctx, sdpDBTx, tssDBTx, tenantID, paymentsToDispatch)
	})
}

func (s *StellarPaymentDispatcher) SupportedPlatform() schema.Platform {
	return schema.StellarPlatform
}

var _ PaymentDispatcherInterface = (*StellarPaymentDispatcher)(nil)

func (s *StellarPaymentDispatcher) sendPaymentsToTSS(ctx context.Context, sdpDBTx, tssDBTx db.DBTransaction, tenantID string, pendingPayments []*data.Payment) error {
	var transactions []txSubStore.Transaction
	for _, payment := range pendingPayments {
		// TODO: change TSS to use string amount [SDP-483]
		amount, err := strconv.ParseFloat(payment.Amount, 64)
		if err != nil {
			return fmt.Errorf("parsing payment amount %s for payment ID %s: %w", payment.Amount, payment.ID, err)
		}

		memo, err := s.memoResolver.GetMemo(ctx, *payment.ReceiverWallet)
		if err != nil {
			return fmt.Errorf("getting memo: %w", err)
		}

		transaction := txSubStore.Transaction{
			ExternalID:  payment.ID,
			AssetCode:   payment.Asset.Code,
			AssetIssuer: payment.Asset.Issuer,
			Amount:      decimal.NewFromFloat(amount),
			Destination: payment.ReceiverWallet.StellarAddress,
			Memo:        memo.Value,
			MemoType:    memo.Type,
			TenantID:    tenantID,
		}
		transactions = append(transactions, transaction)
	}

	insertedTransactions, err := s.tssModel.BulkInsert(ctx, tssDBTx, transactions)
	if err != nil {
		return fmt.Errorf("inserting transactions: %w", err)
	}
	if len(insertedTransactions) > 0 {
		insertedTxIDs := make([]string, 0, len(insertedTransactions))
		for _, insertedTransaction := range insertedTransactions {
			insertedTxIDs = append(insertedTxIDs, insertedTransaction.ID)
		}
		log.Ctx(ctx).Infof("Submitted %d transaction(s) to TSS=%+v", len(insertedTransactions), insertedTxIDs)
	}

	// Update payment status to PENDING in the SDP database:
	if len(pendingPayments) > 0 {
		numUpdated, updateErr := s.sdpModels.Payment.UpdateStatuses(ctx, sdpDBTx, pendingPayments, data.PendingPaymentStatus)
		if updateErr != nil {
			return fmt.Errorf("updating payment statuses to %s: %w", data.PendingPaymentStatus, updateErr)
		}
		updatedPaymentIDs := make([]string, 0, len(pendingPayments))
		for _, pendingPayment := range pendingPayments {
			updatedPaymentIDs = append(updatedPaymentIDs, pendingPayment.ID)
		}
		log.Ctx(ctx).Infof("Updated %d payments to Pending=%+v", numUpdated, updatedPaymentIDs)
	}
	return nil
}
