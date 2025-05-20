package transactionsubmission

import (
	"context"
	"errors"
	"fmt"
	"slices"

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
	handlerFactory      TransactionHandlerFactoryInterface
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
	handlerFactory TransactionHandlerFactoryInterface,
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

	if handlerFactory == nil {
		return TransactionWorker{}, fmt.Errorf("handlerFactory cannot be nil")
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
		handlerFactory:      handlerFactory,
	}, nil
}

// updateContextLogger will update the context logger with the transaction job details.
func (tw *TransactionWorker) updateContextLogger(ctx context.Context, job *TxJob) context.Context {
	tx := job.Transaction

	handler, err := tw.handlerFactory.GetHandler(&tx)
	if err != nil {
		log.Ctx(ctx).Errorf("Error getting transaction handler: %v", err)
		return ctx
	}

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
	if handler != nil {
		handlerFields := handler.AddContextLoggerFields(&tx)
		for k, v := range handlerFields {
			labels[k] = v
		}
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

// handleFailedTransaction will wrap up the job when the transaction was submitted to the network but failed.
// This method will only return an error if something goes wrong when handling the result and marking the transaction as
// ERROR.
//
// Errors that triger the pause/jitter mechanism at TransactionProcessingLimiter:
//   - 504: Timeout
//   - 429: Too Many Requests
//   - 400 - tx_insufficient_fee: Bad Request
//
// Errors marked as definitive error, that won't be resolved with retries:
//   - 400: with any of the transaction error codes [tx_bad_auth, tx_bad_auth_extra, tx_insufficient_balance]
//   - 400: with any of the operation error codes [op_bad_auth, op_underfunded, op_src_not_authorized, op_no_destination, op_no_trust, op_line_full, op_not_authorized, op_no_issuer]
//
// Errors that are marked for retry without pause/jitter but are reported to CrashTracker:
//   - 400 - tx_bad_seq: Bad Request
//
// Errors that are marked for retry without pause/jitter and are not reported to CrashTracker:
//   - 400 - tx_too_late: Bad Request
//   - xxx - Any unexpected error.
func (tw *TransactionWorker) handleFailedTransaction(ctx context.Context, txJob *TxJob, hTxResp horizon.Transaction, hErr *utils.HorizonErrorWrapper) error {
	log.Ctx(ctx).Errorf("🔴 Error processing job: %v", hErr)

	handler, err := tw.handlerFactory.GetHandler(&txJob.Transaction)
	if err != nil {
		return fmt.Errorf("getting transaction handler: %w", err)
	}

	isRetryable := !(hErr.IsHorizonError() && hErr.ShouldMarkAsError())
	defer handler.MonitorTransactionProcessingFailed(ctx, txJob, tw.jobUUID, isRetryable, hErr.Error())

	err = tw.saveResponseXDRIfPresent(ctx, txJob, hTxResp)
	if err != nil {
		return fmt.Errorf("saving response XDR: %w", err)
	}

	var hErrWrapper *utils.HorizonErrorWrapper
	if errors.As(hErr, &hErrWrapper) {
		if hErrWrapper.IsHorizonError() {
			tw.txProcessingLimiter.AdjustLimitIfNeeded(hErrWrapper)

			if hErr.IsHorizonError() && hErr.ShouldMarkAsError() {
				// Building the completed event before updating the transaction status. This way, if the message
				// fails to be built, the transaction will be marked for reprocessing -> reconciliation and the event
				// will be re-tried.
				eventMsg, eventErr := handler.BuildFailureEvent(ctx, txJob, hErrWrapper)
				if eventErr != nil {
					return fmt.Errorf("producing completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, eventErr)
				}

				updatedTx, txErr := tw.txModel.UpdateStatusToError(ctx, txJob.Transaction, hErrWrapper.Error())
				if txErr != nil {
					return fmt.Errorf("updating transaction status to error: %w", err)
				}
				txJob.Transaction = *updatedTx

				// Publishing a new event on the event producer
				err = events.ProduceEvents(ctx, tw.eventProducer, eventMsg)
				if err != nil {
					return fmt.Errorf("producing completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
				}

				// report any terminal errors, excluding those caused by the external account not being valid
				if !hErrWrapper.IsDestinationAccountNotReady() {
					tw.crashTrackerClient.LogAndReportErrors(ctx, hErrWrapper, "transaction error - cannot be retried")
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

	handler, err := tw.handlerFactory.GetHandler(&txJob.Transaction)
	if err != nil {
		return fmt.Errorf("getting transaction handler: %w", err)
	}

	// Building the completed event before updating the transaction status. This way, if the message fails to be
	// built, the transaction will be marked for reprocessing -> reconciliation and the event will be re-tried.
	eventMsg, err := handler.BuildSuccessEvent(ctx, txJob)
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

	handler.MonitorTransactionProcessingSuccess(ctx, txJob, tw.jobUUID)

	log.Ctx(ctx).Infof("🎉 Successfully processed transaction job %v", txJob)

	return nil
}

// reconcileSubmittedTransaction will check the status of a previously submitted transaction and handle it accordingly.
// If the transaction was successful, it will be marked as such and the job will be unlocked.
// If the transaction failed, it will be marked for resubmission.
func (tw *TransactionWorker) reconcileSubmittedTransaction(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🔍 Reconciling previously submitted transaction %v...", txJob)

	handler, err := tw.handlerFactory.GetHandler(&txJob.Transaction)
	if err != nil {
		return fmt.Errorf("getting transaction handler: %w", err)
	}

	err = tw.validateJob(txJob)
	if err != nil {
		return fmt.Errorf("validating bundle: %w", err)
	}

	txHash := txJob.Transaction.StellarTransactionHash.String
	txDetail, err := tw.engine.HorizonClient.TransactionDetail(txHash)
	hWrapperErr := utils.NewHorizonErrorWrapper(err)

	if err == nil && txDetail.Successful {
		err = tw.handleSuccessfulTransaction(ctx, txJob, txDetail)
		if err != nil {
			handler.MonitorTransactionReconciliationFailure(ctx, txJob, tw.jobUUID, false, err.Error())
			return fmt.Errorf("handling successful transaction: %w", err)
		}

		handler.MonitorTransactionReconciliationSuccess(ctx, txJob, tw.jobUUID, ReconcileSuccess)
		return nil
	} else if (err != nil || txDetail.Successful) && !hWrapperErr.IsNotFound() {
		log.Ctx(ctx).Warnf("received unexpected horizon error: %v", hWrapperErr)

		handler.MonitorTransactionReconciliationFailure(ctx, txJob, tw.jobUUID, true, hWrapperErr.Error())
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

	handler.MonitorTransactionReconciliationSuccess(ctx, txJob, tw.jobUUID, ReconcileReprocessing)

	return nil
}

func (tw *TransactionWorker) processTransactionSubmission(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🚧 Processing transaction submission for job %v...", txJob)

	handler, err := tw.handlerFactory.GetHandler(&txJob.Transaction)
	if err != nil {
		return fmt.Errorf("getting transaction handler: %w", err)
	}

	handler.MonitorTransactionProcessingStarted(ctx, txJob, tw.jobUUID)

	// STEP 1: validate bundle
	err = tw.validateJob(txJob)
	if err != nil {
		return fmt.Errorf("validating bundle: %w", err)
	}

	// STEP 2: prepare transaction for processing
	feeBumpTx, err := tw.prepareForSubmission(ctx, txJob)
	if err != nil {
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

func (tw *TransactionWorker) prepareForSubmission(ctx context.Context, txJob *TxJob) (*txnbuild.FeeBumpTransaction, error) {
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

func (tw *TransactionWorker) buildAndSignTransaction(ctx context.Context, txJob *TxJob) (*txnbuild.FeeBumpTransaction, error) {
	handler, err := tw.handlerFactory.GetHandler(&txJob.Transaction)
	if err != nil {
		return nil, fmt.Errorf("getting transaction handler: %w", err)
	}

	distributionAccount, err := tw.engine.DistributionAccountResolver.DistributionAccount(ctx, txJob.Transaction.TenantID)
	if err != nil {
		return nil, fmt.Errorf("resolving distribution account for tenantID=%s: %w", txJob.Transaction.TenantID, err)
	} else if !distributionAccount.IsStellar() {
		return nil, fmt.Errorf("expected distribution account to be a STELLAR account but got %q", distributionAccount.Type)
	}

	horizonAccount, err := tw.engine.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: txJob.ChannelAccount.PublicKey})
	if err != nil {
		err = fmt.Errorf("getting account detail: %w", err)
		return nil, utils.NewHorizonErrorWrapper(err)
	}

	innerTx, err := handler.BuildInnerTransaction(ctx, txJob, horizonAccount.Sequence, distributionAccount.Address)
	if err != nil {
		return nil, fmt.Errorf("building transaction for job %v: %w", txJob, err)
	}

	// Sign tx for the channel account:
	chAccount := schema.TransactionAccount{
		Address: txJob.ChannelAccount.PublicKey,
		Type:    schema.ChannelAccountStellarDB,
	}
	innerTx, err = tw.engine.SignerRouter.SignStellarTransaction(ctx, innerTx, chAccount, distributionAccount)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in job=%v: %w", txJob, err)
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
		return nil, fmt.Errorf("building fee-bump transaction for job %v: %w", txJob, err)
	}

	// Sign fee-bump tx for the distribution account:
	feeBumpTx, err = tw.engine.SignerRouter.SignFeeBumpStellarTransaction(ctx, feeBumpTx, distributionAccount)
	if err != nil {
		return nil, fmt.Errorf("signing fee-bump transaction for job %v: %w", txJob, err)
	}

	return feeBumpTx, nil
}

func (tw *TransactionWorker) submit(ctx context.Context, txJob *TxJob, feeBumpTx *txnbuild.FeeBumpTransaction) error {
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
