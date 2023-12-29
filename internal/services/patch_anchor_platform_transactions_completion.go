package services

import (
	"context"
	"fmt"
	"sort"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

const MaxErrorMessageLength = 255

type PatchAnchorPlatformTransactionCompletionServiceInterface interface {
	PatchTransactionCompletion(ctx context.Context, req PatchAnchorPlatformTransactionCompletionReq) error
	SetModels(models *data.Models)
}

type PatchAnchorPlatformTransactionCompletionService struct {
	apAPISvc  anchorplatform.AnchorPlatformAPIServiceInterface
	sdpModels *data.Models
}

var _ PatchAnchorPlatformTransactionCompletionServiceInterface = new(PatchAnchorPlatformTransactionCompletionService)

type PatchAnchorPlatformTransactionCompletionReq struct {
	PaymentID string `json:"payment_id"`
}

func NewPatchAnchorPlatformTransactionCompletionService(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, sdpModels *data.Models) (*PatchAnchorPlatformTransactionCompletionService, error) {
	if apAPISvc == nil {
		return nil, fmt.Errorf("anchor platform API service is required")
	}

	return &PatchAnchorPlatformTransactionCompletionService{
		apAPISvc:  apAPISvc,
		sdpModels: sdpModels,
	}, nil
}

func (s *PatchAnchorPlatformTransactionCompletionService) PatchTransactionCompletion(ctx context.Context, req PatchAnchorPlatformTransactionCompletionReq) error {
	if s.sdpModels == nil {
		return fmt.Errorf("SDP models are required")
	}

	// Step 1: Get the requested payment.
	payment, err := db.RunInTransactionWithResult(ctx, s.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.Payment, error) {
		payment, err := s.sdpModels.Payment.GetReadyToPatchCompletionAnchorTransactionByID(ctx, dbTx, req.PaymentID)
		if err != nil {
			return nil, fmt.Errorf("getting payment ID %s: %w", req.PaymentID, err)
		}
		return payment, nil
	})
	if err != nil {
		return fmt.Errorf("getting payment from database transaction: %w", err)
	}

	// Step 2: patch the transaction on the AP with the respective status.
	if payment.Status == data.SuccessPaymentStatus {
		err = s.apAPISvc.PatchAnchorTransactionsPostSuccessCompletion(ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
			ID:     payment.ReceiverWallet.AnchorPlatformTransactionID,
			SEP:    "24",
			Status: anchorplatform.APTransactionStatusCompleted,
			StellarTransactions: []anchorplatform.APStellarTransaction{
				{
					ID:       payment.StellarTransactionID,
					Memo:     payment.ReceiverWallet.StellarMemo,
					MemoType: payment.ReceiverWallet.StellarMemoType,
				},
			},
			CompletedAt: &payment.UpdatedAt,
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
	} else if payment.Status == data.FailedPaymentStatus {
		sort.Slice(payment.StatusHistory, func(i, j int) bool {
			return payment.StatusHistory[i].Timestamp.After(payment.StatusHistory[j].Timestamp)
		})

		var status data.PaymentStatusHistoryEntry
		for _, st := range payment.StatusHistory {
			if st.Status == data.FailedPaymentStatus {
				status = st
				break
			}
		}

		messageLength := len(status.StatusMessage)
		if messageLength > MaxErrorMessageLength {
			messageLength = MaxErrorMessageLength - 1
		}

		err = s.apAPISvc.PatchAnchorTransactionsPostErrorCompletion(ctx, anchorplatform.APSep24TransactionPatchPostError{
			ID:      payment.ReceiverWallet.AnchorPlatformTransactionID,
			SEP:     "24",
			Message: status.StatusMessage[:messageLength],
			Status:  anchorplatform.APTransactionStatusError,
		})
		if err != nil {
			err = fmt.Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q with status %q: %w", payment.ReceiverWallet.AnchorPlatformTransactionID, anchorplatform.APTransactionStatusError, err)
			log.Ctx(ctx).Error(err)
			return err
		}
	} else {
		err = fmt.Errorf("PatchAnchorPlatformTransactionService: invalid payment status to patch to anchor platform. Payment ID: %s - Status: %s", payment.ID, payment.Status)
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
