package monitor

const (
	// Metric Tags
	HorizonErrorCounterTag                  MetricTag = "error_count"
	TransactionQueuedToCompletedLatencyTag  MetricTag = "queued_to_completed_latency_seconds"
	TransactionStartedToCompletedLatencyTag MetricTag = "started_to_completed_latency_seconds"
	TransactionRetryCountTag                MetricTag = "retry_count"
	TransactionProcessedCounterTag          MetricTag = "processed_count"

	// Metric Labels
	TransactionStatusSuccessLabel string = "success"
	TransactionStatusErrorLabel   string = "error"

	TransactionErrorBuildFeeBumpLabel string = "building_feebump_txn"
	TransactionErrorSignFeeBumpLebel  string = "sign_feebump_txn"
	TransactionErrorBuildPaymentLabel string = "building_payment_txn"
	TransactionErrorSignPaymentLebel  string = "sign_payment_txn"
	TransactionErrorSubmitLabel       string = "submitting_payment"
	TransactionErrorInvalidStateLabel string = "invalid_state"
	TransactionErrorHashingTxnLabel   string = "hashing_txn"
	TransactionErrorSavingHashLabel   string = "saving_hash"

	// Payment metric tags
	PaymentProcessingStartedTag        MetricTag = "payment_processing_started"
	PaymentTransactionSuccessfulTag    MetricTag = "payment_transaction_successful"
	PaymentReconciliationSuccessfulTag MetricTag = "payment_reconciliation_successful"
	PaymentReconciliationFailureTag    MetricTag = "payment_reconciliation_failure"
	PaymentErrorTag                    MetricTag = "payment_error"

	PaymentProcessingStartedLabel                   string = "payment_processing_started"
	PaymentProcessingSuccessfulLabel                string = "payment_processing_successful"
	PaymentReprocessingSuccessfulLabel              string = "payment_reprocessing_successful"
	PaymentReconciliationTransactionSuccessfulLabel string = "payment_reconciliation_transaction_successful"
	PaymentReconciliationMarkedForReprocessingLabel string = "payment_reconciliation_marked_for_reprocessing"
	PaymentReconciliationUnexpectedErrorLabel       string = "payment_reconciliation_unexpected_error"
	PaymentMarkedForReprocessingLabel               string = "payment_marked_for_reprocessing"
	PaymentFailedLabel                              string = "payment_failed"

	// Wallet creation metric tags
	WalletCreationProcessingStartedTag        MetricTag = "wallet_creation_processing_started"
	WalletCreationTransactionSuccessfulTag    MetricTag = "wallet_creation_transaction_successful"
	WalletCreationReconciliationSuccessfulTag MetricTag = "wallet_creation_reconciliation_successful"
	WalletCreationReconciliationFailureTag    MetricTag = "wallet_creation_reconciliation_failure"
	WalletCreationErrorTag                    MetricTag = "wallet_creation_error"

	WalletCreationProcessingStartedLabel                   string = "wallet_creation_processing_started"
	WalletCreationProcessingSuccessfulLabel                string = "wallet_creation_processing_successful"
	WalletCreationReprocessingSuccessfulLabel              string = "wallet_creation_reprocessing_successful"
	WalletCreationReconciliationTransactionSuccessfulLabel string = "wallet_creation_reconciliation_transaction_successful"
	WalletCreationReconciliationMarkedForReprocessingLabel string = "wallet_creation_reconciliation_marked_for_reprocessing"
	WalletCreationReconciliationUnexpectedErrorLabel       string = "wallet_creation_reconciliation_unexpected_error"
	WalletCreationMarkedForReprocessingLabel               string = "wallet_creation_marked_for_reprocessing"
	WalletCreationFailedLabel                              string = "wallet_creation_failed"
)

func (m MetricTag) ListAllTSSMetricTags() []MetricTag {
	return []MetricTag{
		HorizonErrorCounterTag,
		TransactionQueuedToCompletedLatencyTag,
		TransactionStartedToCompletedLatencyTag,
		TransactionRetryCountTag,
		TransactionProcessedCounterTag,

		PaymentProcessingStartedTag,
		PaymentTransactionSuccessfulTag,
		PaymentReconciliationSuccessfulTag,
		PaymentReconciliationFailureTag,
		PaymentErrorTag,

		WalletCreationProcessingStartedTag,
		WalletCreationTransactionSuccessfulTag,
		WalletCreationReconciliationSuccessfulTag,
		WalletCreationReconciliationFailureTag,
		WalletCreationErrorTag,
	}
}
