package monitor

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

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
	EventID              string
	SrcChannelAcc        string
	TransactionEventType string
	IsHorizonErr         bool   // TODO: remove
	ErrStack             string // TODO: remove
	// Error            string 	//TODO: add
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

// LogAndMonitorTransaction sends a metric about a transaction (payment or wallet creation) to the observer, and logs the event and some additional data.
// The event and the log can be correlated through the event_id field.
func (ms *TSSMonitorService) LogAndMonitorTransaction(ctx context.Context, tx store.Transaction, metricTag sdpMonitor.MetricTag, txMetadata TxMetadata) {
	eventID := txMetadata.EventID
	logMessage := transactionLogMessage(eventID, metricTag)

	// Build base log entry with standard fields
	logEntry := ms.buildBaseLogEntry(ctx, tx, txMetadata)

	// Send metrics
	labels := ms.buildMetricLabels(tx, txMetadata)
	if err := ms.MonitorCounters(metricTag, labels); err != nil {
		logEntry.Errorf("cannot send counters metric for event id with event type: %s, %s", eventID, txMetadata.TransactionEventType)
	}

	// Handle different event types
	ms.logTransactionEvent(logEntry, tx, metricTag, txMetadata, logMessage)
}

// buildMetricLabels creates the label map used for the TSS transaction CounterVecs. Only
// LOW-cardinality labels belong here: event_id, tx_id and event_time are UNBOUNDED (unique per
// transaction) and would explode the Prometheus TSDB, so they live in the logs (buildCommonFields)
// instead — where an operator can still correlate them via the shared event_id. The keys here MUST
// stay in lock-step with the CounterVec label sets declared in internal/monitor (paymentLabelNames /
// walletCreationLabelNames / sponsoredTransactionLabelNames); prometheus panics on any drift.
func (ms *TSSMonitorService) buildMetricLabels(tx store.Transaction, txMetadata TxMetadata) map[string]string {
	return map[string]string{
		// Instance info
		"app_version":     ms.Version,
		"git_commit_hash": ms.GitCommitHash,
		// Event info
		"event_type": txMetadata.TransactionEventType,
		// Transaction info
		"tenant_id":       tx.TenantID,
		"wallet_id":       tx.WalletID.String,
		"channel_account": txMetadata.SrcChannelAcc,
	}
}

// buildCommonFields creates the fields attached to the transaction log entry. Logs keep the full
// per-event detail — including the high-cardinality identifiers (event_id, tx_id, event_time) that
// are deliberately kept OUT of the metric labels — plus payment_id (the SDP Transaction.ExternalID),
// the field an operator uses to correlate a TSS log line back to the originating SDP payment.
func (ms *TSSMonitorService) buildCommonFields(tx store.Transaction, txMetadata TxMetadata) map[string]string {
	fields := map[string]string{
		// Instance info
		"app_version":     ms.Version,
		"git_commit_hash": ms.GitCommitHash,
		// Event info
		"event_id":   txMetadata.EventID,
		"event_type": txMetadata.TransactionEventType,
		"event_time": time.Now().UTC().Format(time.RFC3339),
		// Transaction info
		"tx_id":           tx.ID,
		"tenant_id":       tx.TenantID,
		"wallet_id":       tx.WalletID.String,
		"channel_account": txMetadata.SrcChannelAcc,
	}
	if tx.ExternalID != "" {
		fields["payment_id"] = tx.ExternalID
	}
	return fields
}

// buildBaseLogEntry creates a log entry with common fields for all transaction events
func (ms *TSSMonitorService) buildBaseLogEntry(ctx context.Context, tx store.Transaction, txMetadata TxMetadata) *log.Entry {
	commonFields := ms.buildCommonFields(tx, txMetadata)
	logFields := make(log.F)
	for k, v := range commonFields {
		logFields[k] = v
	}
	return log.Ctx(ctx).WithFields(logFields)
}

// logTransactionEvent handles the actual logging based on the event type
func (ms *TSSMonitorService) logTransactionEvent(logEntry *log.Entry, tx store.Transaction, metricTag sdpMonitor.MetricTag, txMetadata TxMetadata, logMessage string) {
	// Handle processing started events
	if ms.isProcessingStartedEvent(metricTag) {
		logEntry.Debug(logMessage)
		return
	}

	// Handle reconciliation reprocessing events
	if ms.isReconciliationReprocessingEvent(metricTag, txMetadata) {
		logEntry.Info(logMessage)
		return
	}

	// Add transaction details for completed events
	logEntry = ms.addTransactionDetails(logEntry, tx)

	// Handle success/failure events
	if ms.isSuccessfulEvent(metricTag, txMetadata) {
		logEntry.Info(logMessage)
	} else if ms.isErrorEvent(metricTag) {
		logEntry.WithFields(log.F{
			"is_horizon_error": txMetadata.IsHorizonErr,
			"error":            txMetadata.ErrStack,
		}).Error(logMessage)
	} else {
		logEntry.Errorf("Cannot recognize metricTag=%s for event=%s with TransactionEventType=%s", metricTag, txMetadata.EventID, txMetadata.TransactionEventType)
	}
}

// isProcessingStartedEvent checks if this is a processing started event
func (ms *TSSMonitorService) isProcessingStartedEvent(metricTag sdpMonitor.MetricTag) bool {
	processingStartedTags := []sdpMonitor.MetricTag{
		sdpMonitor.PaymentProcessingStartedTag,
		sdpMonitor.WalletCreationProcessingStartedTag,
		sdpMonitor.SponsoredTransactionProcessingStartedTag,
	}
	return slices.Contains(processingStartedTags, metricTag)
}

// isReconciliationReprocessingEvent checks if this is a reconciliation reprocessing event
func (ms *TSSMonitorService) isReconciliationReprocessingEvent(metricTag sdpMonitor.MetricTag, txMetadata TxMetadata) bool {
	reprocessingCombinations := map[sdpMonitor.MetricTag]string{
		sdpMonitor.PaymentReconciliationSuccessfulTag:              sdpMonitor.PaymentReconciliationMarkedForReprocessingLabel,
		sdpMonitor.WalletCreationReconciliationSuccessfulTag:       sdpMonitor.WalletCreationReconciliationMarkedForReprocessingLabel,
		sdpMonitor.SponsoredTransactionReconciliationSuccessfulTag: sdpMonitor.SponsoredTransactionReconciliationMarkedForReprocessingLabel,
	}

	expectedEventType, exists := reprocessingCombinations[metricTag]
	return exists && txMetadata.TransactionEventType == expectedEventType
}

// isSuccessfulEvent checks if this is a successful transaction event
func (ms *TSSMonitorService) isSuccessfulEvent(metricTag sdpMonitor.MetricTag, txMetadata TxMetadata) bool {
	simpleSuccessTags := []sdpMonitor.MetricTag{
		sdpMonitor.PaymentTransactionSuccessfulTag,
		sdpMonitor.WalletCreationTransactionSuccessfulTag,
		sdpMonitor.SponsoredTransactionTransactionSuccessfulTag,
	}
	if slices.Contains(simpleSuccessTags, metricTag) {
		return true
	}

	// Reconciliation success tags that need specific event type checks
	reconciliationSuccessCombinations := map[sdpMonitor.MetricTag]string{
		sdpMonitor.PaymentReconciliationSuccessfulTag:              sdpMonitor.PaymentReconciliationTransactionSuccessfulLabel,
		sdpMonitor.WalletCreationReconciliationSuccessfulTag:       sdpMonitor.WalletCreationReconciliationTransactionSuccessfulLabel,
		sdpMonitor.SponsoredTransactionReconciliationSuccessfulTag: sdpMonitor.SponsoredTransactionReconciliationTransactionSuccessfulLabel,
	}

	expectedEventType, exists := reconciliationSuccessCombinations[metricTag]
	return exists && txMetadata.TransactionEventType == expectedEventType
}

// isErrorEvent checks if this is an error event
func (ms *TSSMonitorService) isErrorEvent(metricTag sdpMonitor.MetricTag) bool {
	errorTags := []sdpMonitor.MetricTag{
		sdpMonitor.PaymentErrorTag,
		sdpMonitor.PaymentReconciliationFailureTag,
		sdpMonitor.WalletCreationErrorTag,
		sdpMonitor.WalletCreationReconciliationFailureTag,
		sdpMonitor.SponsoredTransactionErrorTag,
		sdpMonitor.SponsoredTransactionReconciliationFailureTag,
	}
	return slices.Contains(errorTags, metricTag)
}

// addTransactionDetails adds transaction-specific fields to the log entry
func (ms *TSSMonitorService) addTransactionDetails(logEntry *log.Entry, tx store.Transaction) *log.Entry {
	fields := make(log.F)

	if tx.XDRSent.Valid {
		fields["xdr_sent"] = tx.XDRSent.String
	}
	if tx.XDRReceived.Valid {
		fields["xdr_received"] = tx.XDRReceived.String
	}
	if tx.StellarTransactionHash.Valid {
		fields["tx_hash"] = tx.StellarTransactionHash.String
	}
	if tx.CompletedAt != nil {
		fields["completed_at"] = tx.CompletedAt.String()
	}

	if len(fields) > 0 {
		logEntry = logEntry.WithFields(fields)
	}

	return logEntry
}

func (ms *TSSMonitorService) GetMetricHTTPHandler() (http.Handler, error) {
	if ms.Client == nil {
		return nil, fmt.Errorf("client was not initialized")
	}

	return ms.Client.GetMetricHTTPHandler(), nil
}

func (ms *TSSMonitorService) MonitorCounters(metricTag sdpMonitor.MetricTag, labels map[string]string) error {
	if ms.Client == nil {
		return fmt.Errorf("client was not initialized")
	}

	ms.Client.MonitorCounters(metricTag, labels)

	return nil
}

func transactionLogMessage(eventID string, metricTag sdpMonitor.MetricTag) string {
	return fmt.Sprintf("Transaction event received %s: %s", eventID, metricTag)
}
