package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type MonitorService struct {
	GitCommitHash string
	Version       string
	MonitorClient monitor.MonitorServiceInterface
}

type TxMetadata struct {
	SrcChannelAcc    string
	PaymentEventType string
	IsHorizonErr     bool
	ErrStack         string
}

func NewMonitorService(
	tx context.Context,
	monitorService monitor.MonitorServiceInterface,
	metricOptions monitor.MetricOptions,
	version, gitCommitHash string,
) (MonitorService, error) {
	err := monitorService.Start(metricOptions)
	if err != nil {
		return MonitorService{}, fmt.Errorf("cannot start monitor service: %w", err)
	}

	return MonitorService{
		MonitorClient: monitorService,
		GitCommitHash: gitCommitHash,
		Version:       version,
	}, nil
}

// monitorPayment sends a metric about a payment tx to the observer, linking it to a entry in the logs that contains specific metadata about said tx.
func (ms *MonitorService) MonitorPayment(ctx context.Context, tx store.Transaction, metricTag monitor.MetricTag, txMetadata TxMetadata) {
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

	err := ms.MonitorClient.MonitorCounters(metricTag, labels)
	if err != nil {
		log.Ctx(ctx).Errorf(
			"cannot send counters metric for event id with event type: %s, %s",
			eventID,
			txMetadata.PaymentEventType,
		)
	}

	paymentLog := log.Ctx(ctx)
	for label_name, value := range labels {
		paymentLog.WithField(label_name, value)
	}

	paymentLog.WithFields(
		log.F{
			"created_at":          tx.CreatedAt,
			"updated_at":          tx.UpdatedAt,
			"asset":               tx.AssetCode,
			"channel_account":     txMetadata.SrcChannelAcc,
			"destination_account": tx.Destination,
		},
	)

	if metricTag == monitor.PaymentProcessingStartedTag {
		paymentLog.Debugf(paymentLogMessage)
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
			).Infof(paymentLogMessage)

		return
	}

	// unsuccessful transactions
	if metricTag == monitor.PaymentErrorTag || metricTag == monitor.PaymentReconciliationFailureTag {
		paymentLog.
			WithField("horizon_error?", txMetadata.IsHorizonErr).
			WithField("error", txMetadata.ErrStack).
			Errorf(paymentLogMessage)
	} else {
		log.Ctx(ctx).Errorf("Cannot recognize metric tag %s for event %s", metricTag, eventID)
	}
}

func paymentLogMessage(eventID string, metricTag monitor.MetricTag) string {
	return fmt.Sprintf("Payment event received %s: %s", eventID, metricTag)
}
