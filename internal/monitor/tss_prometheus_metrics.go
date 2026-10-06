package monitor

import "github.com/prometheus/client_golang/prometheus"

// txLatencyBuckets covers 5s–2min at 5s resolution, then a coarser slow tail out to 30min so that
// transactions slower than 2 minutes are no longer all collapsed into the +Inf bucket — exactly the
// range where SLO breaches happen and tail visibility matters most.
var txLatencyBuckets = append(
	prometheus.LinearBuckets(5, 5, 24),            // 5s, 10s, ... 120s
	150, 180, 240, 300, 420, 600, 900, 1200, 1800, // 2.5min ... 30min slow tail
)

// dbQueryObjectives are the quantile targets for DB-query duration summaries, matching the HTTP
// request-duration summary so DB p50/p90/p95/p99 latency can be charted in Grafana.
var dbQueryObjectives = map[float64]float64{
	0.5:  0.05,  // 50th percentile with 5% error
	0.9:  0.01,  // 90th percentile with 1% error
	0.95: 0.01,  // 95th percentile with 1% error
	0.99: 0.001, // 99th percentile with 0.1% error
}

var HistogramTSSVecMetrics = map[MetricTag]*prometheus.HistogramVec{
	TransactionQueuedToCompletedLatencyTag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tss",
		Subsystem: "tx_processing",
		Name:      string(TransactionQueuedToCompletedLatencyTag),
		Help:      "Latency (seconds) taken from when a Transaction was created to when it completed (Success/Error status)",
		Buckets:   txLatencyBuckets,
	},
		[]string{"retried", "result", "error_type"},
	),
	TransactionStartedToCompletedLatencyTag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tss",
		Subsystem: "tx_processing",
		Name:      string(TransactionStartedToCompletedLatencyTag),
		Help:      "Latency (seconds) taken from when a Transaction was started to when it completed (Success/Error status)",
		Buckets:   txLatencyBuckets,
	},
		[]string{"retried", "result", "error_type"},
	),
	TransactionRetryCountTag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tss",
		Subsystem: "tx_processing",
		Name:      string(TransactionRetryCountTag),
		Help:      "Transaction retry count",
		Buckets:   prometheus.LinearBuckets(1, 1, 3), // 1 to 3 retries
	},
		[]string{"retried", "result", "error_type"},
	),
}

var SummaryTSSVecMetrics = map[MetricTag]*prometheus.SummaryVec{
	SuccessfulQueryDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  "tss",
		Subsystem:  "db",
		Name:       string(SuccessfulQueryDurationTag),
		Help:       "Successful DB query durations",
		Objectives: dbQueryObjectives,
	},
		[]string{"query_type"},
	),
	FailureQueryDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  "tss",
		Subsystem:  "db",
		Name:       string(FailureQueryDurationTag),
		Help:       "Failure DB query durations",
		Objectives: dbQueryObjectives,
	},
		[]string{"query_type"},
	),
	// HTTPRequestDurationTag is registered so tssPrometheusClient.MonitorHTTPRequestDuration can index
	// it without a nil-map panic (previously this tag was referenced but never registered).
	HTTPRequestDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  "tss",
		Subsystem:  "http",
		Name:       string(HTTPRequestDurationTag),
		Help:       "HTTP requests durations, sliding window = 10m",
		Objectives: dbQueryObjectives,
	},
		[]string{"status", "route", "method"},
	),
}

var CounterTSSMetrics = map[MetricTag]prometheus.Counter{}

// The per-event TSS counter label sets intentionally exclude event_id, tx_id and event_time: those
// are UNBOUNDED (one unique value per transaction) and as metric labels they mint a fresh Prometheus
// time series on every payment, exploding the TSDB and making the counters unaggregatable in Grafana
// rate() panels. Those identifiers are emitted in the logs instead (see TSSMonitorService.
// buildCommonFields). Only low-cardinality labels remain here.
var (
	paymentLabelNames              = []string{"event_type", "app_version", "git_commit_hash", "tenant_id", "wallet_id", "channel_account"}
	walletCreationLabelNames       = []string{"event_type", "app_version", "git_commit_hash", "tenant_id", "wallet_id", "channel_account"}
	sponsoredTransactionLabelNames = []string{"event_type", "app_version", "git_commit_hash", "tenant_id", "wallet_id", "channel_account"}
)

var CounterTSSVecMetrics = map[MetricTag]*prometheus.CounterVec{
	TransactionProcessedCounterTag: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tss",
		Subsystem: "tx_processing",
		Name:      string(TransactionProcessedCounterTag),
		Help:      "Count of transactions processed by TSS",
	},
		[]string{"retried", "result", "error_type"},
	),
	HorizonErrorCounterTag: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tss",
		Subsystem: "horizon_client",
		Name:      string(HorizonErrorCounterTag),
		Help:      "Count of Horizon related errors",
	},
		[]string{"status_code", "result_code"},
	),

	PaymentProcessingStartedTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(PaymentProcessingStartedTag),
			Help:      "Count of payments that are starting to process",
		},
		paymentLabelNames,
	),
	PaymentTransactionSuccessfulTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(PaymentTransactionSuccessfulTag),
			Help:      "Count of payments that have processed successfully",
		},
		paymentLabelNames,
	),
	PaymentReconciliationSuccessfulTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(PaymentReconciliationSuccessfulTag),
			Help:      "Count of payments that have completed reconciliation successfully",
		},
		paymentLabelNames,
	),
	PaymentReconciliationFailureTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(PaymentReconciliationFailureTag),
			Help:      "Count of payments that have failed reconciliation",
		},
		paymentLabelNames,
	),
	PaymentErrorTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(PaymentErrorTag),
			Help:      "Count of payments that have failed onchain",
		},
		paymentLabelNames,
	),

	WalletCreationProcessingStartedTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(WalletCreationProcessingStartedTag),
			Help:      "Count of wallet creations that are starting to process",
		},
		walletCreationLabelNames,
	),
	WalletCreationTransactionSuccessfulTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(WalletCreationTransactionSuccessfulTag),
			Help:      "Count of wallet creations that have processed successfully",
		},
		walletCreationLabelNames,
	),
	WalletCreationReconciliationSuccessfulTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(WalletCreationReconciliationSuccessfulTag),
			Help:      "Count of wallet creations that have completed reconciliation successfully",
		},
		walletCreationLabelNames,
	),
	WalletCreationReconciliationFailureTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(WalletCreationReconciliationFailureTag),
			Help:      "Count of wallet creations that have failed reconciliation",
		},
		walletCreationLabelNames,
	),
	WalletCreationErrorTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(WalletCreationErrorTag),
			Help:      "Count of wallet creations that have failed onchain",
		},
		walletCreationLabelNames,
	),

	SponsoredTransactionProcessingStartedTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(SponsoredTransactionProcessingStartedTag),
			Help:      "Count of sponsored transactions that are starting to process",
		},
		sponsoredTransactionLabelNames,
	),
	SponsoredTransactionTransactionSuccessfulTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(SponsoredTransactionTransactionSuccessfulTag),
			Help:      "Count of sponsored transactions that have processed successfully",
		},
		sponsoredTransactionLabelNames,
	),
	SponsoredTransactionReconciliationSuccessfulTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(SponsoredTransactionReconciliationSuccessfulTag),
			Help:      "Count of sponsored transactions that have completed reconciliation successfully",
		},
		sponsoredTransactionLabelNames,
	),
	SponsoredTransactionReconciliationFailureTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(SponsoredTransactionReconciliationFailureTag),
			Help:      "Count of sponsored transactions that have failed reconciliation",
		},
		sponsoredTransactionLabelNames,
	),
	SponsoredTransactionErrorTag: prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "tss",
			Name:      string(SponsoredTransactionErrorTag),
			Help:      "Count of sponsored transactions that have failed onchain",
		},
		sponsoredTransactionLabelNames,
	),
}
