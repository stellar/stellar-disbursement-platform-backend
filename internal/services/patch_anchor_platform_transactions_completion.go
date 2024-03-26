package services

import (
	"context"
	"fmt"
	"sort"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
)

const MaxErrorMessageLength = 255

type PatchAnchorPlatformTransactionCompletionServiceInterface interface {
	PatchAPTransactionForPaymentEvent(ctx context.Context, tx schemas.EventPaymentCompletedData) error
	PatchAPTransactionsForPayments(ctx context.Context) error
}

type PatchAnchorPlatformTransactionCompletionService struct {
	apAPISvc  anchorplatform.AnchorPlatformAPIServiceInterface
	sdpModels *data.Models
}

var _ PatchAnchorPlatformTransactionCompletionServiceInterface = new(PatchAnchorPlatformTransactionCompletionService)

func NewPatchAnchorPlatformTransactionCompletionService(apAPISvc anchorplatform.AnchorPlatformAPIServiceInterface, sdpModels *data.Models) (PatchAnchorPlatformTransactionCompletionServiceInterface, error) {
	if apAPISvc == nil || sdpModels == nil {
		return nil, fmt.Errorf("anchor platform API service and SDP models are required")
	}

	return &PatchAnchorPlatformTransactionCompletionService{
		apAPISvc:  apAPISvc,
		sdpModels: sdpModels,
	}, nil
}

func (s *PatchAnchorPlatformTransactionCompletionService) PatchAPTransactionForPaymentEvent(ctx context.Context, tx schemas.EventPaymentCompletedData) error {
	return db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// Step 1: Get the requested payment.
		payment, err := s.sdpModels.Payment.Get(ctx, tx.PaymentID, dbTx)
		if err != nil {
			return fmt.Errorf("getting payment ID %s: %w", tx.PaymentID, err)
		}

		if payment.ReceiverWallet.AnchorPlatformTransactionSyncedAt != nil && !payment.ReceiverWallet.AnchorPlatformTransactionSyncedAt.IsZero() {
			log.Ctx(ctx).Infof("AP Transaction ID %s already patched", payment.ReceiverWallet.AnchorPlatformTransactionID)
			return nil
		}

		// Step 2: patch the transaction on the AP with the respective status.
		paymentStatus := data.PaymentStatus(tx.PaymentStatus)
		if paymentStatus != payment.Status {
			return fmt.Errorf("payment status %s from payment ID %s does not match the status %s from the event", payment.Status, payment.ID, paymentStatus)
		}
		err = s.patchAnchorPaymentTransaction(ctx, *payment, tx.PaymentStatusMessage)
		if err != nil {
			return fmt.Errorf("patching anchor platform transaction: %w", err)
		}

		// Step 3: we update the receiver_wallets table saying that the AP transaction associated with the user registration
		// was successfully patched/synced.
		_, err = s.sdpModels.ReceiverWallet.UpdateAnchorPlatformTransactionSyncedAt(ctx, dbTx, payment.ReceiverWallet.ID)
		if err != nil {
			return fmt.Errorf("updating receiver wallet anchor platform transaction synced at: %w", err)
		}

		return nil
	})
}

func (s *PatchAnchorPlatformTransactionCompletionService) PatchAPTransactionsForPayments(ctx context.Context) error {
	return db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// Step 1: Get all Success and Failed payments from receivers for their respective wallets.
		payments, err := s.sdpModels.Payment.GetAllReadyToPatchCompletionAnchorTransactions(ctx, dbTx)
		if err != nil {
			return fmt.Errorf("getting payments: %w", err)
		}

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

			// Step 4: patch the transaction on the AP with the respective status
			var statusMessage string
			if payment.Status == data.FailedPaymentStatus {
				statusMessage = failedStatusMessageFromPayment(payment)
			}
			patchErr := s.patchAnchorPaymentTransaction(ctx, payment, statusMessage)
			if patchErr != nil {
				log.Ctx(ctx).Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction: %v", patchErr)
				continue
			}
			if payment.Status == data.SuccessPaymentStatus {
				successfulPaymentsForAPTransactionID[payment.ReceiverWallet.AnchorPlatformTransactionID] = struct{}{}
			}

			// Step 5: If the transaction was successfully patched we select it to be marked as synced.
			receiverWalletIDs = append(receiverWalletIDs, payment.ReceiverWallet.ID)
		}

		log.Ctx(ctx).Debugf("PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for %d receiver wallet(s)", len(receiverWalletIDs))

		// Step 6: we update the receiver_wallets table saying that the AP transaction associated with the user registration
		// was successfully patched/synced.
		_, err = s.sdpModels.ReceiverWallet.UpdateAnchorPlatformTransactionSyncedAt(ctx, dbTx, receiverWalletIDs...)
		if err != nil {
			return fmt.Errorf("updating receiver wallet anchor platform transaction synced at: %w", err)
		}

		return nil
	})
}

func failedStatusMessageFromPayment(payment data.Payment) string {
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
	return status.StatusMessage
}

// patchAnchorPaymentTransaction patches the anchor platform transaction with the respective status.
func (s *PatchAnchorPlatformTransactionCompletionService) patchAnchorPaymentTransaction(ctx context.Context, payment data.Payment, statusMessage string) error {
	if payment.Status == data.SuccessPaymentStatus {
		paymentLastUpdatedAtUTC := payment.UpdatedAt.UTC()
		err := s.apAPISvc.PatchAnchorTransactionsPostSuccessCompletion(ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
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
			CompletedAt: &paymentLastUpdatedAtUTC,
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
		messageLength := len(statusMessage)
		if messageLength > MaxErrorMessageLength {
			messageLength = MaxErrorMessageLength - 1
		}

		err := s.apAPISvc.PatchAnchorTransactionsPostErrorCompletion(ctx, anchorplatform.APSep24TransactionPatchPostError{
			ID:      payment.ReceiverWallet.AnchorPlatformTransactionID,
			SEP:     "24",
			Message: statusMessage[:messageLength],
			Status:  anchorplatform.APTransactionStatusError,
		})
		if err != nil {
			err = fmt.Errorf("PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q with status %q: %w", payment.ReceiverWallet.AnchorPlatformTransactionID, anchorplatform.APTransactionStatusError, err)
			log.Ctx(ctx).Error(err)
			return err
		}
	} else {
		err := fmt.Errorf("PatchAnchorPlatformTransactionService: invalid payment status to patch to anchor platform. Payment ID: %s - Status: %s", payment.ID, payment.Status)
		log.Ctx(ctx).Error(err)
		return err
	}
	return nil
}

func (s *PatchAnchorPlatformTransactionCompletionService) SetModels(models *data.Models) {
	s.sdpModels = models
}
