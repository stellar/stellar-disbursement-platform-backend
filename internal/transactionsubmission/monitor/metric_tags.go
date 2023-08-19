package monitor

// TODO: Right now, we are still leveraging SDP's monitoring client but once TSS becomes its
// own entity, we will need to export relevant code into this package.
import "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"

const (
	// payments
	PaymentProcessingStartedTag        monitor.MetricTag = "tss_payment_processing_started"
	PaymentReprocessingSuccessfulTag   monitor.MetricTag = "tss_payment_reprocessing_successful"
	PaymentReconciliationSuccessfulTag monitor.MetricTag = "tss_payment_reconciliation_successful"
	PaymentMarkedForReprocessing       monitor.MetricTag = "tss_payment_marked_for_retry"
	PaymentFailedTag                   monitor.MetricTag = "tss_payment_failed"
)

func ListAllPaymentMetricTags() []monitor.MetricTag {
	return []monitor.MetricTag{
		PaymentProcessingStartedTag,
		PaymentReprocessingSuccessfulTag,
		PaymentReconciliationSuccessfulTag,
		PaymentMarkedForReprocessing,
		PaymentFailedTag,
	}
}
