package monitor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/support/log"

	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

// TSSMonitorService wraps the generic monitoring service from the SDP and provides additional monitoring capability for
// tracking payments that are processed by the TSS.
type TSSMonitorService struct {
	Client sdpMonitor.MonitorClient
	sdpMonitor.MonitorServiceInterface
	GitCommitHash string
	Version       string
}

type TxMetadata struct {
	SrcChannelAcc    string
	PaymentEventType string
	IsHorizonErr     bool
	ErrStack         string
}

// MonitorPayment sends a metric about a payment tx to the observer, linking it to a entry in the logs that contains specific metadata about said tx.
func (ms *TSSMonitorService) MonitorPayment(ctx context.Context, tx store.Transaction, metricTag sdpMonitor.MetricTag, txMetadata TxMetadata) {
	eventID := uuid.New().String()
	paymentLogMessage := paymentLogMessage(eventID, metricTag)

	labels := map[string]string{
		"event_id":        eventID,
		"event_type":      txMetadata.PaymentEventType,
		"tx_id":           tx.ID,
		"event_time":      time.Now().String(),
		"app_version":     ms.Version,
		"git_commit_hash": ms.GitCommitHash,
	}

	err := ms.MonitorCounters(metricTag, labels)
	if err != nil {
		log.Ctx(ctx).Errorf(
			"cannot send counters metric for event id with event type: %s, %s",
			eventID,
			txMetadata.PaymentEventType,
		)
	}

	paymentLog := log.Ctx(ctx)
	for label_name, value := range labels {
		paymentLog = paymentLog.WithField(label_name, value)
	}

	paymentLog = paymentLog.WithFields(
		log.F{
			"created_at":          tx.CreatedAt.String(),
			"updated_at":          tx.UpdatedAt.String(),
			"asset":               tx.AssetCode,
			"channel_account":     txMetadata.SrcChannelAcc,
			"destination_account": tx.Destination,
		},
	)

	if metricTag == sdpMonitor.PaymentProcessingStartedTag {
		paymentLog.Debug(paymentLogMessage)
		return
	}

	// reconciliation - marked for reprocessing
	if metricTag == sdpMonitor.PaymentReconciliationSuccessfulTag && txMetadata.PaymentEventType == sdpMonitor.PaymentReconciliationMarkedForReprocessingLabel {
		paymentLog.Info(paymentLogMessage)
		return
	}

	if tx.XDRSent.Valid {
		paymentLog = paymentLog.WithField("xdr_sent", tx.XDRSent.String)
	}
	if tx.XDRReceived.Valid {
		paymentLog = paymentLog.WithField("xdr_received", tx.XDRReceived.String)
	}

	// successful transactions
	if (metricTag == sdpMonitor.PaymentReconciliationSuccessfulTag && txMetadata.PaymentEventType == sdpMonitor.PaymentReconciliationTransactionSuccessfulLabel) ||
		metricTag == sdpMonitor.PaymentTransactionSuccessfulTag {
		paymentLog.
			WithFields(
				log.F{
					"tx_hash":      tx.StellarTransactionHash.String,
					"completed_at": tx.CompletedAt.String(),
				},
			).Info(paymentLogMessage)

		return
	}

	// unsuccessful transactions
	if metricTag == sdpMonitor.PaymentErrorTag || metricTag == sdpMonitor.PaymentReconciliationFailureTag {
		paymentLog.
			WithFields(
				log.F{
					"horizon_error?": txMetadata.IsHorizonErr,
					"error":          txMetadata.ErrStack,
				},
			).Error(paymentLogMessage)
		return
	}

	log.Ctx(ctx).Errorf("Cannot recognize metric tag %s for event %s", metricTag, eventID)
}

func (ms *TSSMonitorService) GetMetricHttpHandler() (http.Handler, error) {
	if ms.Client == nil {
		return nil, fmt.Errorf("client was not initialized")
	}

	return ms.Client.GetMetricHttpHandler(), nil
}

func (ms *TSSMonitorService) MonitorCounters(metricTag sdpMonitor.MetricTag, labels map[string]string) error {
	if ms.Client == nil {
		return fmt.Errorf("client was not initialized")
	}

	ms.Client.MonitorCounters(metricTag, labels)

	return nil
}

func paymentLogMessage(eventID string, metricTag sdpMonitor.MetricTag) string {
	return fmt.Sprintf("Payment event received %s: %s", eventID, metricTag)
}
