package services

import (
	"context"
	"fmt"
	"sort"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

const MaxErrorMessageLength = 255

type PatchAnchorPlatformTransactionCompletionService struct {
	apAPISvc  anchorplatform.AnchorPlatformAPIServiceInterface
	sdpModels *data.Models
}

func NewPatchAnchorPlatformTransactionCompletionService(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, sdpModels *data.Models) (*PatchAnchorPlatformTransactionCompletionService, error) {
	if apAPISvc == nil {
		return nil, fmt.Errorf("anchor platform API service is required")
	}

	if sdpModels == nil {
		return nil, fmt.Errorf("SDP models are required")
	}

	return &PatchAnchorPlatformTransactionCompletionService{
		apAPISvc:  apAPISvc,
		sdpModels: sdpModels,
	}, nil
}

func (s *PatchAnchorPlatformTransactionCompletionService) PatchTransactionsCompletion(ctx context.Context) error {
	// Step 1: Get all Success and Failed payments from receivers for their respective wallets.
	payments, err := db.RunInTransactionWithResult(ctx, s.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) ([]data.Payment, error) {
		payments, err := s.sdpModels.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbTx)
		if err != nil {
			return nil, fmt.Errorf("getting payments: %w", err)
		}

		return payments, nil
	})
	if err != nil {
		return fmt.Errorf("getting payments from database transaction: %w", err)
	}

	log.Ctx(ctx).Debugf("PatchAnchorPlatformTransactionService: got %d payments to process", len(payments))

	// successfulPaymentsForAPTransactionID has its keys as the AP Transaction ID. Here we store the transaction IDs
	// from the transactions patched to the AP with the "Completed" anchor status. So we avoid concurrency errors like, a receiver having
	// two payments for the same wallet, we report the transaction as "Complete" to the AP and then overwrite this
	// status with the "Error" status.
	successfulPaymentsForAPTransactionID := make(map[string]struct{}, len(payments))

	// Step 2: Iterate over the payments.
	receiverWalletIDs := make([]string, 0)
	for _, payment := range payments {
		// Step 3: Check if the AP transaction was already patched as completed. If it's true we don't need to report it anymore.
		if _, ok := successfulPaymentsForAPTransactionID[payment.ReceiverWallet.AnchorPlatformTransactionID]; ok {
			log.Ctx(ctx).Debugf(
				"PatchAnchorPlatformTransactionService: anchor platform transaction ID %q already patched as completed. No action needed",
				payment.ReceiverWallet.AnchorPlatformTransactionID,
			)
			continue
		}

		// Step 4: patch the transaction on the AP with the respective status.
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
				log.Ctx(ctx).Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q with status %q: %v", payment.ReceiverWallet.AnchorPlatformTransactionID, anchorplatform.APTransactionStatusCompleted, err)
				continue
			}
			// marking the AP Transaction as synced as completed.
			successfulPaymentsForAPTransactionID[payment.ReceiverWallet.AnchorPlatformTransactionID] = struct{}{}
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
				log.Ctx(ctx).Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q with status %q: %v", payment.ReceiverWallet.AnchorPlatformTransactionID, anchorplatform.APTransactionStatusError, err)
				continue
			}
		} else {
			log.Ctx(ctx).Errorf("PatchAnchorPlatformTransactionService: invalid payment status to patch to anchor platform. Payment ID: %s - Status: %s", payment.ID, payment.Status)
			continue
		}

		// Step 5: If the transaction was successfully patched we select it to be marked as synced.
		receiverWalletIDs = append(receiverWalletIDs, payment.ReceiverWallet.ID)
	}

	log.Ctx(ctx).Debugf("PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for %d receiver wallet(s)", len(receiverWalletIDs))

	// Step 6: we update the receiver_wallets table saying that the AP transaction associated with the user registration
	// was successfully patched/synced.
	_, err = s.sdpModels.ReceiverWallet.UpdateAnchorPlatformTransactionSyncedAt(ctx, receiverWalletIDs...)
	if err != nil {
		return fmt.Errorf("updating receiver wallet anchor platform transaction synced at: %w", err)
	}

	return nil
}
