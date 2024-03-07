package monitor

import (
	"context"
	"fmt"
	"net/http"
	"slices"
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

var _ sdpMonitor.MonitorServiceInterface = &TSSMonitorService{}

type TxMetadata struct {
	SrcChannelAcc    string
	PaymentEventType string
	IsHorizonErr     bool
	ErrStack         string
}

func (ms *TSSMonitorService) Start(opts sdpMonitor.MetricOptions) error {
	if ms.Client != nil {
		return fmt.Errorf("service already initialized")
	}

	underlyingMonitorService := &sdpMonitor.MonitorService{}
	if err := underlyingMonitorService.Start(opts); err != nil {
		return fmt.Errorf("starting underlying monitor service: %w", err)
	}

	ms.Client = underlyingMonitorService.MonitorClient
	ms.MonitorServiceInterface = underlyingMonitorService

	return nil
}

// LogAndMonitorTransaction sends a metric about a payment tx to the observer, and logs the event and some additional data.
// The event and the log can be correlated through the event_id field.
func (ms *TSSMonitorService) LogAndMonitorTransaction(ctx context.Context, tx store.Transaction, metricTag sdpMonitor.MetricTag, txMetadata TxMetadata) {
	eventID := uuid.New().String()
	paymentLogMessage := paymentLogMessage(eventID, metricTag)

	labels := map[string]string{
		// Instance info
		"app_version":     ms.Version,
		"git_commit_hash": ms.GitCommitHash,
		// Event info
		"event_id":   eventID,
		"event_type": txMetadata.PaymentEventType,
		"event_time": time.Now().String(),
		// Transaction info
		"tx_id":           tx.ID,
		"tenant_id":       tx.TenantID,
		"channel_account": txMetadata.SrcChannelAcc,
	}
	paymentLog := log.Ctx(ctx).WithFields(log.F{
		"asset":               tx.AssetCode,
		"destination_account": tx.Destination,
		"created_at":          tx.CreatedAt.String(),
		"updated_at":          tx.UpdatedAt.String(),
	})
	for key, value := range labels {
		paymentLog = paymentLog.WithField(key, value)
	}

	err := ms.MonitorCounters(metricTag, labels)
	if err != nil {
		paymentLog.Errorf(
			"cannot send counters metric for event id with event type: %s, %s",
			eventID,
			txMetadata.PaymentEventType,
		)
	}

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
	if tx.StellarTransactionHash.Valid {
		paymentLog = paymentLog.WithField("tx_hash", tx.StellarTransactionHash.String)
	}

	isSuccessful := (metricTag == sdpMonitor.PaymentTransactionSuccessfulTag) ||
		(metricTag == sdpMonitor.PaymentReconciliationSuccessfulTag && txMetadata.PaymentEventType == sdpMonitor.PaymentReconciliationTransactionSuccessfulLabel)
	if isSuccessful {
		// successful transactions
		paymentLog.
			WithField("completed_at", tx.CompletedAt.String()).
			Info(paymentLogMessage)
	} else if slices.Contains([]sdpMonitor.MetricTag{sdpMonitor.PaymentErrorTag, sdpMonitor.PaymentReconciliationFailureTag}, metricTag) {
		// unsuccessful transactions
		paymentLog.
			WithFields(log.F{"horizon_error?": txMetadata.IsHorizonErr, "error": txMetadata.ErrStack}).
			Error(paymentLogMessage)
	} else {
		paymentLog.Errorf("Cannot recognize metricTag=%s for event=%s with PaymentEventType=%s", metricTag, eventID, txMetadata.PaymentEventType)
	}
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
