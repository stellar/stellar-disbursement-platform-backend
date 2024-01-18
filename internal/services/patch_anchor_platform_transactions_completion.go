package services

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
)

const MaxErrorMessageLength = 255

type PatchAnchorPlatformTransactionCompletionServiceInterface interface {
	PatchTransactionCompletion(ctx context.Context, tx schemas.EventPatchAnchorPlatformTransactionCompletionData) error
	SetModels(models *data.Models)
}

type PatchAnchorPlatformTransactionCompletionService struct {
	apAPISvc  anchorplatform.AnchorPlatformAPIServiceInterface
	sdpModels *data.Models
}

var _ PatchAnchorPlatformTransactionCompletionServiceInterface = new(PatchAnchorPlatformTransactionCompletionService)

func NewPatchAnchorPlatformTransactionCompletionService(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, sdpModels *data.Models) (*PatchAnchorPlatformTransactionCompletionService, error) {
	if apAPISvc == nil {
		return nil, fmt.Errorf("anchor platform API service is required")
	}

	return &PatchAnchorPlatformTransactionCompletionService{
		apAPISvc:  apAPISvc,
		sdpModels: sdpModels,
	}, nil
}

func (s *PatchAnchorPlatformTransactionCompletionService) PatchTransactionCompletion(ctx context.Context, tx schemas.EventPatchAnchorPlatformTransactionCompletionData) error {
	if s.sdpModels == nil {
		return fmt.Errorf("SDP models are required")
	}

	// Step 1: Get the requested payment.
	payment, err := db.RunInTransactionWithResult(ctx, s.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.Payment, error) {
		payment, err := s.sdpModels.Payment.Get(ctx, tx.PaymentID, dbTx)
		if err != nil {
			return nil, fmt.Errorf("getting payment ID %s: %w", tx.PaymentID, err)
		}
		return payment, nil
	})
	if err != nil {
		return fmt.Errorf("getting payment from database transaction: %w", err)
	}

	if payment.ReceiverWallet.AnchorPlatformTransactionSyncedAt != nil && !payment.ReceiverWallet.AnchorPlatformTransactionSyncedAt.IsZero() {
		log.Ctx(ctx).Infof("AP Transaction ID %s already patched", payment.ReceiverWallet.AnchorPlatformTransactionID)
		return nil
	}

	paymentStatus := data.PaymentStatus(tx.PaymentStatus)

	// Step 2: patch the transaction on the AP with the respective status.
	if paymentStatus == data.SuccessPaymentStatus {
		err = s.apAPISvc.PatchAnchorTransactionsPostSuccessCompletion(ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
			ID:     payment.ReceiverWallet.AnchorPlatformTransactionID,
			SEP:    "24",
			Status: anchorplatform.APTransactionStatusCompleted,
			StellarTransactions: []anchorplatform.APStellarTransaction{
				{
					ID:       tx.StellarTransactionID,
					Memo:     payment.ReceiverWallet.StellarMemo,
					MemoType: payment.ReceiverWallet.StellarMemoType,
				},
			},
			CompletedAt: &tx.PaymentCompletedAt,
			AmountOut: anchorplatform.APAmount{
				Amount: payment.Amount,
				Asset:  anchorplatform.NewStellarAssetInAIF(payment.Asset.Code, payment.Asset.Issuer),
			},
		})
		if err != nil {
			err = fmt.Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q with status %q: %w", payment.ReceiverWallet.AnchorPlatformTransactionID, anchorplatform.APTransactionStatusCompleted, err)
			log.Ctx(ctx).Error(err)
			return err
		}
	} else if paymentStatus == data.FailedPaymentStatus {
		messageLength := len(tx.PaymentStatusMessage)
		if messageLength > MaxErrorMessageLength {
			messageLength = MaxErrorMessageLength - 1
		}

		err = s.apAPISvc.PatchAnchorTransactionsPostErrorCompletion(ctx, anchorplatform.APSep24TransactionPatchPostError{
			ID:      payment.ReceiverWallet.AnchorPlatformTransactionID,
			SEP:     "24",
			Message: tx.PaymentStatusMessage[:messageLength],
			Status:  anchorplatform.APTransactionStatusError,
		})
		if err != nil {
			err = fmt.Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q with status %q: %w", payment.ReceiverWallet.AnchorPlatformTransactionID, anchorplatform.APTransactionStatusError, err)
			log.Ctx(ctx).Error(err)
			return err
		}
	} else {
		err = fmt.Errorf("PatchAnchorPlatformTransactionService: invalid payment status to patch to anchor platform. Payment ID: %s - Status: %s", payment.ID, paymentStatus)
		log.Ctx(ctx).Error(err)
		return err
	}

	// Step 3: we update the receiver_wallets table saying that the AP transaction associated with the user registration
	// was successfully patched/synced.
	_, err = s.sdpModels.ReceiverWallet.UpdateAnchorPlatformTransactionSyncedAt(ctx, payment.ReceiverWallet.ID)
	if err != nil {
		return fmt.Errorf("updating receiver wallet anchor platform transaction synced at: %w", err)
	}

	return nil
}

func (s *PatchAnchorPlatformTransactionCompletionService) SetModels(models *data.Models) {
	s.sdpModels = models
}
