
package monitor

type MetricTag string

const (
	// payments
	PaymentProcessingStartedTag MetricTag = "tss_payment_processing_started"
	PaymentReprocessingSuccessfulTag MetricTag = "tss_payment_reprocessing_successful"
	PaymentReconciliationSuccessfulTag MetricTag = "tss_payment_reconciliation_successful"
	PaymentMarkedForReprocessing MetricTag = "tss_payment_marked_for_retry"
	PaymentFailedTag MetricTag = "tss_payment_failed"
)

func (m MetricTag) ListAll() []MetricTag {
	return []MetricTag{
		PaymentProcessingStartedTag,
		PaymentReprocessingSuccessfulTag,
		PaymentReconciliationSuccessfulTag,
		PaymentMarkedForReprocessing,
		PaymentFailedTag,
	}
}
