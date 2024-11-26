package monitor

import (
	"github.com/prometheus/client_golang/prometheus"
)

func PrometheusMetrics() map[MetricTag]prometheus.Collector {
	metrics := make(map[MetricTag]prometheus.Collector)

	for tag, summaryVec := range SummaryVecMetrics {
		metrics[tag] = summaryVec
	}

	for tag, counter := range CounterMetrics {
		metrics[tag] = counter
	}

	for tag, histogramVec := range HistogramVecMetrics {
		metrics[tag] = histogramVec
	}

	for tag, counterVec := range CounterVecMetrics {
		metrics[tag] = counterVec
	}

	return metrics
}

var SummaryVecMetrics = map[MetricTag]*prometheus.SummaryVec{
	HttpRequestDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "sdp", Subsystem: "http", Name: string(HttpRequestDurationTag),
		Help: "HTTP requests durations, sliding window = 10m",
	},
		[]string{"status", "route", "method"},
	),
	SuccessfulQueryDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "sdp", Subsystem: "db", Name: string(SuccessfulQueryDurationTag),
		Help: "Successful DB query durations",
	},
		[]string{"query_type"},
	),
	FailureQueryDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "sdp", Subsystem: "db", Name: string(FailureQueryDurationTag),
		Help: "Failure DB query durations",
	},
		[]string{"query_type"},
	),
}

var CounterMetrics = map[MetricTag]prometheus.Counter{
	AnchorPlatformAuthProtectionEnsuredCounterTag: prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "sdp", Subsystem: "anchor_platform", Name: string(AnchorPlatformAuthProtectionEnsuredCounterTag),
		Help: "A counter of how many times the anchor platform auth protection was ensured",
	}),
	AnchorPlatformAuthProtectionMissingCounterTag: prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "sdp", Subsystem: "anchor_platform", Name: string(AnchorPlatformAuthProtectionMissingCounterTag),
		Help: "A counter of how many times the anchor platform auth protection check revealed the AP is not protected",
	}),
}

var HistogramVecMetrics = map[MetricTag]*prometheus.HistogramVec{
	CircleAPIRequestDurationTag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "sdp", Subsystem: "circle", Name: string(CircleAPIRequestDurationTag),
		Help: "A histogram of the Circle API request durations",
	},
		CircleLabelNames,
	),
}

var CounterVecMetrics = map[MetricTag]*prometheus.CounterVec{
	DisbursementsCounterTag: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sdp", Subsystem: "business", Name: string(DisbursementsCounterTag),
		Help: "Disbursements Counter",
	},
		[]string{"asset", "wallet"},
	),
	CircleAPIRequestsTotalTag: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sdp", Subsystem: "circle", Name: string(CircleAPIRequestsTotalTag),
		Help: "A counter of the Circle API requests",
	},
		CircleLabelNames,
	),
}
