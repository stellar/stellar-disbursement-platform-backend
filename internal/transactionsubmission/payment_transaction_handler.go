package transactionsubmission

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// PaymentTransactionHandler implements TransactionHandler for payment transactions
type PaymentTransactionHandler struct {
	engine     *engine.SubmitterEngine
	monitorSvc tssMonitor.TSSMonitorService
}

var _ TransactionHandlerInterface = &PaymentTransactionHandler{}

func NewPaymentTransactionHandler(
	engine *engine.SubmitterEngine,
	monitorSvc tssMonitor.TSSMonitorService,
) (*PaymentTransactionHandler, error) {
	if engine == nil {
		return nil, fmt.Errorf("engine cannot be nil")
	}
	if tssUtils.IsEmpty(monitorSvc) {
		return nil, fmt.Errorf("monitor service cannot be nil")
	}

	return &PaymentTransactionHandler{
		engine:     engine,
		monitorSvc: monitorSvc,
	}, nil
}

func (h *PaymentTransactionHandler) BuildInnerTransaction(ctx context.Context, txJob *TxJob, channelAccountSequenceNum int64, distributionAccount string) (*txnbuild.Transaction, error) {
	if txJob.Transaction.AssetCode == "" {
		return nil, fmt.Errorf("asset code cannot be empty")
	}
	var asset txnbuild.Asset = txnbuild.NativeAsset{}
	if strings.ToUpper(txJob.Transaction.AssetCode) != "XLM" {
		if !strkey.IsValidEd25519PublicKey(txJob.Transaction.AssetIssuer) {
			return nil, fmt.Errorf("invalid asset issuer: %v", txJob.Transaction.AssetIssuer)
		}
		asset = txnbuild.CreditAsset{
			Code:   txJob.Transaction.AssetCode,
			Issuer: txJob.Transaction.AssetIssuer,
		}
	}

	var operation txnbuild.Operation
	var txMemo txnbuild.Memo
	amount := strconv.FormatFloat(txJob.Transaction.Amount, 'f', 6, 32)

	if strkey.IsValidEd25519PublicKey(txJob.Transaction.Destination) {
		memo, err := txJob.Transaction.BuildMemo()
		if err != nil {
			return nil, fmt.Errorf("building memo: %w", err)
		}
		txMemo = memo

		operation = &txnbuild.Payment{
			SourceAccount: distributionAccount,
			Amount:        amount,
			Destination:   txJob.Transaction.Destination,
			Asset:         asset,
		}
	} else if strkey.IsValidContractAddress(txJob.Transaction.Destination) {
		if txJob.Transaction.Memo != "" {
			return nil, fmt.Errorf("memo is not supported for contract destination (%s)", txJob.Transaction.Destination)
		}
		params := txnbuild.PaymentToContractParams{
			NetworkPassphrase: h.engine.SignatureService.NetworkPassphrase(),
			Destination:       txJob.Transaction.Destination,
			Amount:            amount,
			Asset:             asset,
			SourceAccount:     distributionAccount,
		}
		op, err := txnbuild.NewPaymentToContract(params)
		if err != nil {
			return nil, fmt.Errorf("building payment to contract operation: %w", err)
		}
		operation = &op
	} else {
		return nil, fmt.Errorf("invalid destination account (%s)", txJob.Transaction.Destination)
	}

	paymentTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: txJob.ChannelAccount.PublicKey,
				Sequence:  channelAccountSequenceNum,
			},
			Operations: []txnbuild.Operation{
				operation,
			},
			Memo:    txMemo,
			BaseFee: int64(h.engine.MaxBaseFee),
			Preconditions: txnbuild.Preconditions{
				TimeBounds:   txnbuild.NewTimeout(300),                                                 // maximum 5 minutes
				LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)}, // currently, 8-10 ledgers in the future
			},
			IncrementSequenceNum: true,
		},
	)

	return paymentTx, err
}

func (h *PaymentTransactionHandler) BuildSuccessEvent(ctx context.Context, txJob *TxJob) (*events.Message, error) {
	msg := &events.Message{
		Topic:    events.PaymentCompletedTopic,
		Key:      txJob.Transaction.ExternalID,
		TenantID: txJob.Transaction.TenantID,
		Type:     events.PaymentCompletedSuccessType,
		Data: schemas.EventPaymentCompletedData{
			TransactionID:        txJob.Transaction.ID,
			PaymentID:            txJob.Transaction.ExternalID,
			PaymentStatus:        string(data.SuccessPaymentStatus),
			PaymentStatusMessage: "",
			PaymentCompletedAt:   time.Now(),
			StellarTransactionID: txJob.Transaction.StellarTransactionHash.String,
		},
	}

	validationErr := msg.Validate()
	if validationErr != nil {
		return nil, fmt.Errorf("validating message: %w", validationErr)
	}

	return msg, nil
}

func (h *PaymentTransactionHandler) BuildFailureEvent(ctx context.Context, txJob *TxJob, err error) (*events.Message, error) {
	msg := &events.Message{
		Topic:    events.PaymentCompletedTopic,
		Key:      txJob.Transaction.ExternalID,
		TenantID: txJob.Transaction.TenantID,
		Type:     events.PaymentCompletedErrorType,
		Data: schemas.EventPaymentCompletedData{
			TransactionID:        txJob.Transaction.ID,
			PaymentID:            txJob.Transaction.ExternalID,
			PaymentStatus:        string(data.FailedPaymentStatus),
			PaymentStatusMessage: err.Error(),
			PaymentCompletedAt:   time.Now(),
			StellarTransactionID: txJob.Transaction.StellarTransactionHash.String,
		},
	}

	validationErr := msg.Validate()
	if validationErr != nil {
		return nil, fmt.Errorf("validating message: %w", validationErr)
	}

	return msg, nil
}

func (h *PaymentTransactionHandler) MonitorTransactionProcessingStarted(ctx context.Context, txJob *TxJob, jobUUID string) {
	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.PaymentProcessingStartedTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: sdpMonitor.PaymentProcessingStartedLabel,
		},
	)
}

func (h *PaymentTransactionHandler) MonitorTransactionProcessingSuccess(ctx context.Context, txJob *TxJob, jobUUID string) {
	eventType := sdpMonitor.PaymentProcessingSuccessfulLabel
	if txJob.Transaction.AttemptsCount > 1 {
		eventType = sdpMonitor.PaymentReprocessingSuccessfulLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.PaymentTransactionSuccessfulTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			IsHorizonErr:         false,
			TransactionEventType: eventType,
		},
	)
}

func (h *PaymentTransactionHandler) MonitorTransactionProcessingFailed(ctx context.Context, txJob *TxJob, jobUUID string, isRetryable bool, errStack string) {
	eventType := sdpMonitor.PaymentFailedLabel
	if isRetryable {
		eventType = sdpMonitor.PaymentMarkedForReprocessingLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.PaymentErrorTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			IsHorizonErr:         isRetryable,
			ErrStack:             errStack,
			TransactionEventType: eventType,
		},
	)
}

func (h *PaymentTransactionHandler) MonitorTransactionReconciliationSuccess(ctx context.Context, txJob *TxJob, jobUUID string, successType ReconcileSuccessType) {
	paymentEventType := sdpMonitor.PaymentReconciliationTransactionSuccessfulLabel
	if successType == ReconcileReprocessing {
		paymentEventType = sdpMonitor.PaymentReconciliationMarkedForReprocessingLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.PaymentReconciliationSuccessfulTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: paymentEventType,
		},
	)
}

func (h *PaymentTransactionHandler) MonitorTransactionReconciliationFailure(ctx context.Context, txJob *TxJob, jobUUID string, isHorizonErr bool, errStack string) {
	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.PaymentReconciliationFailureTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			IsHorizonErr:         isHorizonErr,
			ErrStack:             errStack,
			TransactionEventType: sdpMonitor.PaymentReconciliationUnexpectedErrorLabel,
		},
	)
}

func (h *PaymentTransactionHandler) RequiresRebuildOnRetry() bool {
	return false
}

func (h *PaymentTransactionHandler) AddContextLoggerFields(transaction *store.Transaction) map[string]interface{} {
	fields := map[string]interface{}{
		"asset":               transaction.AssetCode,
		"destination_account": transaction.Destination,
	}

	if transaction.Memo != "" {
		fields["memo"] = transaction.Memo
		fields["memo_type"] = transaction.MemoType
	}

	return fields
}
