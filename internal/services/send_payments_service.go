package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type SendPaymentsServiceInterface interface {
	SendBatchPayments(ctx context.Context, batchSize int) error
}

type SendPaymentsService struct {
	sdpModels *data.Models
	tssModel  *txSubStore.TransactionModel
}

// SendBatchPayments sends payments in batches
func (s SendPaymentsService) SendBatchPayments(ctx context.Context, batchSize int) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		return s.sendBatchPayments(ctx, dbTx, batchSize)
	})
	if err != nil {
		return fmt.Errorf("error sending payments: %w", err)
	}

	return nil
}

// sendBatchPayments sends payments in batches in a transaction
func (s SendPaymentsService) sendBatchPayments(ctx context.Context, dbTx db.DBTransaction, batchSize int) error {
	// 1. Get payments that are ready to be sent. This will lock the rows.
	// Payments Ready to be sent means:
	//    a. Payment is in `READY` status
	//    b. Receiver Wallet is in `REGISTERED` status
	//    c. Disbursement is in `STARTED` status
	payments, err := s.sdpModels.Payment.GetBatchForUpdate(ctx, dbTx, batchSize)
	if err != nil {
		return fmt.Errorf("error getting payments ready to be sent: %w", err)
	}

	var transactions []txSubStore.Transaction
	var failedPayments []*data.Payment
	var pendingPayments []*data.Payment
	for _, payment := range payments {
		// 2. Validate that payments are ready to be sent
		if errValidation := validatePaymentReadyForSending(payment); errValidation != nil {
			// if payment is not ready for sending, we will mark it as failed later.
			failedPayments = append(failedPayments, payment)
			log.Ctx(ctx).Errorf("Payment %s is not ready for sending. Error:%s", payment.ID, errValidation.Error())
			continue
		}

		// TODO: change TSS to use string amount [SDP-483]
		amount, parseErr := strconv.ParseFloat(payment.Amount, 64)
		if parseErr != nil {
			return fmt.Errorf("error parsing payment amount %s for payment %s: %w", payment.Amount, payment.ID, parseErr)
		}
		transaction := txSubStore.Transaction{
			ExternalID:  payment.ID,
			AssetCode:   payment.Asset.Code,
			AssetIssuer: payment.Asset.Issuer,
			Amount:      amount,
			Destination: payment.ReceiverWallet.StellarAddress,
		}
		transactions = append(transactions, transaction)
		pendingPayments = append(pendingPayments, payment)
	}

	// 3. Persist data in Transactions table
	_, err = s.tssModel.BulkInsert(ctx, dbTx, transactions)
	if err != nil {
		return fmt.Errorf("error inserting transactions: %w", err)
	}
	// 4. Update payment statuses to `Pending`
	err = s.sdpModels.Payment.UpdateStatuses(ctx, dbTx, pendingPayments, data.PendingPaymentStatus)
	if err != nil {
		return fmt.Errorf("error updating payment statuses to Pending: %w", err)
	}

	// 5. Update failed payments statuses to `Failed`
	if len(failedPayments) != 0 {
		err = s.sdpModels.Payment.UpdateStatuses(ctx, dbTx, failedPayments, data.FailedPaymentStatus)
		if err != nil {
			return fmt.Errorf("error updating payment statuses to Failed: %w", err)
		}
	}
	return nil
}

// ValidateReadyForSending validate that payment is ready for sending
//  1. Check Statuses of Payment, Receiver Wallet, and Disbursement
//  2. Check required fields are not empty.
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

func NewSendPaymentsService(models *data.Models) *SendPaymentsService {
	return &SendPaymentsService{
		sdpModels: models,
		tssModel:  txSubStore.NewTransactionModel(models.DBConnectionPool),
	}
}

// Making sure that ServerService implements ServerServiceInterface
var _ SendPaymentsServiceInterface = (*SendPaymentsService)(nil)
