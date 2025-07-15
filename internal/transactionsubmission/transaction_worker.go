package transactionsubmission

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type TxJob store.ChannelTransactionBundle

func (job TxJob) String() string {
	return fmt.Sprintf("TxJob{ChannelAccount: %q, Transaction: %q, Tenant: %q, LockedUntilLedgerNumber: \"%d\"}", job.ChannelAccount.PublicKey, job.Transaction.ID, job.Transaction.TenantID, job.LockedUntilLedgerNumber)
}

type TransactionWorker struct {
	dbConnectionPool    db.DBConnectionPool
	txModel             store.TransactionStore
	chAccModel          store.ChannelAccountStore
	engine              *engine.SubmitterEngine
	crashTrackerClient  crashtracker.CrashTrackerClient
	txProcessingLimiter engine.TransactionProcessingLimiter
	monitorSvc          tssMonitor.TSSMonitorService
	eventProducer       events.Producer
	jobUUID             string
	txHandler           TransactionHandlerInterface
}

func NewTransactionWorker(
	dbConnectionPool db.DBConnectionPool,
	txModel *store.TransactionModel,
	chAccModel *store.ChannelAccountModel,
	engine *engine.SubmitterEngine,
	crashTrackerClient crashtracker.CrashTrackerClient,
	txProcessingLimiter engine.TransactionProcessingLimiter,
	monitorSvc tssMonitor.TSSMonitorService,
	eventProducer events.Producer,
	txHandler TransactionHandlerInterface,
) (TransactionWorker, error) {
	if dbConnectionPool == nil {
		return TransactionWorker{}, fmt.Errorf("dbConnectionPool cannot be nil")
	}

	if txModel == nil {
		return TransactionWorker{}, fmt.Errorf("txModel cannot be nil")
	}

	if chAccModel == nil {
		return TransactionWorker{}, fmt.Errorf("chAccModel cannot be nil")
	}

	if engine == nil {
		return TransactionWorker{}, fmt.Errorf("engine cannot be nil")
	}
	if err := engine.Validate(); err != nil {
		return TransactionWorker{}, fmt.Errorf("validating engine: %w", err)
	}

	if crashTrackerClient == nil {
		return TransactionWorker{}, fmt.Errorf("crashTrackerClient cannot be nil")
	}

	if txProcessingLimiter == nil {
		return TransactionWorker{}, fmt.Errorf("txProcessingLimiter cannot be nil")
	}

	if tssUtils.IsEmpty(monitorSvc) {
		return TransactionWorker{}, fmt.Errorf("monitorSvc cannot be nil")
	}

	if txHandler == nil {
		return TransactionWorker{}, fmt.Errorf("txHandler cannot be nil")
	}

	return TransactionWorker{
		jobUUID:             uuid.NewString(),
		dbConnectionPool:    dbConnectionPool,
		txModel:             txModel,
		chAccModel:          chAccModel,
		engine:              engine,
		crashTrackerClient:  crashTrackerClient,
		txProcessingLimiter: txProcessingLimiter,
		monitorSvc:          monitorSvc,
		eventProducer:       eventProducer,
		txHandler:           txHandler,
	}, nil
}

// updateContextLogger will update the context logger with the transaction job details.
func (tw *TransactionWorker) updateContextLogger(ctx context.Context, job *TxJob) context.Context {
	tx := job.Transaction

	// Common fields for all transaction types
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
		"created_at":      tx.CreatedAt.String(),
		"updated_at":      tx.UpdatedAt.String(),
	}

	// Add handler-specific fields if we have a handler
	handlerFields := tw.txHandler.AddContextLoggerFields(&tx)
	for k, v := range handlerFields {
		labels[k] = v
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

func (tw *TransactionWorker) Run(ctx context.Context, txJob *TxJob) {
	ctx = tw.updateContextLogger(ctx, txJob)
	err := tw.runJob(ctx, txJob)
	if err != nil {
		tw.crashTrackerClient.LogAndReportErrors(ctx, err, "unexpected TSS error")
	}
}

// TODO: add unit tests and godoc to this function
func (tw *TransactionWorker) runJob(ctx context.Context, txJob *TxJob) error {
	err := tw.validateJob(txJob)
	if err != nil {
		return fmt.Errorf("validating job: %w", err)
	}

	if txJob.Transaction.StellarTransactionHash.Valid {
		return tw.reconcileSubmittedTransaction(ctx, txJob)
	} else {
		return tw.processTransactionSubmission(ctx, txJob)
	}
}

// handleFailedTransaction handles both Horizon and RPC errors through a unified interface.
// This method will only return an error if something goes wrong when handling the result and marking the transaction as ERROR.
//
// Errors that trigger the pause/jitter mechanism at TransactionProcessingLimiter:
//
//	Horizon: 504 (Timeout), 429 (Too Many Requests), 400 tx_insufficient_fee
//	RPC: Network errors, Resource errors
//
// Errors marked as definitive error, that won't be resolved with retries:
//
//	Horizon: 400 with tx_bad_auth, tx_bad_auth_extra, tx_insufficient_balance, or operation error codes
//	RPC: Contract errors, auth failures
//
// Errors that are marked for retry without pause/jitter but are reported to CrashTracker:
//
//	Horizon: 400 tx_bad_seq
//
// Errors that are marked for retry without pause/jitter and are not reported to CrashTracker:
//
//	Horizon: 400 tx_too_late, unexpected errors
//	RPC: unexpected errors
func (tw *TransactionWorker) handleFailedTransaction(ctx context.Context, txJob *TxJob, hTxResp horizon.Transaction, txErr utils.TransactionError) error {
	log.Ctx(ctx).Errorf("🔴 Error processing job (%s): %v", txErr.GetErrorType(), txErr)

	defer tw.txHandler.MonitorTransactionProcessingFailed(ctx, txJob, tw.jobUUID, txErr.IsRetryable(), txErr.Error())

	err := tw.saveResponseXDRIfPresent(ctx, txJob, hTxResp)
	if err != nil {
		return fmt.Errorf("saving response XDR: %w", err)
	}

	tw.txProcessingLimiter.AdjustLimitIfNeeded(txErr)

	if txErr.ShouldMarkAsError() {
		if markErr := tw.markTransactionAsError(ctx, txJob, txErr, txErr.Error()); markErr != nil {
			return markErr
		}

		if txErr.ShouldReportToCrashTracker() {
			tw.crashTrackerClient.LogAndReportErrors(ctx, txErr, fmt.Sprintf("%s transaction error - cannot be retried", strings.ToLower(txErr.GetErrorType())))
		}
	} else {
		if txErr.IsRetryable() && tw.txHandler.RequiresRebuildOnRetry() {
			if _, prepareErr := tw.txModel.PrepareTransactionForReprocessing(ctx, tw.dbConnectionPool, txJob.Transaction.ID); prepareErr != nil {
				return fmt.Errorf("preparing transaction for reprocessing: %w", prepareErr)
			}
		}

		if horizonErr, ok := txErr.(utils.HorizonSpecificError); ok && horizonErr.IsBadSequence() {
			// Special handling for bad sequence errors (Horizon-specific)
			tw.crashTrackerClient.LogAndReportErrors(ctx, txErr, "tx_bad_seq detected!")
		}
	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

	return nil
}

// markTransactionAsError handles the process of marking a transaction as ERROR and producing events
func (tw *TransactionWorker) markTransactionAsError(ctx context.Context, txJob *TxJob, txErr utils.TransactionError, errorMsg string) error {
	// Building the completed event before updating the transaction status. This way, if the message
	// fails to be built, the transaction will be marked for reprocessing -> reconciliation and the event
	// will be re-tried.
	eventMsg, eventErr := tw.txHandler.BuildFailureEvent(ctx, txJob, txErr)
	if eventErr != nil {
		return fmt.Errorf("producing completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, eventErr)
	}

	updatedTx, updateErr := tw.txModel.UpdateStatusToError(ctx, txJob.Transaction, errorMsg)
	if updateErr != nil {
		return fmt.Errorf("updating transaction status to error: %w", updateErr)
	}
	txJob.Transaction = *updatedTx

	// Publishing a new event on the event producer
	err := events.ProduceEvents(ctx, tw.eventProducer, eventMsg)
	if err != nil {
		return fmt.Errorf("producing completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
	}

	return nil
}

// TODO: add tests
// unlockJob will unlock the channel account and transaction instantaneously, so they can be made available ASAP. If
// this method is not called, the algorithm will fall back to get these resources qutomatically unlocked when their
// `locked-to-ledger` expire.
func (tw *TransactionWorker) unlockJob(ctx context.Context, txJob *TxJob) error {
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

// handleSuccessfulTransaction will wrap up the job when the transaction has been successfully submitted to the network.
// This method will only return an error if something goes wromg when handling the result and marking the transaction as SUCCESS.
func (tw *TransactionWorker) handleSuccessfulTransaction(ctx context.Context, txJob *TxJob, hTxResp horizon.Transaction) error {
	err := tw.saveResponseXDRIfPresent(ctx, txJob, hTxResp)
	if err != nil {
		return fmt.Errorf("saving response XDR: %w", err)
	}
	if !hTxResp.Successful {
		return fmt.Errorf("transaction was not successful for some reason")
	}

	// Building the completed event before updating the transaction status. This way, if the message fails to be
	// built, the transaction will be marked for reprocessing -> reconciliation and the event will be re-tried.
	eventMsg, err := tw.txHandler.BuildSuccessEvent(ctx, txJob)
	if err != nil {
		return fmt.Errorf("building completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
	}

	updatedTx, err := tw.txModel.UpdateStatusToSuccess(ctx, txJob.Transaction)
	if err != nil {
		return utils.NewTransactionStatusUpdateError("SUCCESS", txJob.Transaction.ID, false, err)
	}
	txJob.Transaction = *updatedTx

	// Publishing a new event on the event producer
	err = events.ProduceEvents(ctx, tw.eventProducer, eventMsg)
	if err != nil {
		return fmt.Errorf("producing completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

	tw.txHandler.MonitorTransactionProcessingSuccess(ctx, txJob, tw.jobUUID)

	log.Ctx(ctx).Infof("🎉 Successfully processed transaction job %v", txJob)

	return nil
}

// reconcileSubmittedTransaction will check the status of a previously submitted transaction and handle it accordingly.
// If the transaction was successful, it will be marked as such and the job will be unlocked.
// If the transaction failed, it will be marked for resubmission.
func (tw *TransactionWorker) reconcileSubmittedTransaction(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🔍 Reconciling previously submitted transaction %v...", txJob)

	err := tw.validateJob(txJob)
	if err != nil {
		return fmt.Errorf("validating bundle: %w", err)
	}

	txHash := txJob.Transaction.StellarTransactionHash.String
	txDetail, err := tw.engine.HorizonClient.TransactionDetail(txHash)
	hWrapperErr := utils.NewHorizonErrorWrapper(err)

	if err == nil && txDetail.Successful {
		err = tw.handleSuccessfulTransaction(ctx, txJob, txDetail)
		if err != nil {
			tw.txHandler.MonitorTransactionReconciliationFailure(ctx, txJob, tw.jobUUID, false, err.Error())
			return fmt.Errorf("handling successful transaction: %w", err)
		}

		tw.txHandler.MonitorTransactionReconciliationSuccess(ctx, txJob, tw.jobUUID, ReconcileSuccess)
		return nil
	} else if (err != nil || txDetail.Successful) && !hWrapperErr.IsNotFound() {
		log.Ctx(ctx).Warnf("received unexpected horizon error: %v", hWrapperErr)

		tw.txHandler.MonitorTransactionReconciliationFailure(ctx, txJob, tw.jobUUID, true, hWrapperErr.Error())
		return fmt.Errorf("unexpected error: %w", hWrapperErr)
	}

	log.Ctx(ctx).Warnf("Previous transaction didn't make through, marking %v for resubmission...", txJob)

	_, err = tw.txModel.PrepareTransactionForReprocessing(ctx, tw.dbConnectionPool, txJob.Transaction.ID)
	if err != nil {
		return fmt.Errorf("pushing back transaction to queue: %w", err)
	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

	tw.txHandler.MonitorTransactionReconciliationSuccess(ctx, txJob, tw.jobUUID, ReconcileReprocessing)

	return nil
}

func (tw *TransactionWorker) processTransactionSubmission(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🚧 Processing transaction submission for job %v...", txJob)

	tw.txHandler.MonitorTransactionProcessingStarted(ctx, txJob, tw.jobUUID)

	// STEP 1: validate bundle
	err := tw.validateJob(txJob)
	if err != nil {
		return fmt.Errorf("validating bundle: %w", err)
	}

	// STEP 2: prepare transaction for processing
	feeBumpTx, handled, err := tw.prepareForSubmission(ctx, txJob)
	if err != nil {
		if handled {
			return nil
		}
		return fmt.Errorf("preparing bundle for processing: %w", err)
	}

	// STEP 3: process transaction
	err = tw.submit(ctx, txJob, feeBumpTx)
	if err != nil {
		return fmt.Errorf("processing bundle: %w", err)
	}

	return nil
}

// validateJob will check if the job is valid for processing or reconciliation.
func (tw *TransactionWorker) validateJob(txJob *TxJob) error {
	if txJob == nil {
		return fmt.Errorf("transaction job cannot be nil")
	}

	allowedStatuses := []store.TransactionStatus{store.TransactionStatusPending, store.TransactionStatusProcessing}
	if !slices.Contains(allowedStatuses, txJob.Transaction.Status) {
		return fmt.Errorf("invalid transaction status: %v", txJob.Transaction.Status)
	}

	currentLedgerNumber, err := tw.engine.LedgerNumberTracker.GetLedgerNumber()
	if err != nil {
		return fmt.Errorf("getting current ledger number: %w", err)
	}

	if !txJob.Transaction.IsLocked(int32(currentLedgerNumber)) {
		return fmt.Errorf("transaction should be locked")
	}

	if !txJob.ChannelAccount.IsLocked(int32(currentLedgerNumber)) {
		return fmt.Errorf("channel account should be locked")
	}

	return nil
}

func (tw *TransactionWorker) prepareForSubmission(ctx context.Context, txJob *TxJob) (*txnbuild.FeeBumpTransaction, bool, error) {
	feeBumpTx, handled, err := tw.buildAndSignTransaction(ctx, txJob)
	if err != nil {
		return nil, handled, err
	}

	innerTx := feeBumpTx.InnerTransaction()
	distributionAccount := innerTx.Operations()[0].GetSourceAccount()

	// Important: We need to save tx hash before submitting a transaction.
	// If the script/server crashes after transaction is submitted but before the response
	// is processed, we can easily determine whether tx was sent or not later using tx hash.
	feeBumpTxHash, err := feeBumpTx.HashHex(tw.engine.SignatureService.NetworkPassphrase())
	if err != nil {
		return nil, false, fmt.Errorf("hashing transaction for job %v: %w", txJob, err)
	}

	sentXDR, err := feeBumpTx.Base64()
	if err != nil {
		return nil, false, fmt.Errorf("getting envelopeXDR for job %v: %w", txJob, err)
	}

	updatedTx, err := tw.txModel.UpdateStellarTransactionHashXDRSentAndDistributionAccount(ctx, txJob.Transaction.ID, feeBumpTxHash, sentXDR, distributionAccount)
	if err != nil {
		return nil, false, fmt.Errorf("saving transaction metadata for job %v: %w", txJob, err)
	}
	txJob.Transaction = *updatedTx

	return feeBumpTx, false, nil
}

func (tw *TransactionWorker) buildAndSignTransaction(ctx context.Context, txJob *TxJob) (*txnbuild.FeeBumpTransaction, bool, error) {
	distributionAccount, err := tw.engine.DistributionAccountResolver.DistributionAccount(ctx, txJob.Transaction.TenantID)
	if err != nil {
		return nil, false, fmt.Errorf("resolving distribution account for tenantID=%s: %w", txJob.Transaction.TenantID, err)
	} else if !distributionAccount.IsStellar() {
		return nil, false, fmt.Errorf("expected distribution account to be a STELLAR account but got %q", distributionAccount.Type)
	}

	horizonAccount, err := tw.engine.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: txJob.ChannelAccount.PublicKey})
	if err != nil {
		handled, handlerErr := tw.handlePreparationError(ctx, txJob, err)
		return nil, handled, handlerErr
	}

	innerTx, err := tw.txHandler.BuildInnerTransaction(ctx, txJob, horizonAccount.Sequence, distributionAccount.Address)
	if err != nil {
		handled, handlerErr := tw.handlePreparationError(ctx, txJob, err)
		return nil, handled, handlerErr
	}

	// Sign tx for the channel account:
	chAccount := schema.TransactionAccount{
		Address: txJob.ChannelAccount.PublicKey,
		Type:    schema.ChannelAccountStellarDB,
	}
	innerTx, err = tw.engine.SignerRouter.SignStellarTransaction(ctx, innerTx, chAccount, distributionAccount)
	if err != nil {
		return nil, false, fmt.Errorf("signing transaction in job=%v: %w", txJob, err)
	}

	// build the outer fee-bump transaction
	feeBumpTx, err := txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      innerTx,
			FeeAccount: distributionAccount.Address,
			BaseFee:    int64(tw.engine.MaxBaseFee),
		},
	)
	if err != nil {
		return nil, false, fmt.Errorf("building fee-bump transaction for job %v: %w", txJob, err)
	}

	// Sign fee-bump tx for the distribution account:
	feeBumpTx, err = tw.engine.SignerRouter.SignFeeBumpStellarTransaction(ctx, feeBumpTx, distributionAccount)
	if err != nil {
		return nil, false, fmt.Errorf("signing fee-bump transaction for job %v: %w", txJob, err)
	}

	return feeBumpTx, false, nil
}

func (tw *TransactionWorker) submit(ctx context.Context, txJob *TxJob, feeBumpTx *txnbuild.FeeBumpTransaction) error {
	resp, err := tw.engine.HorizonClient.SubmitFeeBumpTransactionWithOptions(feeBumpTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		txErr := &utils.HorizonTransactionError{HorizonErrorWrapper: utils.NewHorizonErrorWrapper(err)}
		err = tw.handleFailedTransaction(ctx, txJob, resp, txErr)
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

func (tw *TransactionWorker) saveResponseXDRIfPresent(ctx context.Context, txJob *TxJob, resp horizon.Transaction) error {
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

// handlePreparationError processes errors during transaction preparation and determines if they need special handling.
// Returns (handled bool, error) where handled=true means the error was already processed by handleFailedTransaction.
func (tw *TransactionWorker) handlePreparationError(ctx context.Context, txJob *TxJob, err error) (bool, error) {
	// Check if it's a Horizon error
	var hErr *utils.HorizonErrorWrapper
	if errors.As(err, &hErr) && hErr.IsHorizonError() {
		txErr := &utils.HorizonTransactionError{HorizonErrorWrapper: hErr}
		handlerErr := tw.handleFailedTransaction(ctx, txJob, horizon.Transaction{}, txErr)
		if handlerErr != nil {
			return false, fmt.Errorf("handling horizon error: %w", handlerErr)
		}
		return true, hErr
	}

	// Check if it's an RPC error
	var rpcErr *utils.RPCErrorWrapper
	if errors.As(err, &rpcErr) && rpcErr.IsRPCError() {
		txErr := &utils.RPCTransactionError{RPCErrorWrapper: rpcErr}
		handlerErr := tw.handleFailedTransaction(ctx, txJob, horizon.Transaction{}, txErr)
		if handlerErr != nil {
			return false, fmt.Errorf("handling rpc error: %w", handlerErr)
		}
		return true, rpcErr
	}

	// For other errors, return as-is without special handling
	return false, err
}
