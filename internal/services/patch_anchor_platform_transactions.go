package services

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type PatchAnchorPlatformTransactionService struct {
	apAPISvc  anchorplatform.AnchorPlatformAPIServiceInterface
	sdpModels *data.Models
}

func NewPatchAnchorPlatformTransactionService(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, sdpModels *data.Models) (*PatchAnchorPlatformTransactionService, error) {
	if apAPISvc == nil {
		return nil, fmt.Errorf("anchor platform API service is required")
	}

	if sdpModels == nil {
		return nil, fmt.Errorf("SDP models are required")
	}

	return &PatchAnchorPlatformTransactionService{
		apAPISvc:  apAPISvc,
		sdpModels: sdpModels,
	}, nil
}

func (s *PatchAnchorPlatformTransactionService) PatchTransactions(ctx context.Context) error {
	// Step 1: Get all Success and Failed payments from receivers for their respective wallets.
	payments, err := s.sdpModels.Payment.GetAllReadyToPatchAnchorTransactions(ctx)
	if err != nil {
		return fmt.Errorf("getting payments: %w", err)
	}

	log.Ctx(ctx).Infof("PatchAnchorPlatformTransactionService: got %d payments to process", len(payments))

	// successfulPaymentsForAPTransactionID has its keys as the AP Transaction ID. Here we store the transaction IDs
	// from the transactions patched to the AP with the "Completed" anchor status. So we avoid concurrency errors like, a receiver having
	// two payments for the same wallet, we report the transaction as "Complete" to the AP and then overwrite this
	// status with the "Error" status.
	successfulPaymentsForAPTransactionID := make(map[string]struct{}, len(payments))

	// Step 2: Iterate over the payments.
	receiverWalletIDs := make([]string, 0)
	for _, payment := range payments {
		// Step 3: Check the payment status. We should only accept Success and Failed status. These are the statuses the anchor is expecting.
		var status anchorplatform.APTransactionStatus
		if payment.Status == data.SuccessPaymentStatus {
			status = anchorplatform.APTransactionStatusCompleted
		} else if payment.Status == data.FailedPaymentStatus {
			status = anchorplatform.APTransactionStatusError
		} else {
			log.Ctx(ctx).Errorf("PatchAnchorPlatformTransactionService: invalid payment status to patch to anchor platform. Payment ID: %s - Status: %s", payment.ID, payment.Status)
			continue
		}

		// Step 4: Check if the AP transaction was already patched as completed. If it's true we don't need to report it anymore.
		if _, ok := successfulPaymentsForAPTransactionID[payment.ReceiverWallet.AnchorPlatformTransactionID]; ok {
			log.Ctx(ctx).Infof(
				"PatchAnchorPlatformTransactionService: anchor platform transaction ID %q already patched as completed. No action needed",
				payment.ReceiverWallet.AnchorPlatformTransactionID,
			)
			continue
		}

		// Step 5: patch the transaction on the AP with the respective status.
		err = s.apAPISvc.PatchAnchorTransactionsPostRegistration(ctx, anchorplatform.APSep24TransactionPatchPostRegistration{
			ID:     payment.ReceiverWallet.AnchorPlatformTransactionID,
			SEP:    "24",
			Status: status,
		})
		if err != nil {
			log.Ctx(ctx).Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q: %v", payment.ReceiverWallet.AnchorPlatformTransactionID, err)
			continue
		}

		// Step 6: If the transaction was successfully patched and its status is "completed", we select it to be marked as synced. So we
		// don't need to patch this transaction.
		if status == anchorplatform.APTransactionStatusCompleted {
			receiverWalletIDs = append(receiverWalletIDs, payment.ReceiverWallet.ID)
			successfulPaymentsForAPTransactionID[payment.ReceiverWallet.AnchorPlatformTransactionID] = struct{}{}
		}
	}

	log.Ctx(ctx).Infof("PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for %d receiver wallet(s)", len(receiverWalletIDs))

	// Step 7: we update the receiver_wallets table saying that the AP transaction associated with the user registration
	// was successfully patched/synced.
	_, err = s.sdpModels.ReceiverWallet.UpdateAnchorPlatformTransactionSyncedAt(ctx, receiverWalletIDs...)
	if err != nil {
		return fmt.Errorf("updating receiver wallet anchor platform transaction synced at: %w", err)
	}

	return nil
}
