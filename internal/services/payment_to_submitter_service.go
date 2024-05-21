package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type PaymentToSubmitterServiceInterface interface {
	SendPaymentsReadyToPay(ctx context.Context, paymentsReadyToPay schemas.EventPaymentsReadyToPayData) error
	SendBatchPayments(ctx context.Context, batchSize int) error
}

// Making sure that ServerService implements ServerServiceInterface:
var _ PaymentToSubmitterServiceInterface = (*PaymentToSubmitterService)(nil)

// PaymentToSubmitterService is a service that pushes SDP's ready-to-pay payments to the transaction submission service.
type PaymentToSubmitterService struct {
	sdpModels *data.Models
	tssModel  *txSubStore.TransactionModel
}

func NewPaymentToSubmitterService(models *data.Models, tssDBConnectionPool db.DBConnectionPool) *PaymentToSubmitterService {
	return &PaymentToSubmitterService{
		sdpModels: models,
		tssModel:  txSubStore.NewTransactionModel(tssDBConnectionPool),
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

			var transactions []txSubStore.Transaction
			var failedPayments []*data.Payment
			var pendingPayments []*data.Payment

			for _, payment := range payments {
				// 1. For each payment, validate it is ready to be sent
				err = validatePaymentReadyForSending(payment)
				if err != nil {
					// if payment is not ready for sending, we will mark it as failed later.
					failedPayments = append(failedPayments, payment)
					log.Ctx(ctx).Errorf("Payment %s is not ready for sending. Error=%v", payment.ID, err)
					continue
				}

				// TODO: change TSS to use string amount [SDP-483]
				var amount float64
				amount, err = strconv.ParseFloat(payment.Amount, 64)
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
				pendingPayments = append(pendingPayments, payment)
			}

			// 3. Persist data in Transactions table
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

			// 4. Update payment statuses to `Pending`
			if len(pendingPayments) > 0 {
				numUpdated, err := s.sdpModels.Payment.UpdateStatuses(ctx, sdpDBTx, pendingPayments, data.PendingPaymentStatus)
				if err != nil {
					return fmt.Errorf("updating payment statuses to Pending: %w", err)
				}
				updatedPaymentIDs := make([]string, 0, len(pendingPayments))
				for _, pendingPayment := range pendingPayments {
					updatedPaymentIDs = append(updatedPaymentIDs, pendingPayment.ID)
				}
				log.Ctx(ctx).Infof("Updated %d payments to Pending=%+v", numUpdated, updatedPaymentIDs)
			}

			// 5. Update failed payments statuses to `Failed`
			if len(failedPayments) != 0 {
				numUpdated, err := s.sdpModels.Payment.UpdateStatuses(ctx, sdpDBTx, failedPayments, data.FailedPaymentStatus)
				if err != nil {
					return fmt.Errorf("updating payment statuses to Failed: %w", err)
				}
				failedPaymentIDs := make([]string, 0, len(failedPayments))
				for _, failedPayment := range failedPayments {
					failedPaymentIDs = append(failedPaymentIDs, failedPayment.ID)
				}
				log.Ctx(ctx).Warnf("Updated %d payments to Failed=%+v", numUpdated, failedPaymentIDs)
			}

			return nil
		})
	})
	if outerErr != nil {
		return fmt.Errorf("sending payments ready-to-pay inside syncronized database transactions: %w", outerErr)
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
