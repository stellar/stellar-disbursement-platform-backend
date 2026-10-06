package monitor

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

// Test_TSSMonitor_TransactionMetricLabels_MatchCounterCardinality is a regression test for a
// crash that took the TSS transaction submitter down on the first multi-wallet payment:
//
//	panic: inconsistent label cardinality: expected 8 label values but got 9 in
//	 prometheus.Labels{..., "wallet_id":"..."}
//
// The observability change added "wallet_id" to the label map emitted by
// buildMetricLabels/buildCommonFields, but the matching prometheus CounterVecs in
// internal/monitor (paymentLabelNames / walletCreationLabelNames / sponsoredTransactionLabelNames)
// were not updated in lock-step — so every real payment panicked the submitter, and no
// disbursement could ever pay out. The existing unit tests passed because they only asserted the
// label *map*; they never incremented a live counter with it.
//
// This test exercises the real emission -> increment path across both packages: it builds the
// exact label map production emits and increments each real CounterVec with it. prometheus'
// (*CounterVec).With panics if the label set drifts from the vec's declared labels (in either
// direction), so this fails loudly if the two are ever out of sync again.
func Test_TSSMonitor_TransactionMetricLabels_MatchCounterCardinality(t *testing.T) {
	ms := &TSSMonitorService{
		Version:       "test-version",
		GitCommitHash: "test-commit",
	}
	tx := store.Transaction{
		ID:       "tx-id",
		TenantID: "tenant-id",
		WalletID: sql.NullString{String: "44444444-4444-4444-4444-444444444444", Valid: true},
	}
	txMetadata := TxMetadata{
		EventID:              "event-id",
		SrcChannelAcc:        "GCHANNELACCOUNT",
		TransactionEventType: "payment",
	}

	// The exact label map production emits for a TSS transaction metric.
	labels := ms.buildMetricLabels(tx, txMetadata)
	require.Contains(t, labels, "wallet_id", "emitted TSS metric labels must carry wallet_id")
	require.Equal(t, "44444444-4444-4444-4444-444444444444", labels["wallet_id"])

	// Unbounded, per-transaction identifiers must NEVER be metric labels (they explode the TSDB);
	// they belong in logs. This locks in the cardinality fix.
	for _, forbidden := range []string{"event_id", "tx_id", "event_time"} {
		require.NotContains(t, labels, forbidden,
			"%q is high-cardinality and must not be a TSS metric label", forbidden)
	}

	// Every TSS transaction counter emitted with these labels. Incrementing the real CounterVec
	// with the real emitted labels panics on any label-cardinality / label-name drift.
	txCounterTags := []sdpMonitor.MetricTag{
		sdpMonitor.PaymentProcessingStartedTag,
		sdpMonitor.PaymentTransactionSuccessfulTag,
		sdpMonitor.PaymentReconciliationSuccessfulTag,
		sdpMonitor.PaymentReconciliationFailureTag,
		sdpMonitor.PaymentErrorTag,
		sdpMonitor.WalletCreationProcessingStartedTag,
		sdpMonitor.WalletCreationTransactionSuccessfulTag,
		sdpMonitor.WalletCreationReconciliationSuccessfulTag,
		sdpMonitor.WalletCreationReconciliationFailureTag,
		sdpMonitor.WalletCreationErrorTag,
		sdpMonitor.SponsoredTransactionProcessingStartedTag,
		sdpMonitor.SponsoredTransactionTransactionSuccessfulTag,
		sdpMonitor.SponsoredTransactionReconciliationSuccessfulTag,
		sdpMonitor.SponsoredTransactionReconciliationFailureTag,
		sdpMonitor.SponsoredTransactionErrorTag,
	}

	for _, tag := range txCounterTags {
		t.Run(string(tag), func(t *testing.T) {
			vec, ok := sdpMonitor.CounterTSSVecMetrics[tag]
			require.True(t, ok, "TSS counter %q must be registered", tag)
			assert.NotPanics(t, func() {
				vec.With(labels).Inc()
			}, "incrementing %q with the emitted labels must not panic (label cardinality drift)", tag)
		})
	}
}
