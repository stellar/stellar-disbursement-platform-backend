package transactionsubmission

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

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
)

// Review these TODOs originally created by Stephen:
// TODO - memo/memoType not supported yet - [SDP-463]

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
	txProcessingLimiter *engine.TransactionProcessingLimiter
	monitorSvc          tssMonitor.TSSMonitorService
	eventProducer       events.Producer
	jobUUID             string
}

func NewTransactionWorker(
	dbConnectionPool db.DBConnectionPool,
	txModel *store.TransactionModel,
	chAccModel *store.ChannelAccountModel,
	engine *engine.SubmitterEngine,
	crashTrackerClient crashtracker.CrashTrackerClient,
	txProcessingLimiter *engine.TransactionProcessingLimiter,
	monitorSvc tssMonitor.TSSMonitorService,
	eventProducer events.Producer,
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
	}, nil
}

// updateContextLogger will update the context logger with the transaction job details.
func (tw *TransactionWorker) updateContextLogger(ctx context.Context, job *TxJob) context.Context {
	tx := job.Transaction

	labels := map[string]interface{}{
		// Instance info
		"app_version":     tw.monitorSvc.Version,
		"git_commit_hash": tw.monitorSvc.GitCommitHash,
		// Job info
		"event_id": tw.jobUUID,
		// Transaction info
		"channel_account":     job.ChannelAccount.PublicKey,
		"tx_id":               tx.ID,
		"tenant_id":           tx.TenantID,
		"asset":               tx.AssetCode,
		"destination_account": tx.Destination,
		"created_at":          tx.CreatedAt.String(),
		"updated_at":          tx.UpdatedAt.String(),
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
		log.Ctx(ctx).Errorf("Handle unexpected error: %v", err)
	}
}

// TODO: add unit tests and godoc to this function
func (tw *TransactionWorker) runJob(ctx context.Context, txJob *TxJob) error {
	err := tw.validateJob(txJob)
	if err != nil {
		return fmt.Errorf("validating job: %w", err)
	}

	if txJob == nil {
		return fmt.Errorf("received nil transaction job")
	} else if txJob.Transaction.StellarTransactionHash.Valid {
		return tw.reconcileSubmittedTransaction(ctx, txJob)
	} else {
		return tw.processTransactionSubmission(ctx, txJob)
	}
}

// TODO: add tests
// handleFailedTransaction will wrap up the job when the transaction was submitted to the network but failed.
// This method will only return an error if something goes wrong when handling the result and marking the transaction as ERROR.
func (tw *TransactionWorker) handleFailedTransaction(ctx context.Context, txJob *TxJob, hTxResp horizon.Transaction, hErr error) error {
	log.Ctx(ctx).Errorf("🔴 Error processing job: %v", hErr)

	err := tw.saveResponseXDRIfPresent(ctx, txJob, hTxResp)
	if err != nil {
		return fmt.Errorf("saving response XDR: %w", err)
	}

	var shouldMarkAsError bool
	var isHorizonErr bool
	var hErrWrapper *utils.HorizonErrorWrapper
	defer func() {
		metricTag := sdpMonitor.PaymentErrorTag
		eventType := sdpMonitor.PaymentFailedLabel
		if !shouldMarkAsError {
			eventType = sdpMonitor.PaymentMarkedForReprocessingLabel
		}

		tw.monitorSvc.LogAndMonitorTransaction(
			ctx,
			txJob.Transaction,
			metricTag,
			tssMonitor.TxMetadata{
				EventID:          tw.jobUUID,
				SrcChannelAcc:    txJob.ChannelAccount.PublicKey,
				IsHorizonErr:     isHorizonErr,
				PaymentEventType: eventType,
			},
		)
	}()

	if errors.As(hErr, &hErrWrapper) {
		tw.txProcessingLimiter.AdjustLimitIfNeeded(hErrWrapper)

		if hErrWrapper.IsHorizonError() {
			isHorizonErr = true

			// Errors that are not marked as definitive errors:
			//   - 504: Timeout
			//   - 429: Too Many Requests
			//   - 400: with any of the codes: [tx_insufficient_fee, tx_too_late, tx_bad_seq]
			//   - 5xx
			//   - random network errors
			if hErrWrapper.ShouldMarkAsError() {
				var updatedTx *store.Transaction
				updatedTx, err = tw.txModel.UpdateStatusToError(ctx, txJob.Transaction, hErrWrapper.Error())
				if err != nil {
					return fmt.Errorf("updating transaction status to error: %w", err)
				}
				txJob.Transaction = *updatedTx

				// Publishing a new event on the event producer
				err = tw.producePaymentCompletedEvent(ctx, events.PaymentCompletedErrorType, &txJob.Transaction, data.FailedPaymentStatus)
				if err != nil {
					return fmt.Errorf("producing payment completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
				}

				// report any terminal errors, excluding those caused by the external account not being valid
				if !hErrWrapper.IsDestinationAccountNotReady() {
					tw.crashTrackerClient.LogAndReportErrors(ctx, hErrWrapper, "transaction error - cannot be retried")
				}
			}
		}
	}

	// TODO: op_bad_auth, tx_bad_auth, tx_bad_auth_extra are big problems that need to be reported accordingly
	// TODO: tx_bad_seq is a big problem that needs to be reported accordingly

	// {Old TSS approach} -> {new approach}:
	// - `504`: {retry in memory} -> {marked for retry} (pause/jitter could come later)
	// - `429`: {paused and marked for retry} -> {marked for retry} (pause/jitter could come later)
	// - `400 - tx_insufficient_fee` {marked for retry with exponential jitter until max_retry is reached} -> {marked for retry forever} (pause/jitter could come later)
	// - `400 - tx_bad_seq` {marked as failed} -> {marked for retry and reported to crash tracker and observer}
	// - `400 - tx_too_late` (bounds expired) {marked as failed} -> {marked for retry and reported to crash tracker and observer}
	// - `400 - ???`: {marked as failed} -> {marked for retry and reported to crash tracker and observer}
	// - unsupported error: {marked as failed} -> {marked for retry and reported to crash tracker and observer}

	// Some ideas for error handling (ref: https://developers.stellar.org/api/horizon/errors/result-codes/):
	// BadAuthentication():
	// op_bad_auth (in result_codes.operations)
	// tx_bad_auth (in result_codes.(inner_)transaction)
	// tx_bad_auth_extra (in result_codes.(inner_)transaction)
	//
	// NotEnoughLumens():
	// op_underfunded (in result_codes.operations)
	// tx_insufficient_balance  (in result_codes.(inner_)transaction)
	//
	// SendingAccountIsBlocked()
	//  op_src_not_authorized (in result_codes.operations)
	//
	// DestinationAccountNotFound():
	// op_no_destination (in result_codes.operations)
	//
	// DesinationIsMissingTrustlineOrLimit():
	// op_no_trust (in result_codes.operations)
	// op_line_full (in result_codes.operations)
	//
	// DestinationAccountIsBlocked():
	// op_not_authorized (in result_codes.operations)
	//
	// NonExistentAsset():
	// op_no_issuer (in result_codes.operations)

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

	updatedTx, err := tw.txModel.UpdateStatusToSuccess(ctx, txJob.Transaction)
	if err != nil {
		return utils.NewTransactionStatusUpdateError("SUCCESS", txJob.Transaction.ID, false, err)
	}
	txJob.Transaction = *updatedTx

	// Publishing a new event on the event producer
	err = tw.producePaymentCompletedEvent(ctx, events.PaymentCompletedSuccessType, &txJob.Transaction, data.SuccessPaymentStatus)
	if err != nil {
		return fmt.Errorf("producing payment completed event Status %s - Job %v: %w", txJob.Transaction.Status, txJob, err)
	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

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
			tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.PaymentReconciliationFailureTag, tssMonitor.TxMetadata{
				EventID:          tw.jobUUID,
				SrcChannelAcc:    txJob.ChannelAccount.PublicKey,
				IsHorizonErr:     false,
				ErrStack:         err.Error(),
				PaymentEventType: sdpMonitor.PaymentReconciliationUnexpectedErrorLabel,
			})
			return fmt.Errorf("handling successful transaction: %w", err)
		}

		tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.PaymentReconciliationSuccessfulTag, tssMonitor.TxMetadata{
			EventID:          tw.jobUUID,
			SrcChannelAcc:    txJob.ChannelAccount.PublicKey,
			PaymentEventType: sdpMonitor.PaymentReconciliationTransactionSuccessfulLabel,
		})
		return nil
	} else if (err != nil || txDetail.Successful) && !hWrapperErr.IsNotFound() {
		log.Ctx(ctx).Warnf("received unexpected horizon error: %v", hWrapperErr)

		tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.PaymentReconciliationFailureTag, tssMonitor.TxMetadata{
			EventID:          tw.jobUUID,
			SrcChannelAcc:    txJob.ChannelAccount.PublicKey,
			IsHorizonErr:     true,
			ErrStack:         hWrapperErr.Error(),
			PaymentEventType: sdpMonitor.PaymentReconciliationUnexpectedErrorLabel,
		})

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

	tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.PaymentReconciliationSuccessfulTag, tssMonitor.TxMetadata{
		EventID:          tw.jobUUID,
		SrcChannelAcc:    txJob.ChannelAccount.PublicKey,
		PaymentEventType: sdpMonitor.PaymentReconciliationMarkedForReprocessingLabel,
	})

	return nil
}

func (tw *TransactionWorker) producePaymentCompletedEvent(ctx context.Context, eventType string, tx *store.Transaction, paymentStatus data.PaymentStatus) error {
	if paymentStatus != data.SuccessPaymentStatus && paymentStatus != data.FailedPaymentStatus {
		return fmt.Errorf("invalid payment status to produce payment completed event")
	}

	msg := events.Message{
		Topic:    events.PaymentCompletedTopic,
		Key:      tx.ExternalID,
		TenantID: tx.TenantID,
		Type:     eventType,
		Data: schemas.EventPaymentCompletedData{
			TransactionID:        tx.ID,
			PaymentID:            tx.ExternalID,
			PaymentStatus:        string(paymentStatus),
			PaymentStatusMessage: tx.StatusMessage.String,
			PaymentCompletedAt:   time.Now(),
			StellarTransactionID: tx.StellarTransactionHash.String,
		},
	}

	if tw.eventProducer != nil {
		err := tw.eventProducer.WriteMessages(ctx, msg)
		if err != nil {
			return fmt.Errorf("writing message %s on event producer: %w", msg, err)
		}
	} else {
		log.Ctx(ctx).Errorf("event producer is nil, could not publish message %+v", msg)
	}

	return nil
}

func (tw *TransactionWorker) processTransactionSubmission(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🚧 Processing transaction submission for job %v...", txJob)

	tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.PaymentProcessingStartedTag, tssMonitor.TxMetadata{
		EventID:          tw.jobUUID,
		SrcChannelAcc:    txJob.ChannelAccount.PublicKey,
		PaymentEventType: sdpMonitor.PaymentProcessingStartedLabel,
	})

	// STEP 1: validate bundle
	err := tw.validateJob(txJob)
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
	allowedStatuses := []store.TransactionStatus{store.TransactionStatusPending, store.TransactionStatusProcessing}
	if !slices.Contains(allowedStatuses, txJob.Transaction.Status) {
		return fmt.Errorf("invalid transaction status: %v", txJob.Transaction.Status)
	}

	// TODO: make sure we're handling 429s upstream
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

// buildAndSignTransaction builds & signs a Stellar payment transaction that is wrapped in a feebump transaction.
func (tw *TransactionWorker) buildAndSignTransaction(ctx context.Context, txJob *TxJob) (feeBumpTx *txnbuild.FeeBumpTransaction, err error) {
	// validate the transaction asset
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

	var distributionAccountPubKey string
	var distributionAccount *schema.DistributionAccount
	if distributionAccount, err = tw.engine.DistributionAccountResolver.DistributionAccount(ctx, txJob.Transaction.TenantID); err != nil {
		return nil, fmt.Errorf("resolving distribution account for tenantID=%s: %w", txJob.Transaction.TenantID, err)
	} else if !distributionAccount.IsStellar() {
		return nil, fmt.Errorf("expected distribution account to be a STELLAR account but got %q", distributionAccount.Type)
	} else {
		distributionAccountPubKey = distributionAccount.ID
	}

	horizonAccount, err := tw.engine.HorizonClient.AccountDetail(horizonclient.AccountRequest{AccountID: txJob.ChannelAccount.PublicKey})
	if err != nil {
		return nil, utils.NewHorizonErrorWrapper(err)
	}

	// build the inner payment transaction
	paymentTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: txJob.ChannelAccount.PublicKey,
				Sequence:  horizonAccount.Sequence,
			},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					SourceAccount: distributionAccountPubKey,
					Amount:        strconv.FormatFloat(txJob.Transaction.Amount, 'f', 6, 32), // TODO find a better way to do this
					Destination:   txJob.Transaction.Destination,
					Asset:         asset,
				},
			},
			BaseFee: int64(tw.engine.MaxBaseFee),
			Preconditions: txnbuild.Preconditions{
				TimeBounds:   txnbuild.NewTimeout(300),                                                 // maximum 5 minutes
				LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)}, // currently, 8-10 ledgers in the future
			},
			IncrementSequenceNum: true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("building transaction for job %v: %w", txJob, err)
	}

	// Sign tx for the channel account:
	paymentTx, err = tw.engine.ChAccountSigner.SignStellarTransaction(ctx, paymentTx, txJob.ChannelAccount.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("signing transaction for channel account: for job %v: %w", txJob, err)
	}
	// Sign tx for the distribution account:
	paymentTx, err = tw.engine.DistAccountSigner.SignStellarTransaction(ctx, paymentTx, distributionAccountPubKey)
	if err != nil {
		return nil, fmt.Errorf("signing transaction for distribution account: for job %v: %w", txJob, err)
	}

	// build the outer fee-bump transaction
	feeBumpTx, err = txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      paymentTx,
			FeeAccount: distributionAccountPubKey,
			BaseFee:    int64(tw.engine.MaxBaseFee),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("building fee-bump transaction for job %v: %w", txJob, err)
	}

	// Sign fee-bump tx for the distribution account:
	feeBumpTx, err = tw.engine.DistAccountSigner.SignFeeBumpStellarTransaction(ctx, feeBumpTx, distributionAccountPubKey)
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

		eventType := sdpMonitor.PaymentProcessingSuccessfulLabel
		if txJob.Transaction.AttemptsCount > 1 {
			eventType = sdpMonitor.PaymentReprocessingSuccessfulLabel
		}

		tw.monitorSvc.LogAndMonitorTransaction(ctx, txJob.Transaction, sdpMonitor.PaymentTransactionSuccessfulTag, tssMonitor.TxMetadata{
			EventID:          tw.jobUUID,
			SrcChannelAcc:    txJob.ChannelAccount.PublicKey,
			IsHorizonErr:     false,
			PaymentEventType: eventType,
		})
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

// TODO: possibly use this code as a reference when addressing [SDP-772].
// updateTransactionsMetric calculates and observes metrics for a given Transaction
// func (s *Submitter) updateTransactionsMetric(ctx context.Context, result, error_type string, tx *store.Transaction) {
// 	retried := "false"
// 	if tx.RetryCount > 0 {
// 		retried = "true"
// 	}
// 	labels := map[string]string{
// 		"result":     result,
// 		"error_type": error_type,
// 		"retried":    retried,
// 	}
// 	// observe latency taken for transaction to complete
// 	err := s.MonitorService.MonitorHistogram(time.Since(*tx.CreatedAt).Seconds(), monitor.TransactionQueuedToCompletedLatencyTag, labels)
// 	if err != nil {
// 		log.Ctx(ctx).Errorf("error updating transaction metric counter: %s", err.Error())
// 	}

// 	err = s.MonitorService.MonitorHistogram(time.Since(*tx.StartedAt).Seconds(), monitor.TransactionStartedToCompletedLatencyTag, labels)
// 	if err != nil {
// 		log.Ctx(ctx).Errorf("error updating transaction metric counter: %s", err.Error())
// 	}

// 	err = s.MonitorService.MonitorHistogram(float64(tx.RetryCount), monitor.TransactionRetryCountTag, labels)
// 	if err != nil {
// 		log.Ctx(ctx).Errorf("error updating transaction metric counter: %s", err.Error())
// 	}

// 	err = s.MonitorService.MonitorCounters(monitor.TransactionProcessedCounterTag, labels)
// 	if err != nil {
// 		log.Ctx(ctx).Errorf("error updating transaction metric counter: %s", err.Error())
// 	}
// }

// // observeHorizonErrorMetric observes error metrics from horizon
// func (s *Submitter) observeHorizonErrorMetric(ctx context.Context, statusCode int, resultCode string) {
// 	labels := map[string]string{
// 		"status_code": strconv.Itoa(statusCode),
// 		"result_code": resultCode,
// 	}
// 	err := s.MonitorService.MonitorCounters(monitor.HorizonErrorCounterTag, labels)
// 	if err != nil {
// 		log.Ctx(ctx).Errorf("error updating horizon error counter metric: %s", err.Error())
// 	}
// }
