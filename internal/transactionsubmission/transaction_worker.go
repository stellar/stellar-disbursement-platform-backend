package transactionsubmission

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// Review these TODOs originally created by Stephen:
// TODO - memo/memoType not supported yet - [SDP-463]
// TODO - re-enable metrics/observer – [SDP-772]

const (
	paymentLoggerStr = "Payment event received %s: %s"
)

type TxJob store.ChannelTransactionBundle

func (job TxJob) String() string {
	return fmt.Sprintf("TxJob{ChannelAccount: %q, Transaction: %q, LockedUntilLedgerNumber: \"%d\"}", job.ChannelAccount.PublicKey, job.Transaction.ID, job.LockedUntilLedgerNumber)
}

type TransactionWorker struct {
	dbConnectionPool    db.DBConnectionPool
	txModel             store.TransactionStore
	chAccModel          store.ChannelAccountStore
	engine              *engine.SubmitterEngine
	sigService          engine.SignatureService
	maxBaseFee          int
	crashTrackerClient  crashtracker.CrashTrackerClient
	txProcessingLimiter *engine.TransactionProcessingLimiter
	monitorSvc          monitor.MonitorServiceInterface
	version             string
	gitCommitHash       string
}

type TxMetadata struct {
	srcChannelAcc    string
	paymentEventType string
	horizonErr       bool
	errStack         string
}

func NewTransactionWorker(
	dbConnectionPool db.DBConnectionPool,
	txModel *store.TransactionModel,
	chAccModel *store.ChannelAccountModel,
	engine *engine.SubmitterEngine,
	sigService engine.SignatureService,
	maxBaseFee int,
	crashTrackerClient crashtracker.CrashTrackerClient,
	txProcessingLimiter *engine.TransactionProcessingLimiter,
	monitorSvc monitor.MonitorServiceInterface,
	version string,
	gitCommitHash string,
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

	if tssUtils.IsEmpty(engine) {
		return TransactionWorker{}, fmt.Errorf("engine cannot be nil")
	}

	if tssUtils.IsEmpty(sigService) {
		return TransactionWorker{}, fmt.Errorf("sigService cannot be nil")
	}

	if maxBaseFee < txnbuild.MinBaseFee {
		return TransactionWorker{}, fmt.Errorf("maxBaseFee must be greater than or equal to %d", txnbuild.MinBaseFee)
	}

	if crashTrackerClient == nil {
		return TransactionWorker{}, fmt.Errorf("crashTrackerClient cannot be nil")
	}

	if txProcessingLimiter == nil {
		return TransactionWorker{}, fmt.Errorf("txProcessingLimiter cannot be nil")
	}

	if monitorSvc == nil {
		return TransactionWorker{}, fmt.Errorf("monitorSvc cannot be nil")
	}

	if version == "" {
		return TransactionWorker{}, fmt.Errorf("version cannot be empty string")
	}

	/*if gitCommitHash == "" {
		return TransactionWorker{}, fmt.Errorf("gitCommitHash cannot be empty string")
	}*/

	return TransactionWorker{
		dbConnectionPool:    dbConnectionPool,
		txModel:             txModel,
		chAccModel:          chAccModel,
		engine:              engine,
		sigService:          sigService,
		maxBaseFee:          maxBaseFee,
		crashTrackerClient:  crashTrackerClient,
		txProcessingLimiter: txProcessingLimiter,
		monitorSvc:          monitorSvc,
		version:             version,
		gitCommitHash:       gitCommitHash,
	}, nil
}

func (tw *TransactionWorker) Run(ctx context.Context, txJob *TxJob) {
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
// This method will only return an error if something goes wromg when handling the result and marking the transaction as ERROR.
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
		metricTag := monitor.PaymentErrorTag
		eventType := monitor.PaymentFailedLabel
		if !shouldMarkAsError {
			eventType = monitor.PaymentMarkedForReprocessingLabel
		}

		tw.monitorPayment(
			ctx,
			&txJob.Transaction,
			metricTag,
			&TxMetadata{
				srcChannelAcc:    txJob.ChannelAccount.PublicKey,
				horizonErr:       isHorizonErr,
				paymentEventType: eventType,
			},
		)
	}()

	if errors.As(hErr, &hErrWrapper) {
		tw.txProcessingLimiter.AdjustLimitIfNeeded(hErrWrapper)

		if hErrWrapper.ResultCodes != nil {
			isHorizonErr = true
			// TODO: move this logic inside the HorizonErrorWrapper
			// ref: https://developers.stellar.org/api/horizon/errors/result-codes/
			failedTxErrCodes := []string{
				"tx_bad_auth",
				"tx_bad_auth_extra",
				"tx_insufficient_balance",
			}
			if slices.Contains(failedTxErrCodes, hErrWrapper.ResultCodes.TransactionCode) || slices.Contains(failedTxErrCodes, hErrWrapper.ResultCodes.InnerTransactionCode) {
				shouldMarkAsError = true
			}

			// TODO: move this logic inside the HorizonErrorWrapper
			// ref: https://developers.stellar.org/api/horizon/errors/result-codes/
			failedOpCodes := []string{
				"op_bad_auth",
				"op_underfunded",
				"op_src_not_authorized",
				"op_no_destination",
				"op_no_trust",
				"op_line_full",
				"op_not_authorized",
				"op_no_issuer",
			}
			if !shouldMarkAsError {
				for _, opResult := range hErrWrapper.ResultCodes.OperationCodes {
					if slices.Contains(failedOpCodes, opResult) {
						shouldMarkAsError = true
						break
					}
				}
			}

			if shouldMarkAsError {
				var updatedTx *store.Transaction
				updatedTx, err = tw.txModel.UpdateStatusToError(ctx, txJob.Transaction, hErrWrapper.Error())
				if err != nil {
					return fmt.Errorf("updating transaction status to error: %w", err)
				}

				txJob.Transaction = *updatedTx
			}
		}
	}

	// TODO: call MonitorService if needed
	// TODO: call crashTrackerClient if needed
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

	_, err = tw.txModel.UpdateStatusToSuccess(ctx, txJob.Transaction)
	if err != nil {
		return utils.NewTransactionStatusUpdateError("SUCCESS", txJob.Transaction.ID, false, err)
	}

	err = tw.unlockJob(ctx, txJob)
	if err != nil {
		return fmt.Errorf("unlocking job: %w", err)
	}

	log.Ctx(ctx).Infof("🎉 Successfully processed transaction job %v", txJob)

	eventType := monitor.PaymentProcessingSuccessfulLabel
	if txJob.Transaction.AttemptsCount > 1 {
		eventType = monitor.PaymentReprocessingSuccessfulLabel
	}

	tw.monitorPayment(ctx, &txJob.Transaction, monitor.PaymentTransactionSuccessfulTag, &TxMetadata{
		srcChannelAcc:    txJob.ChannelAccount.PublicKey,
		horizonErr:       false,
		paymentEventType: eventType,
	})

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
			return fmt.Errorf("handling successful transaction: %w", err)
		}
		return nil
	} else if (err == nil && !txDetail.Successful) || hWrapperErr.IsNotFound() {
		// Unsuccesful hash: 98d3549076b119dbda42c17c2310d04666ef35524397ad3decb773ef1cebab1e
		// Nonexistent hash: 3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889
		log.Ctx(ctx).Warnf("Previous transaction didn't make through, marking %v for resubmission...", txJob)

		_, err = tw.txModel.PrepareTransactionForReprocessing(ctx, tw.dbConnectionPool, txJob.Transaction.ID)
		if err != nil {
			return fmt.Errorf("pushing back transaction to queue: %w", err)
		}

		err = tw.unlockJob(ctx, txJob)
		if err != nil {
			return fmt.Errorf("unlocking job: %w", err)
		}
	} else {
		// Invalid hash: 123
		log.Ctx(ctx).Warnf("received unexpected horizon error: %v", hWrapperErr)
		return fmt.Errorf("unexpected error: %w", hWrapperErr)
	}

	tw.monitorPayment(ctx, &txJob.Transaction, monitor.PaymentReconciliationSuccessfulTag, &TxMetadata{
		srcChannelAcc:    txJob.ChannelAccount.PublicKey,
		horizonErr:       false,
		paymentEventType: monitor.PaymentReconciliationSuccessfulLabel,
	})
	return nil
}

func (tw *TransactionWorker) processTransactionSubmission(ctx context.Context, txJob *TxJob) error {
	log.Ctx(ctx).Infof("🚧 Processing transaction submission for job %v...", txJob)

	tw.monitorPayment(ctx, &txJob.Transaction, monitor.PaymentProcessingStartedTag, &TxMetadata{
		srcChannelAcc:    txJob.ChannelAccount.PublicKey,
		horizonErr:       false,
		paymentEventType: monitor.PaymentProcessingStartedLabel,
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
	feeBumpTxHash, err := feeBumpTx.HashHex(tw.sigService.NetworkPassphrase())
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
					SourceAccount: tw.sigService.DistributionAccount(),
					Amount:        strconv.FormatFloat(txJob.Transaction.Amount, 'f', 6, 32), // TODO find a better way to do this
					Destination:   txJob.Transaction.Destination,
					Asset:         asset,
				},
			},
			BaseFee: int64(tw.maxBaseFee),
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

	paymentTx, err = tw.sigService.SignStellarTransaction(ctx, paymentTx, tw.sigService.DistributionAccount(), txJob.ChannelAccount.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("signing transaction: for job %v: %w", txJob, err)
	}

	// build the outer fee-bump transaction
	feeBumpTx, err = txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      paymentTx,
			FeeAccount: tw.sigService.DistributionAccount(),
			BaseFee:    int64(tw.maxBaseFee),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("building fee-bump transaction for job %v: %w", txJob, err)
	}

	// generate a random number to use as the fee-bump transaction's sequence number
	feeBumpTx, err = tw.sigService.SignFeeBumpStellarTransaction(ctx, feeBumpTx, tw.sigService.DistributionAccount())
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

// monitorPayment sends a metric about a payment tx to the observer, linking it to a entry in the logs that contains specific metadata about said tx.
func (tw *TransactionWorker) monitorPayment(ctx context.Context, tx *store.Transaction, metricTag monitor.MetricTag, txMetadata *TxMetadata) {
	eventID := uuid.New().String()
	labels := map[string]string{
		"event_id":        eventID,
		"event_type":      txMetadata.paymentEventType,
		"tx_id":           tx.ID,
		"event_time":      time.Now().String(),
		"app_version":     tw.version,
		"git_commit_hash": tw.gitCommitHash,
	}

	err := tw.monitorSvc.MonitorCounters(metricTag, labels)
	if err != nil {
		fmt.Println(err)
	}

	paymentLog := log.Ctx(ctx)
	for label_name, value := range labels {
		paymentLog.WithField(label_name, value)
	}

	paymentLog.WithFields(
		log.F{
			"event_id":            eventID,
			"tx_id":               tx.ID,
			"created_at":          tx.CreatedAt,
			"updated_at":          tx.UpdatedAt,
			"assset":              tx.AssetCode,
			"channel_account":     txMetadata.srcChannelAcc,
			"destination_account": tx.Destination,
		},
	)

	if metricTag == monitor.PaymentProcessingStartedTag {
		paymentLog.Debugf(paymentLoggerStr, eventID, metricTag)
		return
	}

	paymentLog.
		WithFields(
			log.F{
				"xdr_received": tx.XDRReceived,
				"xdr_sent":     tx.XDRSent,
			},
		)

	// successful transactions
	if metricTag == monitor.PaymentReconciliationSuccessfulTag || metricTag == monitor.PaymentTransactionSuccessfulTag {
		paymentLog.
			WithFields(
				log.F{
					"tx_hash":      tx.StellarTransactionHash,
					"completed_at": tx.CompletedAt,
				},
			).Infof(paymentLoggerStr, eventID, metricTag)

		return
	}

	// unsuccessful transactions
	if metricTag == monitor.PaymentErrorTag {
		if txMetadata.paymentEventType == monitor.PaymentFailedLabel {
			// payment could fail because of internal error too
			paymentLog.WithField("horizon_error?", txMetadata.horizonErr)
		}

		paymentLog.
			WithField("error", txMetadata.errStack).
			Errorf(paymentLoggerStr, eventID, metricTag)
	} else {
		log.Ctx(ctx).Errorf("Cannot recognize metric tag %s for event %s", metricTag, eventID)
	}
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
