package transactionsubmission

import (
	"context"
	"encoding/base64"
	"fmt"

	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"

	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	// SponsoredTransactionTimeoutSeconds is the timeout in seconds for sponsored transactions
	SponsoredTransactionTimeoutSeconds = 300
)

type SponsoredTransactionHandler struct {
	engine     *engine.SubmitterEngine
	rpcClient  stellar.RPCClient
	monitorSvc tssMonitor.TSSMonitorService
}

var _ TransactionHandlerInterface = &SponsoredTransactionHandler{}

func NewSponsoredTransactionHandler(
	engine *engine.SubmitterEngine,
	rpcClient stellar.RPCClient,
	monitorSvc tssMonitor.TSSMonitorService,
) (*SponsoredTransactionHandler, error) {
	if engine == nil {
		return nil, fmt.Errorf("engine cannot be nil")
	}
	if rpcClient == nil {
		return nil, fmt.Errorf("rpcClient cannot be nil")
	}
	if tssUtils.IsEmpty(monitorSvc) {
		return nil, fmt.Errorf("monitorSvc cannot be nil")
	}

	return &SponsoredTransactionHandler{
		engine:     engine,
		rpcClient:  rpcClient,
		monitorSvc: monitorSvc,
	}, nil
}

func (h *SponsoredTransactionHandler) BuildInnerTransaction(ctx context.Context, txJob *TxJob, channelAccountSequenceNum int64, distributionAccount string) (*txnbuild.Transaction, error) {
	if txJob.Transaction.Sponsored.SponsoredAccount == "" {
		return nil, fmt.Errorf("sponsored account cannot be empty")
	}
	if txJob.Transaction.Sponsored.SponsoredOperationXDR == "" {
		return nil, fmt.Errorf("sponsored operation XDR cannot be empty")
	}

	if !strkey.IsValidContractAddress(txJob.Transaction.Sponsored.SponsoredAccount) {
		return nil, fmt.Errorf("sponsored account is not a valid contract address")
	}

	xdrBytes, err := base64.StdEncoding.DecodeString(txJob.Transaction.Sponsored.SponsoredOperationXDR)
	if err != nil {
		return nil, fmt.Errorf("sponsored operation XDR is not valid base64: %w", err)
	}

	var operation xdr.InvokeHostFunctionOp
	err = xdr.SafeUnmarshal(xdrBytes, &operation)
	if err != nil {
		return nil, fmt.Errorf("sponsored operation XDR is not valid: %w", err)
	}

	if operation.HostFunction.Type != xdr.HostFunctionTypeHostFunctionTypeInvokeContract {
		return nil, fmt.Errorf("unsupported host function type: %v", operation.HostFunction.Type)
	}
	if operation.HostFunction.InvokeContract == nil {
		return nil, fmt.Errorf("invoke contract operation is missing contract details")
	}

	if len(operation.Auth) != 0 {
		channelAccountID := xdr.MustAddress(txJob.ChannelAccount.PublicKey)
		distributionAccountID := xdr.MustAddress(distributionAccount)

		for _, auth := range operation.Auth {
			if auth.Credentials.Type != xdr.SorobanCredentialsTypeSorobanCredentialsAddress {
				return nil, fmt.Errorf("invalid auth credentials type")
			}
			if auth.Credentials.Address == nil {
				return nil, fmt.Errorf("auth credentials address cannot be nil")
			}

			authAddress := auth.Credentials.Address.Address
			switch authAddress.Type {
			case xdr.ScAddressTypeScAddressTypeAccount:
				if authAddress.AccountId == nil {
					return nil, fmt.Errorf("auth credentials account ID cannot be nil")
				}
				authAccountID := *authAddress.AccountId

				// Ensure the inner operation doesn't require auth from infrastructure accounts.
				if authAccountID.Equals(channelAccountID) {
					return nil, fmt.Errorf("sponsored operation cannot require authorization from channel account")
				}
				if authAccountID.Equals(distributionAccountID) {
					return nil, fmt.Errorf("sponsored operation cannot require authorization from distribution account")
				}
			case xdr.ScAddressTypeScAddressTypeMuxedAccount:
				if authAddress.MuxedAccount == nil {
					return nil, fmt.Errorf("auth credentials muxed account cannot be nil")
				}

				// For muxed auth, compare the underlying ed25519 key.
				authMuxedEd25519 := authAddress.MuxedAccount.Ed25519
				if authMuxedEd25519 == channelAccountID.MustEd25519() {
					return nil, fmt.Errorf("sponsored operation cannot require authorization from channel account")
				}
				if authMuxedEd25519 == distributionAccountID.MustEd25519() {
					return nil, fmt.Errorf("sponsored operation cannot require authorization from distribution account")
				}
			case xdr.ScAddressTypeScAddressTypeContract:
			case xdr.ScAddressTypeScAddressTypeClaimableBalance:
			case xdr.ScAddressTypeScAddressTypeLiquidityPool:
			}
		}
	}

	sponsoredOperation := &txnbuild.InvokeHostFunction{
		SourceAccount: distributionAccount,
		HostFunction:  operation.HostFunction,
		Auth:          operation.Auth,
	}

	txParams := txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: txJob.ChannelAccount.PublicKey,
			Sequence:  channelAccountSequenceNum + 1,
		},
		Operations: []txnbuild.Operation{sponsoredOperation},
		BaseFee:    int64(h.engine.MaxBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds:   txnbuild.NewTimeout(SponsoredTransactionTimeoutSeconds),
			LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)},
		},
	}

	initialTx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("building initial transaction: %w", err)
	}

	txEnvelope, err := initialTx.Base64()
	if err != nil {
		return nil, fmt.Errorf("encoding transaction envelope: %w", err)
	}

	simulationResult, simulationErr := h.rpcClient.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{
		Transaction: txEnvelope,
	})
	if simulationErr != nil {
		return nil, utils.NewRPCErrorWrapper(simulationErr)
	}

	simulationResponse := simulationResult.Response

	if applyErr := h.applyTransactionData(sponsoredOperation, simulationResponse); applyErr != nil {
		return nil, fmt.Errorf("applying transaction data: %w", applyErr)
	}

	preparedTx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("building final transaction: %w", err)
	}

	return preparedTx, nil
}

func (h *SponsoredTransactionHandler) RequiresRebuildOnRetry() bool {
	return false
}

func (h *SponsoredTransactionHandler) applyTransactionData(operation *txnbuild.InvokeHostFunction, simulationResponse protocol.SimulateTransactionResponse) error {
	if simulationResponse.TransactionDataXDR == "" {
		return nil
	}

	var transactionData xdr.SorobanTransactionData
	if err := xdr.SafeUnmarshalBase64(simulationResponse.TransactionDataXDR, &transactionData); err != nil {
		return fmt.Errorf("unmarshaling transaction data: %w", err)
	}

	operation.Ext = xdr.TransactionExt{
		V:           1,
		SorobanData: &transactionData,
	}
	return nil
}

func (h *SponsoredTransactionHandler) AddContextLoggerFields(transaction *store.Transaction) map[string]interface{} {
	fields := map[string]interface{}{
		"sponsored_account": transaction.Sponsored.SponsoredAccount,
	}

	return fields
}

func (h *SponsoredTransactionHandler) MonitorTransactionProcessingStarted(ctx context.Context, txJob *TxJob, jobUUID string) {
	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.SponsoredTransactionProcessingStartedTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: sdpMonitor.SponsoredTransactionProcessingStartedLabel,
		},
	)
}

func (h *SponsoredTransactionHandler) MonitorTransactionProcessingSuccess(ctx context.Context, txJob *TxJob, jobUUID string) {
	eventType := sdpMonitor.SponsoredTransactionProcessingSuccessfulLabel
	if txJob.Transaction.AttemptsCount > 1 {
		eventType = sdpMonitor.SponsoredTransactionReprocessingSuccessfulLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.SponsoredTransactionTransactionSuccessfulTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: eventType,
			IsHorizonErr:         false,
		},
	)
}

func (h *SponsoredTransactionHandler) MonitorTransactionProcessingFailed(ctx context.Context, txJob *TxJob, jobUUID string, isRetryable bool, errStack string) {
	eventType := sdpMonitor.SponsoredTransactionFailedLabel
	if isRetryable {
		eventType = sdpMonitor.SponsoredTransactionMarkedForReprocessingLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.SponsoredTransactionErrorTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: eventType,
			IsHorizonErr:         isRetryable,
			ErrStack:             errStack,
		},
	)
}

func (h *SponsoredTransactionHandler) MonitorTransactionReconciliationSuccess(ctx context.Context, txJob *TxJob, jobUUID string, successType ReconcileSuccessType) {
	sponsoredTransactionEventType := sdpMonitor.SponsoredTransactionReconciliationTransactionSuccessfulLabel
	if successType == ReconcileReprocessing {
		sponsoredTransactionEventType = sdpMonitor.SponsoredTransactionReconciliationMarkedForReprocessingLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.SponsoredTransactionReconciliationSuccessfulTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: sponsoredTransactionEventType,
		},
	)
}

func (h *SponsoredTransactionHandler) MonitorTransactionReconciliationFailure(ctx context.Context, txJob *TxJob, jobUUID string, isHorizonErr bool, errStack string) {
	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.SponsoredTransactionReconciliationFailureTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: sdpMonitor.SponsoredTransactionReconciliationUnexpectedErrorLabel,
			IsHorizonErr:         isHorizonErr,
			ErrStack:             errStack,
		},
	)
}
