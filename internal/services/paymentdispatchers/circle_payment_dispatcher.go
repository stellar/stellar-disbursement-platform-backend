package paymentdispatchers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
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

var _ PaymentDispatcherInterface = (*CirclePaymentDispatcher)(nil)

func (c *CirclePaymentDispatcher) sendPaymentsToCircle(ctx context.Context, sdpDBTx db.DBTransaction, circleWalletID string, paymentsToSubmit []*data.Payment) error {
	for _, payment := range paymentsToSubmit {
		// 1. Create a new circle transfer request
		transferRequest, err := c.sdpModels.CircleTransferRequests.GetOrInsert(ctx, payment.ID)
		if err != nil {
			return fmt.Errorf("inserting circle transfer request: %w", err)
		}

		// 2. Submit the payment to Circle
		transfer, err := c.circleService.SendPayment(ctx, circle.PaymentRequest{
			SourceWalletID:            circleWalletID,
			DestinationStellarAddress: payment.ReceiverWallet.StellarAddress,
			Amount:                    payment.Amount,
			StellarAssetCode:          payment.Asset.Code,
			IdempotencyKey:            transferRequest.IdempotencyKey,
		})

		if err != nil {
			// 3. If the transfer fails, set the payment status to failed
			log.Ctx(ctx).Errorf("Failed to submit payment %s to Circle: %v", payment.ID, err)
			err = c.sdpModels.Payment.UpdateStatus(ctx, sdpDBTx, payment.ID, data.FailedPaymentStatus, utils.StringPtr(err.Error()), "")
			if err != nil {
				return fmt.Errorf("marking payment as failed: %w", err)
			}
		} else {
			// 4. Update the circle transfer request with the response from Circle
			if err = c.updateCircleTransferRequest(ctx, sdpDBTx, circleWalletID, transfer, transferRequest); err != nil {
				return fmt.Errorf("updating circle transfer request: %w", err)
			}

			// 5. Update the payment status based on the transfer status
			if err = c.updatePaymentStatusForCircleTransfer(ctx, sdpDBTx, transfer, payment); err != nil {
				return fmt.Errorf("updating payment status for Circle transfer: %w", err)
			}
		}
	}
	return nil
}

// updateCircleTransferRequest updates the circle_transfer_request table with the response from Circle.
func (c *CirclePaymentDispatcher) updateCircleTransferRequest(
	ctx context.Context,
	sdpDBTx db.DBTransaction,
	circleWalletID string,
	transfer *circle.Transfer,
	transferRequest *data.CircleTransferRequest,
) error {
	if transfer == nil {
		return fmt.Errorf("transfer cannot be nil")
	}

	jsonBody, err := json.Marshal(transfer)
	if err != nil {
		return fmt.Errorf("converting transfer body to json: %w", err)
	}

	var completedAt *time.Time
	circleStatus := data.CircleTransferStatus(transfer.Status)
	if circleStatus.IsCompleted() {
		completedAt = utils.TimePtr(time.Now())
	}

	_, err = c.sdpModels.CircleTransferRequests.Update(ctx, sdpDBTx, transferRequest.IdempotencyKey, data.CircleTransferRequestUpdate{
		CircleTransferID: transfer.ID,
		Status:           circleStatus,
		ResponseBody:     jsonBody,
		SourceWalletID:   circleWalletID,
		CompletedAt:      completedAt,
	})
	if err != nil {
		return fmt.Errorf("updating circle transfer request: %w", err)
	}

	return nil
}

// updatePaymentStatusForCircleTransfer updates the payment status based on the transfer status.
func (c *CirclePaymentDispatcher) updatePaymentStatusForCircleTransfer(ctx context.Context, sdpDBTx db.DBTransaction, transfer *circle.Transfer, payment *data.Payment) error {
	paymentStatus, err := transfer.Status.ToPaymentStatus()
	if err != nil {
		return fmt.Errorf("converting CIRCLE transfer status to SDP Payment status: %w", err)
	}

	statusMsg := fmt.Sprintf("Transfer %s is %s in Circle", transfer.ID, transfer.Status)
	err = c.sdpModels.Payment.UpdateStatus(ctx, sdpDBTx, payment.ID, paymentStatus, &statusMsg, transfer.TransactionHash)
	if err != nil {
		return fmt.Errorf("marking payment as %s: %w", paymentStatus, err)
	}

	return nil
}
