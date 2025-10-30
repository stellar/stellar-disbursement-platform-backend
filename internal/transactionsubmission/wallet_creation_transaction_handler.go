package transactionsubmission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stellar/stellar-rpc/protocol"

	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type WalletCreationTransactionHandler struct {
	engine     *engine.SubmitterEngine
	rpcClient  stellar.RPCClient
	monitorSvc tssMonitor.TSSMonitorService
}

var _ TransactionHandlerInterface = &WalletCreationTransactionHandler{}

func NewWalletCreationTransactionHandler(
	engine *engine.SubmitterEngine,
	rpcClient stellar.RPCClient,
	monitorSvc tssMonitor.TSSMonitorService,
) (*WalletCreationTransactionHandler, error) {
	if engine == nil {
		return nil, fmt.Errorf("engine cannot be nil")
	}
	if rpcClient == nil {
		return nil, fmt.Errorf("rpc client cannot be nil")
	}
	if tssUtils.IsEmpty(monitorSvc) {
		return nil, fmt.Errorf("monitor service cannot be nil")
	}

	return &WalletCreationTransactionHandler{
		engine:     engine,
		rpcClient:  rpcClient,
		monitorSvc: monitorSvc,
	}, nil
}

const (
	expectedPublicKeyLength = 65
	expectedWasmHashLength  = 32
)

func (h *WalletCreationTransactionHandler) BuildInnerTransaction(ctx context.Context, txJob *TxJob, channelAccountSequenceNum int64, distributionAccount string) (*txnbuild.Transaction, error) {
	if txJob.Transaction.WalletCreation.PublicKey == "" {
		return nil, fmt.Errorf("public key cannot be empty")
	}
	if txJob.Transaction.WalletCreation.WasmHash == "" {
		return nil, fmt.Errorf("wasm hash cannot be empty")
	}

	publicKeyBytes, err := hex.DecodeString(txJob.Transaction.WalletCreation.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decoding public key: %w", err)
	}
	if len(publicKeyBytes) != expectedPublicKeyLength {
		return nil, fmt.Errorf("public key must be %d bytes, got %d", expectedPublicKeyLength, len(publicKeyBytes))
	}

	wasmHashBytes, err := hex.DecodeString(txJob.Transaction.WalletCreation.WasmHash)
	if err != nil {
		return nil, fmt.Errorf("decoding wasm hash: %w", err)
	}
	if len(wasmHashBytes) != expectedWasmHashLength {
		return nil, fmt.Errorf("wasm hash must be %d bytes, got %d", expectedWasmHashLength, len(wasmHashBytes))
	}

	wasmHash := xdr.Hash(wasmHashBytes)
	publicKeyHash := sha256.Sum256(publicKeyBytes)
	salt := xdr.Uint256(publicKeyHash)

	distributionAccountID := xdr.MustAddress(distributionAccount)
	distributionScAddress := xdr.ScAddress{
		Type:      xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &distributionAccountID,
	}
	argAdmin := xdr.ScVal{
		Type:    xdr.ScValTypeScvAddress,
		Address: &distributionScAddress,
	}

	publicKeyScBytes := xdr.ScBytes(publicKeyBytes)
	argPublicKey := xdr.ScVal{
		Type:  xdr.ScValTypeScvBytes,
		Bytes: &publicKeyScBytes,
	}
	hostFunction := xdr.HostFunction{
		Type: xdr.HostFunctionTypeHostFunctionTypeCreateContractV2,
		CreateContractV2: &xdr.CreateContractArgsV2{
			ContractIdPreimage: xdr.ContractIdPreimage{
				Type: xdr.ContractIdPreimageTypeContractIdPreimageFromAddress,
				FromAddress: &xdr.ContractIdPreimageFromAddress{
					Address: distributionScAddress,
					Salt:    salt,
				},
			},
			Executable: xdr.ContractExecutable{
				Type:     xdr.ContractExecutableTypeContractExecutableWasm,
				WasmHash: &wasmHash,
			},
			ConstructorArgs: []xdr.ScVal{argAdmin, argPublicKey},
		},
	}

	operation := &txnbuild.InvokeHostFunction{
		SourceAccount: distributionAccount,
		HostFunction:  hostFunction,
		Auth:          []xdr.SorobanAuthorizationEntry{},
	}

	txParams := txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: txJob.ChannelAccount.PublicKey,
			Sequence:  channelAccountSequenceNum + 1,
		},
		Operations: []txnbuild.Operation{operation},
		BaseFee:    int64(h.engine.MaxBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds:   txnbuild.NewTimeout(300),
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

	operation.Auth, err = h.extractAuthEntries(simulationResponse)
	if err != nil {
		return nil, fmt.Errorf("extracting auth entries: %w", err)
	}

	if applyErr := h.applyTransactionData(operation, simulationResponse); applyErr != nil {
		return nil, fmt.Errorf("applying transaction data: %w", applyErr)
	}

	preparedTx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("building final transaction: %w", err)
	}

	return preparedTx, nil
}

func (h *WalletCreationTransactionHandler) extractAuthEntries(simulationResponse protocol.SimulateTransactionResponse) ([]xdr.SorobanAuthorizationEntry, error) {
	var auth []xdr.SorobanAuthorizationEntry
	if len(simulationResponse.Results) > 0 && simulationResponse.Results[0].AuthXDR != nil {
		for _, b64 := range *simulationResponse.Results[0].AuthXDR {
			var a xdr.SorobanAuthorizationEntry
			err := xdr.SafeUnmarshalBase64(b64, &a)
			if err != nil {
				return nil, fmt.Errorf("unmarshalling authorization entry: %w", err)
			}
			auth = append(auth, a)
		}
	}
	return auth, nil
}

func (h *WalletCreationTransactionHandler) applyTransactionData(operation *txnbuild.InvokeHostFunction, simulationResponse protocol.SimulateTransactionResponse) error {
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

func (h *WalletCreationTransactionHandler) MonitorTransactionProcessingStarted(ctx context.Context, txJob *TxJob, jobUUID string) {
	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.WalletCreationProcessingStartedTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: sdpMonitor.WalletCreationProcessingStartedLabel,
		},
	)
}

func (h *WalletCreationTransactionHandler) MonitorTransactionProcessingSuccess(ctx context.Context, txJob *TxJob, jobUUID string) {
	eventType := sdpMonitor.WalletCreationProcessingSuccessfulLabel
	if txJob.Transaction.AttemptsCount > 1 {
		eventType = sdpMonitor.WalletCreationReprocessingSuccessfulLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.WalletCreationTransactionSuccessfulTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			IsHorizonErr:         false,
			TransactionEventType: eventType,
		},
	)
}

func (h *WalletCreationTransactionHandler) MonitorTransactionProcessingFailed(ctx context.Context, txJob *TxJob, jobUUID string, isRetryable bool, errStack string) {
	eventType := sdpMonitor.WalletCreationFailedLabel
	if isRetryable {
		eventType = sdpMonitor.WalletCreationMarkedForReprocessingLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.WalletCreationErrorTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			IsHorizonErr:         isRetryable,
			ErrStack:             errStack,
			TransactionEventType: eventType,
		},
	)
}

func (h *WalletCreationTransactionHandler) MonitorTransactionReconciliationSuccess(ctx context.Context, txJob *TxJob, jobUUID string, successType ReconcileSuccessType) {
	walletCreationEventType := sdpMonitor.WalletCreationReconciliationTransactionSuccessfulLabel
	if successType == ReconcileReprocessing {
		walletCreationEventType = sdpMonitor.WalletCreationReconciliationMarkedForReprocessingLabel
	}

	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.WalletCreationReconciliationSuccessfulTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			TransactionEventType: walletCreationEventType,
		},
	)
}

func (h *WalletCreationTransactionHandler) MonitorTransactionReconciliationFailure(ctx context.Context, txJob *TxJob, jobUUID string, isHorizonErr bool, errStack string) {
	h.monitorSvc.LogAndMonitorTransaction(
		ctx,
		txJob.Transaction,
		sdpMonitor.WalletCreationReconciliationFailureTag,
		tssMonitor.TxMetadata{
			EventID:              jobUUID,
			SrcChannelAcc:        txJob.ChannelAccount.PublicKey,
			IsHorizonErr:         isHorizonErr,
			ErrStack:             errStack,
			TransactionEventType: sdpMonitor.WalletCreationReconciliationUnexpectedErrorLabel,
		},
	)
}

func (h *WalletCreationTransactionHandler) RequiresRebuildOnRetry() bool {
	return true // Wallet creation transactions need fresh simulation data
}

func (h *WalletCreationTransactionHandler) AddContextLoggerFields(transaction *store.Transaction) map[string]interface{} {
	fields := map[string]interface{}{
		"public_key": transaction.WalletCreation.PublicKey,
		"wasm_hash":  transaction.WalletCreation.WasmHash,
	}

	return fields
}
