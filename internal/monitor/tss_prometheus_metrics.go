package monitor

import "github.com/prometheus/client_golang/prometheus"

var HistogramTSSVecMetrics = map[MetricTag]*prometheus.HistogramVec{
	TransactionQueuedToCompletedLatencyTag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tss",
		Subsystem: "tx_processing",
		Name:      string(TransactionQueuedToCompletedLatencyTag),
		Help:      "Latency (seconds) taken from when a Transaction was created to when it completed (Success/Error status)",
		Buckets:   prometheus.LinearBuckets(5, 5, 24), // 5 seconds to 2 minutes
	},
		[]string{"retried", "result", "error_type"},
	),
	TransactionStartedToCompletedLatencyTag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tss",
		Subsystem: "tx_processing",
		Name:      string(TransactionStartedToCompletedLatencyTag),
		Help:      "Latency (seconds) taken from when a Transaction was started to when it completed (Success/Error status)",
		Buckets:   prometheus.LinearBuckets(5, 5, 24),
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
		Namespace: "tss",
		Subsystem: "db",
		Name:      string(SuccessfulQueryDurationTag),
		Help:      "Successful DB query durations",
	},
		[]string{"query_type"},
	),
	FailureQueryDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "tss",
		Subsystem: "db",
		Name:      string(FailureQueryDurationTag),
		Help:      "Failure DB query durations",
	},
		[]string{"query_type"},
	),
}

var CounterTSSMetrics = map[MetricTag]prometheus.Counter{}

var paymentLabelNames = []string{"event_id", "event_type", "tx_id", "event_time", "app_version", "git_commit_hash"}

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
}
