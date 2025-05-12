package transactionsubmission

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/google/uuid"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	rpcClient "github.com/stellar/stellar-rpc/client"
	rpc "github.com/stellar/stellar-rpc/protocol"
)

type WalletCreationTransactionWorker struct {
	dbConnectionPool    db.DBConnectionPool
	txModel             store.TransactionStore
	chAccModel          store.ChannelAccountStore
	engine              *engine.SubmitterEngine
	crashTrackerClient  crashtracker.CrashTrackerClient
	txProcessingLimiter engine.TransactionProcessingLimiter
	monitorSvc          tssMonitor.TSSMonitorService
	eventProducer       events.Producer
	jobUUID             string
	// TODO: move somewhere else
	rpcClient *rpcClient.Client
}

func NewWalletCreationTransactionWorker(
	dbConnectionPool db.DBConnectionPool,
	txModel *store.TransactionModel,
	chAccModel *store.ChannelAccountModel,
	engine *engine.SubmitterEngine,
	crashTrackerClient crashtracker.CrashTrackerClient,
	txProcessingLimiter engine.TransactionProcessingLimiter,
	monitorSvc tssMonitor.TSSMonitorService,
	eventProducer events.Producer,
) (WalletCreationTransactionWorker, error) {
	if dbConnectionPool == nil {
		return WalletCreationTransactionWorker{}, fmt.Errorf("dbConnectionPool cannot be nil")
	}

	if txModel == nil {
		return WalletCreationTransactionWorker{}, fmt.Errorf("txModel cannot be nil")
	}

	if chAccModel == nil {
		return WalletCreationTransactionWorker{}, fmt.Errorf("chAccModel cannot be nil")
	}

	if engine == nil {
		return WalletCreationTransactionWorker{}, fmt.Errorf("engine cannot be nil")
	}

	if crashTrackerClient == nil {
		return WalletCreationTransactionWorker{}, fmt.Errorf("crashTrackerClient cannot be nil")
	}

	if txProcessingLimiter == nil {
		return WalletCreationTransactionWorker{}, fmt.Errorf("txProcessingLimiter cannot be nil")
	}

	if tssUtils.IsEmpty(monitorSvc) {
		return WalletCreationTransactionWorker{}, fmt.Errorf("monitorSvc cannot be nil")
	}

	return WalletCreationTransactionWorker{
		jobUUID:             uuid.NewString(),
		dbConnectionPool:    dbConnectionPool,
		txModel:             txModel,
		chAccModel:          chAccModel,
		engine:              engine,
		crashTrackerClient:  crashTrackerClient,
		txProcessingLimiter: txProcessingLimiter,
		monitorSvc:          monitorSvc,
		eventProducer:       eventProducer,
		rpcClient:           rpcClient.NewClient("https://soroban-testnet.stellar.org", http.DefaultClient),
	}, nil
}

func (tw *WalletCreationTransactionWorker) updateContextLogger(ctx context.Context, job *TxJob) context.Context {
	tx := job.Transaction

	labels := map[string]interface{}{
		// Instance info
		"app_version":     tw.monitorSvc.Version,
		"git_commit_hash": tw.monitorSvc.GitCommitHash,
		// Job info
		"event_id": tw.jobUUID,
		// Transaction info
		"channel_account": job.ChannelAccount.PublicKey,
		"tx_id":           tx.ID,
		"tenant_id":       tx.TenantID,
		"public_key":      tx.PublicKey,
		"credential_id":   tx.CredentialID,
		"wasm_hash":       tx.WasmHash,
		"created_at":      tx.CreatedAt,
		"updated_at":      tx.UpdatedAt,
	}

	if tx.XDRSent.Valid {
		labels["xdr_sent"] = tx.XDRSent.String
	}
	if tx.XDRReceived.Valid {
		labels["xdr_received"] = tx.XDRReceived.String
	}
	if tx.StellarTransactionHash.Valid {
		labels["tx_hash"] = tx.StellarTransactionHash.String
	}

	return log.Set(ctx, log.Ctx(ctx).WithFields(labels))
}

func (tw *WalletCreationTransactionWorker) Run(ctx context.Context, txJob *TxJob) {
	ctx = tw.updateContextLogger(ctx, txJob)
	err := tw.runJob(ctx, txJob)
	if err != nil {
		tw.crashTrackerClient.LogAndReportErrors(ctx, err, "unexpected TSS error")
	}
}

func (tw *WalletCreationTransactionWorker) runJob(ctx context.Context, txJob *TxJob) error {
	if txJob.Transaction.StellarTransactionHash.Valid {
		return tw.reconcileSubmittedTransaction(ctx, txJob)
	} else {
		return tw.processTransactionSubmission(ctx, txJob)
	}
}

func (tw *WalletCreationTransactionWorker) handleFailedTransaction(ctx context.Context, txJob *TxJob, hTxResp horizon.Transaction, hErr *utils.HorizonErrorWrapper) error {
	log.Ctx(ctx).Errorf("🔴 Error processing job: %v", hErr)

	metricsMetadata := tssMonitor.TxMetadata{
		EventID:       tw.jobUUID,
		SrcChannelAcc: txJob.ChannelAccount.PublicKey,
		// TODO: payment event type?
	}
	defer func() {
		tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.WalletCreationErrorTag, metricsMetadata)
	}()

	err := tw.saveResponseXDRIfPresent(ctx, txJob, hTxResp)
	if err != nil {
		return fmt.Errorf("saving response xdr: %w", err)
	}

	var hErrWrapper *utils.HorizonErrorWrapper
	if errors.As(hErr, &hErrWrapper) {
		if hErrWrapper.IsHorizonError() {
			metricsMetadata.IsHorizonErr = true
			tw.txProcessingLimiter.AdjustLimitIfNeeded(hErrWrapper)

			if hErrWrapper.ShouldMarkAsError() {
				var msg *events.Message
				msg, err = tw.buildWalletCreationCompletedEvent(events.WalletCreationCompletedErrorType, &txJob.Transaction, data.FailedCreationStatus, hErrWrapper.Error())
				if err != nil {
					return fmt.Errorf("producing wallet creation completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
				}

				var updatedTx *store.Transaction
				updatedTx, err = tw.txModel.UpdateStatusToError(ctx, txJob.Transaction, hErrWrapper.Error())
				if err != nil {
					return fmt.Errorf("updating transaction status to error: %w", err)
				}
				txJob.Transaction = *updatedTx

				err = events.ProduceEvents(ctx, tw.eventProducer, msg)
				if err != nil {
					return fmt.Errorf("producing wallet creation completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
				}

			} else if hErrWrapper.IsBadSequence() {
				tw.crashTrackerClient.LogAndReportErrors(ctx, hErrWrapper, "tx_bad_seq detected!")
			}
		}

	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

	return nil
}

func (tw *WalletCreationTransactionWorker) unlockJob(ctx context.Context, txJob *TxJob) error {
	_, err := tw.chAccModel.Unlock(ctx, tw.dbConnectionPool, txJob.ChannelAccount.PublicKey)
	if err != nil {
		return fmt.Errorf("unlocking channel account: %w", err)
	}

	_, err = tw.txModel.Unlock(ctx, tw.dbConnectionPool, txJob.Transaction.ID)
	if err != nil {
		return fmt.Errorf("unlocking transaction: %w", err)
	}

	return nil
}

func (tw *WalletCreationTransactionWorker) handleSuccessfulTransaction(ctx context.Context, txJob *TxJob, hTxResp horizon.Transaction) error {
	err := tw.saveResponseXDRIfPresent(ctx, txJob, hTxResp)
	if err != nil {
		return fmt.Errorf("saving response xdr: %w", err)
	}
	if !hTxResp.Successful {
		return fmt.Errorf("transaction not successful: %s", hTxResp.Hash)
	}

	msg, err := tw.buildWalletCreationCompletedEvent(events.WalletCreationCompletedSuccessType, &txJob.Transaction, data.SuccessCreationStatus, "")
	if err != nil {
		return fmt.Errorf("producing wallet creation completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
	}

	updatedTx, err := tw.txModel.UpdateStatusToSuccess(ctx, txJob.Transaction)
	if err != nil {
		return utils.NewTransactionStatusUpdateError("SUCCESS", txJob.Transaction.ID, false, err)
	}
	txJob.Transaction = *updatedTx

	err = events.ProduceEvents(ctx, tw.eventProducer, msg)
	if err != nil {
		return fmt.Errorf("producing wallet creation completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

	log.Ctx(ctx).Infof("🎉 Successfully processed transaction job %v", txJob)

	return nil
}

func (tw *WalletCreationTransactionWorker) processTransactionSubmission(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🚧 Processing transaction submission for job %v...", txJob)

	tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.WalletCreationStartedTag, tssMonitor.TxMetadata{
		EventID:       tw.jobUUID,
		SrcChannelAcc: txJob.ChannelAccount.PublicKey,
		// TODO: payment event type?
	})

	feeBumpTx, err := tw.prepareForSubmission(ctx, txJob)
	if err != nil {
		return fmt.Errorf("preparing bundle for processing: %w", err)
	}

	err = tw.submit(ctx, txJob, feeBumpTx)
	if err != nil {
		return fmt.Errorf("submitting transaction: %w", err)
	}

	return nil
}

func (tw *WalletCreationTransactionWorker) reconcileSubmittedTransaction(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🔍 Reconciling previously submitted transaction %v...", txJob)

	txHash := txJob.Transaction.StellarTransactionHash.String
	txDetail, err := tw.engine.HorizonClient.TransactionDetail(txHash)
	hWrapperErr := utils.NewHorizonErrorWrapper(err)
	if err == nil && txDetail.Successful {
		err = tw.handleSuccessfulTransaction(ctx, txJob, txDetail)
		if err != nil {
			return fmt.Errorf("handling successful transaction: %w", err)
		}
		return nil
	} else if (err != nil || txDetail.Successful) && !hWrapperErr.IsNotFound() {
		log.Ctx(ctx).Warnf("received unexpected horizon error: %v", hWrapperErr)
		return fmt.Errorf("unexpected error: %w", hWrapperErr)
	}

	log.Ctx(ctx).Warnf("Previous transaction didn't make through, marking %v for resubmission", txJob)

	_, err = tw.txModel.PrepareTransactionForReprocessing(ctx, tw.dbConnectionPool, txJob.Transaction.ID)
	if err != nil {
		return fmt.Errorf("pushing back transaction to queue: %w", err)
	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

	return nil
}

func (tw *WalletCreationTransactionWorker) buildWalletCreationCompletedEvent(eventType string, tx *store.Transaction, walletStatus data.CreationStatus, statusMsg string) (*events.Message, error) {
	msg := &events.Message{
		Topic:    events.WalletCreationCompletedTopic,
		Key:      tx.ExternalID,
		TenantID: tx.TenantID,
		Type:     eventType,
		Data: schemas.EventWalletCreationCompletedData{
			TransactionID:        tx.ID,
			WalletAddress:        "TODO",
			WalletStatus:         string(walletStatus),
			WalletStatusMessage:  statusMsg,
			StellarTransactionID: tx.StellarTransactionHash.String,
		},
	}

	err := msg.Validate()
	if err != nil {
		return nil, fmt.Errorf("validating message: %w", err)
	}

	return msg, nil
}

func (tw *WalletCreationTransactionWorker) prepareForSubmission(ctx context.Context, txJob *TxJob) (*txnbuild.FeeBumpTransaction, error) {
	feeBumpTx, err := tw.buildAndSignTransaction(ctx, txJob)
	if err != nil {
		return nil, fmt.Errorf("building transaction: %w", err)
	}

	// Important: We need to save tx hash before submitting a transaction.
	// If the script/server crashes after transaction is submitted but before the response
	// is processed, we can easily determine whether tx was sent or not later using tx hash.
	feeBumpTxHash, err := feeBumpTx.HashHex(tw.engine.SignatureService.NetworkPassphrase())
	if err != nil {
		return nil, fmt.Errorf("hashing transaction for job %v: %w", txJob, err)
	}

	sentXDR, err := feeBumpTx.Base64()
	if err != nil {
		return nil, fmt.Errorf("getting envelopeXDR for job %v: %w", txJob, err)
	}

	updatedTx, err := tw.txModel.UpdateStellarTransactionHashAndXDRSent(ctx, txJob.Transaction.ID, feeBumpTxHash, sentXDR)
	if err != nil {
		return nil, fmt.Errorf("saving transaction metadata for job %v: %w", txJob, err)
	}
	txJob.Transaction = *updatedTx

	return feeBumpTx, nil

}

func (tw *WalletCreationTransactionWorker) buildAndSignTransaction(ctx context.Context, txJob *TxJob) (feeBumpTx *txnbuild.FeeBumpTransaction, err error) {
	distributionAccount, err := tw.engine.DistributionAccountResolver.DistributionAccount(ctx, txJob.Transaction.TenantID)
	if err != nil {
		return nil, fmt.Errorf("resolving distribution account for tenantID=%v: %w", txJob.Transaction.TenantID, err)
	} else if !distributionAccount.IsStellar() {
		return nil, fmt.Errorf("expected distribution account to be a STELLAR account but got %q", distributionAccount.Type)
	}

	decodedAddress, err := strkey.Decode(strkey.VersionByteAccountID, distributionAccount.Address)
	if err != nil {
		return nil, fmt.Errorf("decoding distribution account public key: %w", err)
	}
	var publicKey xdr.Uint256
	copy(publicKey[:], decodedAddress[0:32])
	distributionAccountAddress := xdr.ScAddress{
		Type: xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &xdr.AccountId{
			Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
			Ed25519: &publicKey,
		},
	}

	wasmHashBytes, err := hex.DecodeString(txJob.Transaction.WasmHash)
	if err != nil {
		return nil, fmt.Errorf("decoding wasm hash: %w", err)
	}
	var wasmHashXdr xdr.Hash
	copy(wasmHashXdr[:], wasmHashBytes[0:32])

	publicKeyBytes, err := hex.DecodeString(txJob.Transaction.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decoding passkey public key: %w", err)
	}
	var passkeyPublicKey [65]byte
	copy(passkeyPublicKey[:], publicKeyBytes[0:65])
	argPk, err := xdr.NewScVal(xdr.ScValTypeScvBytes, xdr.ScBytes(passkeyPublicKey[:]))
	if err != nil {
		return nil, fmt.Errorf("creating public_key ScVal: %w", err)
	}

	credentialIdBytes, err := hex.DecodeString(txJob.Transaction.CredentialID)
	if err != nil {
		return nil, fmt.Errorf("decoding credential id: %w", err)
	}
	var credentialIdXdr [16]byte
	copy(credentialIdXdr[:], credentialIdBytes[0:16])
	argCredentialId, err := xdr.NewScVal(xdr.ScValTypeScvBytes, xdr.ScBytes(credentialIdXdr[:]))
	if err != nil {
		return nil, fmt.Errorf("creating credential_id ScVal: %w", err)
	}

	invokeHostFunctionOp := txnbuild.InvokeHostFunction{
		SourceAccount: distributionAccount.Address,
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeCreateContractV2,
			CreateContractV2: &xdr.CreateContractArgsV2{
				ContractIdPreimage: xdr.ContractIdPreimage{
					Type: xdr.ContractIdPreimageTypeContractIdPreimageFromAddress,
					FromAddress: &xdr.ContractIdPreimageFromAddress{
						Address: distributionAccountAddress,
						// Salt:
					},
				},
				Executable: xdr.ContractExecutable{
					Type:     xdr.ContractExecutableTypeContractExecutableWasm,
					WasmHash: &wasmHashXdr,
				},
				ConstructorArgs: []xdr.ScVal{argCredentialId, argPk},
			},
		},
	}

	channelAccount, err := tw.engine.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: txJob.ChannelAccount.PublicKey})
	if err != nil {
		err = fmt.Errorf("getting account detail: %w", err)
		return nil, utils.NewHorizonErrorWrapper(err)
	}

	txnParams := txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: txJob.ChannelAccount.PublicKey,
			Sequence:  channelAccount.Sequence,
		},
		Operations: []txnbuild.Operation{
			&invokeHostFunctionOp,
		},
		BaseFee: int64(tw.engine.MaxBaseFee),
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
			LedgerBounds: &txnbuild.LedgerBounds{
				MaxLedger: uint32(txJob.LockedUntilLedgerNumber)},
		},
		IncrementSequenceNum: true,
	}

	tx, err := txnbuild.NewTransaction(
		txnParams,
	)
	if err != nil {
		return nil, fmt.Errorf("creating transaction: %w", err)
	}

	txEnvelope, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("getting envelopeXDR: %w", err)
	}
	simulationResponse, err := tw.rpcClient.SimulateTransaction(ctx, rpc.SimulateTransactionRequest{
		Transaction: txEnvelope,
	})
	log.Ctx(ctx).Debugf("tx envelope: %s", txEnvelope)
	if err != nil {
		return nil, fmt.Errorf("simulating transaction %s: %w", txEnvelope, err)
	}

	if simulationResponse.Error != "" {
		return nil, fmt.Errorf("simulation error: %s", simulationResponse.Error)
	}

	var auth []xdr.SorobanAuthorizationEntry
	if simulationResponse.Results[0].AuthXDR != nil {
		for _, b64 := range *simulationResponse.Results[0].AuthXDR {
			var a xdr.SorobanAuthorizationEntry
			err := xdr.SafeUnmarshalBase64(b64, &a)
			if err != nil {
				return nil, fmt.Errorf("unmarshalling authXDR: %w", err)
			}
			auth = append(auth, a)
		}
	}
	invokeHostFunctionOp.Auth = auth

	var transactionData xdr.SorobanTransactionData
	err = xdr.SafeUnmarshalBase64(simulationResponse.TransactionDataXDR, &transactionData)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling transactionDataXDR: %w", err)
	}

	transactionExt, err := xdr.NewTransactionExt(1, transactionData)
	if err != nil {
		return nil, fmt.Errorf("creating transactionExt: %w", err)
	}

	invokeHostFunctionOp.Ext = transactionExt

	sorobanFee := int64(transactionExt.SorobanData.ResourceFee)
	adjustedBaseFee := int64(txnParams.BaseFee) - sorobanFee

	txnParams.BaseFee = int64(math.Max(float64(adjustedBaseFee), float64(txnbuild.MinBaseFee)))

	channelAccount, err = tw.engine.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: txJob.ChannelAccount.PublicKey})
	if err != nil {
		err = fmt.Errorf("getting account detail: %w", err)
		return nil, utils.NewHorizonErrorWrapper(err)
	}
	txnParams.SourceAccount = &txnbuild.SimpleAccount{
		AccountID: txJob.ChannelAccount.PublicKey,
		Sequence:  channelAccount.Sequence,
	}

	preparedTx, err := txnbuild.NewTransaction(
		txnParams,
	)
	if err != nil {
		return nil, fmt.Errorf("creating transaction: %w", err)
	}

	// Sign tx for the channel account
	chAccount := schema.TransactionAccount{
		Address: txJob.ChannelAccount.PublicKey,
		Type:    schema.ChannelAccountStellarDB,
	}
	preparedTx, err = tw.engine.SignStellarTransaction(ctx, preparedTx, chAccount, distributionAccount)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in job=%v: %w", txJob, err)
	}

	// Build the fee bump transaction
	feeBumpTx, err = txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      preparedTx,
			FeeAccount: distributionAccount.Address,
			BaseFee:    txnParams.BaseFee,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("creating fee bump transaction: %w", err)
	}

	// Sign fee bump tx for the distribution account
	feeBumpTx, err = tw.engine.SignFeeBumpStellarTransaction(ctx, feeBumpTx, distributionAccount, distributionAccount)
	if err != nil {
		return nil, fmt.Errorf("signing fee bump transaction in job=%v: %w", txJob, err)
	}

	return feeBumpTx, nil
}

func (tw *WalletCreationTransactionWorker) submit(ctx context.Context, txJob *TxJob, feeBumpTx *txnbuild.FeeBumpTransaction) error {
	resp, err := tw.engine.HorizonClient.SubmitFeeBumpTransactionWithOptions(feeBumpTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		err = tw.handleFailedTransaction(ctx, txJob, resp, utils.NewHorizonErrorWrapper(err))
		if err != nil {
			return fmt.Errorf("handling failed transaction: %w", err)
		}
	} else {
		err = tw.handleSuccessfulTransaction(ctx, txJob, resp)
		if err != nil {
			return fmt.Errorf("handling successful transaction: %w", err)
		}
	}

	return nil
}

func (tw *WalletCreationTransactionWorker) saveResponseXDRIfPresent(ctx context.Context, txJob *TxJob, resp horizon.Transaction) error {
	if tssUtils.IsEmpty(resp) {
		return nil
	}

	resultXDR := resp.ResultXdr
	updatedTx, err := tw.txModel.UpdateStellarTransactionXDRReceived(ctx, txJob.Transaction.ID, resultXDR)
	if err != nil {
		return fmt.Errorf("updating XDRReceived(%s) for job %v: %w", resultXDR, txJob, err)
	}
	txJob.Transaction = *updatedTx

	return nil
}
