package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type PaymentToSubmitterServiceInterface interface {
	SendPaymentsReadyToPay(ctx context.Context, paymentsReadyToPay schemas.EventPaymentsReadyToPayData) error
	SendBatchPayments(ctx context.Context, batchSize int) error
}

// Making sure that ServerService implements ServerServiceInterface:
var _ PaymentToSubmitterServiceInterface = (*PaymentToSubmitterService)(nil)

// PaymentToSubmitterService is a service that pushes SDP's ready-to-pay payments to the transaction submission service.
type PaymentToSubmitterService struct {
	sdpModels           *data.Models
	tssModel            *txSubStore.TransactionModel
	distAccountResolver signing.DistributionAccountResolver
	circleService       circle.ServiceInterface
}

type PaymentToSubmitterServiceOptions struct {
	Models              *data.Models
	TSSDBConnectionPool db.DBConnectionPool
	DistAccountResolver signing.DistributionAccountResolver
	CircleService       circle.ServiceInterface
}

func NewPaymentToSubmitterService(opts PaymentToSubmitterServiceOptions) *PaymentToSubmitterService {
	return &PaymentToSubmitterService{
		sdpModels:           opts.Models,
		tssModel:            txSubStore.NewTransactionModel(opts.TSSDBConnectionPool),
		distAccountResolver: opts.DistAccountResolver,
		circleService:       opts.CircleService,
	}
}

// SendPaymentsReadyToPay sends SDP's ready-to-pay payments (in batches) to the transaction submission service.
func (s PaymentToSubmitterService) SendPaymentsReadyToPay(ctx context.Context, paymentsReadyToPay schemas.EventPaymentsReadyToPayData) error {
	paymentIDs := make([]string, 0, len(paymentsReadyToPay.Payments))
	for _, paymentReadyToPay := range paymentsReadyToPay.Payments {
		paymentIDs = append(paymentIDs, paymentReadyToPay.ID)
	}

	err := s.sendPaymentsReadyToPay(ctx, paymentsReadyToPay.TenantID, func(sdpDBTx db.DBTransaction) ([]*data.Payment, error) {
		log.Ctx(ctx).Infof("Registering %d payments into the TSS, paymentIDs=%v", len(paymentIDs), paymentIDs)

		payments, innerErr := s.sdpModels.Payment.GetReadyByID(ctx, sdpDBTx, paymentIDs...)
		if len(payments) != len(paymentIDs) {
			log.Ctx(ctx).Errorf("[PaymentToSubmitterService] The number of incoming payments to be processed (%d) is different from the number ready to be processed found in the database (%d)", len(paymentIDs), len(payments))
		}

		return payments, innerErr
	})
	if err != nil {
		return fmt.Errorf("sending payments: %w", err)
	}

	return nil
}

// SendBatchPayments sends SDP's ready-to-pay payments (in batches) to the transaction submission service.
func (s PaymentToSubmitterService) SendBatchPayments(ctx context.Context, batchSize int) error {
	t, tenantErr := tenant.GetTenantFromContext(ctx)
	if tenantErr != nil {
		return fmt.Errorf("getting tenant from context: %w", tenantErr)
	}

	err := s.sendPaymentsReadyToPay(ctx, t.ID, func(sdpDBTx db.DBTransaction) ([]*data.Payment, error) {
		return s.sdpModels.Payment.GetBatchForUpdate(ctx, sdpDBTx, batchSize)
	})
	if err != nil {
		return fmt.Errorf("sending payments: %w", err)
	}

	return nil
}

// sendPaymentsReadyToPay sends SDP's ready-to-pay payments to the transaction submission service, using two DB
// transactions (for SDP and TSS), in order to guarantee that the data is consistent in both data stores.
//
// Payments ready-to-pay meet all the following conditions:
//
//   - Payment is in `READY` status
//   - Receiver Wallet is in `REGISTERED` status
//   - Disbursement is in `STARTED` status.
func (s PaymentToSubmitterService) sendPaymentsReadyToPay(
	ctx context.Context,
	tenantID string,
	getPaymentsFn func(sdpDBTx db.DBTransaction) ([]*data.Payment, error),
) error {
	outerErr := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			payments, err := getPaymentsFn(sdpDBTx)
			if err != nil {
				return fmt.Errorf("getting payments ready to be sent: %w", err)
			}

			var failedPayments []*data.Payment
			var pendingPayments []*data.Payment

			// 1. For each payment, validate it is ready to be sent
			for _, payment := range payments {
				err = validatePaymentReadyForSending(payment)
				if err != nil {
					// if payment is not ready for sending, we will mark it as failed later.
					failedPayments = append(failedPayments, payment)
					log.Ctx(ctx).Errorf("Payment %s is not ready for sending. Error=%v", payment.ID, err)
					continue
				}

				pendingPayments = append(pendingPayments, payment)
			}

			// 2. Update failed payments statuses to `Failed`. These payments won't even be attempted.
			if err = s.markPaymentsAsFailed(ctx, sdpDBTx, failedPayments); err != nil {
				return fmt.Errorf("marking payments as failed: %w", err)
			}

			// 3. Submit Payments to proper platform (TSS or Circle)
			err = s.sendPaymentsToProperPlatform(ctx, sdpDBTx, tssDBTx, tenantID, pendingPayments)
			if err != nil {
				return fmt.Errorf("sending payments to target platform: %w", err)
			}

			return nil
		})
	})
	if outerErr != nil {
		return fmt.Errorf("sending payments ready-to-pay inside syncronized database transactions: %w", outerErr)
	}

	return nil
}

func (s PaymentToSubmitterService) markPaymentsAsFailed(ctx context.Context, sdpDBTx db.DBTransaction, failedPayments []*data.Payment) error {
	if len(failedPayments) == 0 {
		return nil
	}

	numUpdated, err := s.sdpModels.Payment.UpdateStatuses(ctx, sdpDBTx, failedPayments, data.FailedPaymentStatus)
	if err != nil {
		return fmt.Errorf("updating payment statuses to Failed: %w", err)
	}

	failedPaymentIDs := make([]string, 0, len(failedPayments))
	for _, failedPayment := range failedPayments {
		failedPaymentIDs = append(failedPaymentIDs, failedPayment.ID)
	}
	log.Ctx(ctx).Warnf("Updated %d payments to Failed=%+v", numUpdated, failedPaymentIDs)

	return nil
}

func (s PaymentToSubmitterService) sendPaymentsToProperPlatform(ctx context.Context, sdpDBTx, tssDBTx db.DBTransaction, tenantID string, paymentsToSubmit []*data.Payment) error {
	if len(paymentsToSubmit) == 0 {
		return nil
	}

	distAccount, err := s.distAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}

	switch distAccount.Type.Platform() {
	case schema.CirclePlatform:
		return s.sendPaymentsToCircle(ctx, sdpDBTx, distAccount.CircleWalletID, paymentsToSubmit)
	case schema.StellarPlatform:
		return s.sendPaymentsToTSS(ctx, sdpDBTx, tssDBTx, tenantID, paymentsToSubmit)
	default:
		return fmt.Errorf("unknown platform type: %s", distAccount.Type.Platform())
	}
}

func (s PaymentToSubmitterService) sendPaymentsToCircle(ctx context.Context, sdpDBTx db.DBTransaction, circleWalletID string, paymentsToSubmit []*data.Payment) error {
	for _, payment := range paymentsToSubmit {
		// 1. Create a new circle transfer request
		transferRequest, err := s.sdpModels.CircleTransferRequests.GetOrInsert(ctx, payment.ID)
		if err != nil {
			return fmt.Errorf("inserting circle transfer request: %w", err)
		}

		// 2. Submit the payment to Circle
		transfer, err := s.circleService.SendPayment(ctx, circle.PaymentRequest{
			SourceWalletID:            circleWalletID,
			DestinationStellarAddress: payment.ReceiverWallet.StellarAddress,
			Amount:                    payment.Amount,
			StellarAssetCode:          payment.Asset.Code,
			IdempotencyKey:            transferRequest.IdempotencyKey,
		})

		if err != nil {
			// 3. If the transfer fails, set the payment status to failed
			log.Ctx(ctx).Errorf("Failed to submit payment %s to Circle: %v", payment.ID, err)
			// TODO: [SDP-1245] if the transfer fails because of authentication error, set the account status to `PENDING_USER_ACTIVATION`
			err = s.sdpModels.Payment.UpdateStatus(ctx, sdpDBTx, payment.ID, data.FailedPaymentStatus, utils.StringPtr(err.Error()), "")
			if err != nil {
				return fmt.Errorf("marking payment as failed: %w", err)
			}
		} else {
			// 4. Update the circle transfer request with the response from Circle
			if err = s.updateCircleTransferRequest(ctx, sdpDBTx, circleWalletID, transfer, transferRequest); err != nil {
				return fmt.Errorf("updating circle transfer request: %w", err)
			}

			// 5. Update the payment status based on the transfer status
			if err = s.updatePaymentStatusForCircleTransfer(ctx, sdpDBTx, transfer, payment); err != nil {
				return fmt.Errorf("updating payment status for Circle transfer: %w", err)
			}
		}
	}
	return nil
}

// updateCircleTransferRequest updates the circle_transfer_request table with the response from Circle.
func (s PaymentToSubmitterService) updateCircleTransferRequest(
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

	_, err = s.sdpModels.CircleTransferRequests.Update(ctx, sdpDBTx, transferRequest.IdempotencyKey, data.CircleTransferRequestUpdate{
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
func (s PaymentToSubmitterService) updatePaymentStatusForCircleTransfer(ctx context.Context, sdpDBTx db.DBTransaction, transfer *circle.Transfer, payment *data.Payment) error {
	paymentStatus, err := transfer.Status.ToPaymentStatus()
	if err != nil {
		return fmt.Errorf("converting CIRCLE transfer status to SDP Payment status: %w", err)
	}

	statusMsg := fmt.Sprintf("Transfer %s is %s in Circle", transfer.ID, transfer.Status)
	err = s.sdpModels.Payment.UpdateStatus(ctx, sdpDBTx, payment.ID, paymentStatus, &statusMsg, transfer.TransactionHash)
	if err != nil {
		return fmt.Errorf("marking payment as %s: %w", paymentStatus, err)
	}

	return nil
}

func (s PaymentToSubmitterService) sendPaymentsToTSS(ctx context.Context, sdpDBTx, tssDBTx db.DBTransaction, tenantID string, pendingPayments []*data.Payment) error {
	var transactions []txSubStore.Transaction
	for _, payment := range pendingPayments {
		// TODO: change TSS to use string amount [SDP-483]
		amount, err := strconv.ParseFloat(payment.Amount, 64)
		if err != nil {
			return fmt.Errorf("parsing payment amount %s for payment ID %s: %w", payment.Amount, payment.ID, err)
		}

		transaction := txSubStore.Transaction{
			ExternalID:  payment.ID,
			AssetCode:   payment.Asset.Code,
			AssetIssuer: payment.Asset.Issuer,
			Amount:      amount,
			Destination: payment.ReceiverWallet.StellarAddress,
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

// validatePaymentReadyForSending validates that a payment is ready for sending, by:
//  1. checking the statuses of Payment, Receiver Wallet, and Disbursement.
//  2. checking that the required fields are not empty.
func validatePaymentReadyForSending(p *data.Payment) error {
	// check statuses
	if p.Status != data.ReadyPaymentStatus {
		return fmt.Errorf("payment %s is not in %s state", p.ID, data.ReadyPaymentStatus)
	}
	if p.ReceiverWallet.Status != data.RegisteredReceiversWalletStatus {
		return fmt.Errorf("receiver wallet %s for payment %s is not in %s state", p.ReceiverWallet.ID, p.ID, data.RegisteredReceiversWalletStatus)
	}
	if p.Disbursement.Status != data.StartedDisbursementStatus {
		return fmt.Errorf("disbursement %s for payment %s is not in %s state", p.Disbursement.ID, p.ID, data.StartedDisbursementStatus)
	}

	// verify that transaction required fields are not empty
	//  1. payment.ID is used as transaction.ExternalID
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("payment ID is empty for Payment")
	}
	// 2. payment.asset.Code is used as transaction.AssetCode
	if strings.TrimSpace(p.Asset.Code) == "" {
		return fmt.Errorf("payment asset code is empty for payment %s", p.ID)
	}
	// 3. payment.asset.Issuer is used as transaction.AssetIssuer
	if strings.TrimSpace(p.Asset.Issuer) == "" && strings.TrimSpace(strings.ToUpper(p.Asset.Code)) != "XLM" {
		return fmt.Errorf("payment asset issuer is empty for payment %s", p.ID)
	}
	// 4. payment.Amount is used as transaction.Amount
	if err := utils.ValidateAmount(p.Amount); err != nil {
		return fmt.Errorf("payment amount is invalid for payment %s", p.ID)
	}
	// 5. payment.ReceiverWallet.StellarAddress is used as transaction.Destination
	if strings.TrimSpace(p.ReceiverWallet.StellarAddress) == "" {
		return fmt.Errorf("payment receiver wallet stellar address is empty for payment %s", p.ID)
	}

	return nil
}
