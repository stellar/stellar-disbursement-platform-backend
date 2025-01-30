package paymentdispatchers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/google/uuid"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var ErrCircleRecipientCreationFailedTooManyTimes = errors.New("Circle recipient creation failed too many times")

const (
	// maxCircleRecipientCreationAttempts is the maximum number of attempts to create a Circle recipient before giving up.
	maxCircleRecipientCreationAttempts = 5
	// initialBackoffDelay is the initial delay for the retry policy when creating a Circle recipient. The retry policy
	// uses exponential backoff with this value as the starting point.
	initialBackoffDelay = 100 * time.Millisecond
)

type CirclePaymentDispatcher struct {
	sdpModels           *data.Models
	circleService       circle.ServiceInterface
	distAccountResolver signing.DistributionAccountResolver
}

func NewCirclePaymentDispatcher(sdpModels *data.Models, circleService circle.ServiceInterface, distAccountResolver signing.DistributionAccountResolver) *CirclePaymentDispatcher {
	return &CirclePaymentDispatcher{
		sdpModels:           sdpModels,
		circleService:       circleService,
		distAccountResolver: distAccountResolver,
	}
}

func (c *CirclePaymentDispatcher) DispatchPayments(ctx context.Context, sdpDBTx db.DBTransaction, tenantID string, paymentsToDispatch []*data.Payment) error {
	if len(paymentsToDispatch) == 0 {
		return nil
	}

	distAccount, err := c.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	if !distAccount.Type.IsCircle() {
		return fmt.Errorf("distribution account is not a Circle account for tenant %s", tenantID)
	}

	circleWalletID := distAccount.CircleWalletID
	return c.sendPaymentsToCircle(ctx, sdpDBTx, circleWalletID, paymentsToDispatch)
}

func (c *CirclePaymentDispatcher) SupportedPlatform() schema.Platform {
	return schema.CirclePlatform
}

var _ PaymentDispatcherInterface = (*CirclePaymentDispatcher)(nil)

func (c *CirclePaymentDispatcher) sendPaymentsToCircle(ctx context.Context, sdpDBTx db.DBTransaction, circleWalletID string, paymentsToSubmit []*data.Payment) error {
	for _, payment := range paymentsToSubmit {
		// 1. Ensure the recipient is ready
		recipient, err := c.ensureRecipientIsReadyWithRetry(ctx, *payment.ReceiverWallet, initialBackoffDelay)
		if err != nil {
			// 2. If the recipient creation fails, set the payment status to failed
			err = fmt.Errorf("failed to create Circle recipient for payment ID %s: %w", payment.ID, err)
			log.Ctx(ctx).Error(err)
			err = c.sdpModels.Payment.UpdateStatus(ctx, sdpDBTx, payment.ID, data.FailedPaymentStatus, utils.Ptr(err.Error()), "")
			if err != nil {
				return fmt.Errorf("marking payment as failed: %w", err)
			}
			continue
		}

		// 3. Create a new circle transfer request
		transferRequest, err := c.sdpModels.CircleTransferRequests.GetOrInsert(ctx, payment.ID)
		if err != nil {
			return fmt.Errorf("inserting circle transfer request: %w", err)
		}

		// 4. Submit the payment to Circle
		payout, err := c.circleService.SendPayment(ctx, circle.PaymentRequest{
			APIType:          circle.APITypePayouts,
			SourceWalletID:   circleWalletID,
			RecipientID:      recipient.CircleRecipientID,
			Amount:           payment.Amount,
			StellarAssetCode: payment.Asset.Code,
			IdempotencyKey:   transferRequest.IdempotencyKey,
		})
		if err != nil {
			var cAPIErr *circle.APIError
			// 5.1. If the destination address is invalid, mark the recipient as failed
			if errors.As(err, &cAPIErr) && slices.Contains(circle.DestinationAddressErrorCodes, cAPIErr.Code) {
				log.Ctx(ctx).Error("the destination address is deemed invalid by Circle, marking the recipient as denied...")
				_, cRecipientUpdateErr := c.sdpModels.CircleRecipient.Update(ctx, recipient.ReceiverWalletID, data.CircleRecipientUpdate{Status: data.CircleRecipientStatusDenied})
				if cRecipientUpdateErr != nil {
					return fmt.Errorf("updating Circle recipient status: %w", cRecipientUpdateErr)
				}
			}

			// 5.2 If the payout fails, set the payment status to failed
			err = fmt.Errorf("failed to submit payment ID %s to Circle: %w", payment.ID, err)
			log.Ctx(ctx).Error(err)
			err = c.sdpModels.Payment.UpdateStatus(ctx, sdpDBTx, payment.ID, data.FailedPaymentStatus, utils.Ptr(err.Error()), "")
			if err != nil {
				return fmt.Errorf("marking payment as failed: %w", err)
			}

			continue
		}

		// 6. Update the circle transfer request with the response from Circle
		if err = c.updateCircleTransferRequest(ctx, sdpDBTx, circleWalletID, payout, transferRequest); err != nil {
			return fmt.Errorf("updating circle transfer request: %w", err)
		}

		// 7. Update the payment status based on the transfer status
		if err = c.updatePaymentStatusForCirclePayout(ctx, sdpDBTx, payout, payment); err != nil {
			return fmt.Errorf("updating payment status for Circle transfer: %w", err)
		}
	}

	return nil
}

// updateCircleTransferRequest updates the circle_transfer_request table with the response from Circle POST /payouts.
func (c *CirclePaymentDispatcher) updateCircleTransferRequest(
	ctx context.Context,
	sdpDBTx db.DBTransaction,
	circleWalletID string,
	payout *circle.Payout,
	transferRequest *data.CircleTransferRequest,
) error {
	if payout == nil {
		return fmt.Errorf("payout cannot be nil")
	}

	jsonBody, err := json.Marshal(payout)
	if err != nil {
		return fmt.Errorf("converting transfer body to json: %w", err)
	}

	var completedAt *time.Time
	circleStatus := data.CircleTransferStatus(payout.Status)
	if circleStatus.IsCompleted() {
		completedAt = utils.TimePtr(time.Now())
	}

	_, err = c.sdpModels.CircleTransferRequests.Update(ctx, sdpDBTx, transferRequest.IdempotencyKey, data.CircleTransferRequestUpdate{
		CirclePayoutID: payout.ID,
		Status:         circleStatus,
		ResponseBody:   jsonBody,
		SourceWalletID: circleWalletID,
		CompletedAt:    completedAt,
	})
	if err != nil {
		return fmt.Errorf("updating circle transfer request: %w", err)
	}

	return nil
}

// updatePaymentStatusForCirclePayout updates the payment status based on the status coming from Circle.
func (c *CirclePaymentDispatcher) updatePaymentStatusForCirclePayout(ctx context.Context, sdpDBTx db.DBTransaction, payout *circle.Payout, payment *data.Payment) error {
	paymentStatus, err := payout.Status.ToPaymentStatus()
	if err != nil {
		return fmt.Errorf("converting CIRCLE payout status to SDP Payment status: %w", err)
	}

	statusMsg := fmt.Sprintf("Payout ID %s has status=%s in Circle", payout.ID, payout.Status)
	err = c.sdpModels.Payment.UpdateStatus(ctx, sdpDBTx, payment.ID, paymentStatus, &statusMsg, payout.TransactionHash)
	if err != nil {
		return fmt.Errorf("marking payment as %s: %w", paymentStatus, err)
	}

	return nil
}

// getOrCreateRecipient gets or creates a circle_recipient entry from the database for the given receiver_wallet_id.
func (c *CirclePaymentDispatcher) getOrCreateRecipient(ctx context.Context, receiverWallet data.ReceiverWallet) (*data.CircleRecipient, error) {
	dataRecipient, err := c.sdpModels.CircleRecipient.GetByReceiverWalletID(ctx, receiverWallet.ID)

	if errors.Is(err, data.ErrRecordNotFound) {
		// DOES NOT EXIST in the DB
		log.Ctx(ctx).Infof("Inserting circle_recipient for receiver_wallet_id %q...", receiverWallet.ID)
		dataRecipient, err = c.sdpModels.CircleRecipient.Insert(ctx, receiverWallet.ID)
		if err != nil {
			return nil, fmt.Errorf("inserting Circle recipient: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("getting Circle recipient: %w", err)
	}

	// Too many attempts
	if dataRecipient.SyncAttempts >= maxCircleRecipientCreationAttempts {
		return nil, ErrCircleRecipientCreationFailedTooManyTimes
	}

	return dataRecipient, nil
}

// handleFailedRecipientIfNeeded handles the case when the recipient is in a FAILED or INACTIVE state.
func (c *CirclePaymentDispatcher) handleFailedRecipientIfNeeded(ctx context.Context, dataRecipient *data.CircleRecipient) (*data.CircleRecipient, error) {
	if !dataRecipient.Status.IsCompleted() {
		return dataRecipient, nil
	}

	// FAILED, DENIED or INACTIVE -> renovate the idempotency key
	var err error
	log.Ctx(ctx).Infof("Renovating idempotency_key for circle_recipient with receiver_wallet_id %q and status %s", dataRecipient.ReceiverWalletID, dataRecipient.Status)
	dataRecipient, err = c.sdpModels.CircleRecipient.Update(ctx, dataRecipient.ReceiverWalletID, data.CircleRecipientUpdate{
		IdempotencyKey: uuid.NewString(),
	})
	if err != nil {
		return nil, fmt.Errorf("updating Circle recipient's idempotency key: %w", err)
	}

	return dataRecipient, nil
}

// submitRecipientToCircle submits the recipient creation request to Circle.
func (c *CirclePaymentDispatcher) submitRecipientToCircle(ctx context.Context, receiverWallet data.ReceiverWallet, dataRecipient *data.CircleRecipient) (*data.CircleRecipient, error) {
	// NULL, PENDING, INACTIVE (with renovated idempotency_key) or FAILED (with renovated idempotency_key) -> (re)submit the recipient creation request
	nickname := receiverWallet.ID
	if receiverWallet.Receiver.PhoneNumber != "" {
		nickname = receiverWallet.Receiver.PhoneNumber
	}
	recipient, err := c.circleService.PostRecipient(ctx, circle.RecipientRequest{
		IdempotencyKey: dataRecipient.IdempotencyKey,
		Address:        receiverWallet.StellarAddress,
		Chain:          circle.StellarChainCode,
		Metadata: circle.RecipientMetadata{
			Nickname: nickname,
			Email:    receiverWallet.Receiver.Email,
		},
	})
	if err != nil {
		// Bump the sync_attempt count if the recipient creation failed
		_, updateErr := c.sdpModels.CircleRecipient.Update(ctx, dataRecipient.ReceiverWalletID, data.CircleRecipientUpdate{
			SyncAttempts:      dataRecipient.SyncAttempts + 1,
			LastSyncAttemptAt: time.Now(),
			ResponseBody:      []byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())),
		})
		if updateErr != nil {
			return nil, fmt.Errorf("updating Circle recipient after postRecipientErr: %w", updateErr)
		}

		return nil, fmt.Errorf("creating Circle recipient: %w", err)
	}

	recipientJson, err := json.Marshal(recipient)
	if err != nil {
		return nil, fmt.Errorf("marshalling Circle recipient: %w", err)
	}

	dataRecipientStatus, err := data.ParseRecipientStatus(recipient.Status)
	if err != nil {
		return nil, fmt.Errorf("parsing Circle recipient status: %w", err)
	}

	dataRecipient, err = c.sdpModels.CircleRecipient.Update(ctx, dataRecipient.ReceiverWalletID, data.CircleRecipientUpdate{
		IdempotencyKey:    dataRecipient.IdempotencyKey,
		CircleRecipientID: recipient.ID,
		Status:            dataRecipientStatus,
		ResponseBody:      recipientJson,
		SyncAttempts:      dataRecipient.SyncAttempts + 1,
		LastSyncAttemptAt: time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("updating Circle recipient: %w", err)
	}

	return dataRecipient, nil
}

// ensureRecipientIsReady ensures that the recipient is ready to receive payments, creating it in the database and in
// the Circle API if needed.
func (c *CirclePaymentDispatcher) ensureRecipientIsReady(ctx context.Context, receiverWallet data.ReceiverWallet) (*data.CircleRecipient, error) {
	dataRecipient, err := c.getOrCreateRecipient(ctx, receiverWallet)
	if err != nil {
		return nil, fmt.Errorf("getting or creating Circle recipient: %w", err)
	}

	// SUCCESS
	if dataRecipient.Status == data.CircleRecipientStatusActive {
		return dataRecipient, nil
	}

	// FAILED or INACTIVE
	dataRecipient, err = c.handleFailedRecipientIfNeeded(ctx, dataRecipient)
	if err != nil {
		return nil, fmt.Errorf("handling failed or inactive recipient: %w", err)
	}

	// NULL, PENDING, INACTIVE (with renovated idempotency_key) or FAILED (with renovated idempotency_key) -> (re)submit the recipient creation request
	dataRecipient, err = c.submitRecipientToCircle(ctx, receiverWallet, dataRecipient)
	if err != nil {
		return nil, fmt.Errorf("submitting recipient to Circle: %w", err)
	}

	return dataRecipient, nil
}

// calls ensureRecipientIsReadyWithRetry with a retry policy.
func (c *CirclePaymentDispatcher) ensureRecipientIsReadyWithRetry(ctx context.Context, receiverWallet data.ReceiverWallet, initialDelay time.Duration) (*data.CircleRecipient, error) {
	if initialDelay <= 0 || initialDelay > time.Second {
		initialDelay = initialBackoffDelay
	}

	var recipient *data.CircleRecipient
	err := retry.Do(
		func() error {
			var err error
			recipient, err = c.ensureRecipientIsReady(ctx, receiverWallet)
			if err != nil {
				if errors.Is(err, ErrCircleRecipientCreationFailedTooManyTimes) {
					return retry.Unrecoverable(err) // Stop retrying on this specific error
				}
				return err
			}

			// Check the recipient status
			if recipient.Status != data.CircleRecipientStatusActive {
				// Retry if the status isn't "completed"
				return fmt.Errorf("recipient not ready, status: %s", recipient.Status)
			}

			// Successful case, no retry needed
			return nil
		},
		retry.Attempts(maxCircleRecipientCreationAttempts), // Maximum attempts
		retry.DelayType(retry.BackOffDelay),                // Exponential backoff
		retry.Delay(initialDelay),                          // Initial delay (optional, default is 100ms)
		retry.Context(ctx),                                 // Respect the context's cancellation
	)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure recipient is ready: %w", err)
	}

	return recipient, nil
}
