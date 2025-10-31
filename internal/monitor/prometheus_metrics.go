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
	HTTPRequestDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: DefaultNamespace, Subsystem: string(HTTPSubservice), Name: string(HTTPRequestDurationTag),
		Help: "HTTP requests durations, sliding window = 10m",
		Objectives: map[float64]float64{
			0.5:  0.05,  // 50th percentile with 5% error
			0.9:  0.01,  // 90th percentile with 1% error
			0.95: 0.01,  // 95th percentile with 1% error
			0.99: 0.001, // 99th percentile with 0.1% error
		},
	},
		[]string{"status", "route", "method", "tenant_name"},
	),
	SuccessfulQueryDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: DefaultNamespace, Subsystem: string(DBSubservice), Name: string(SuccessfulQueryDurationTag),
		Help: "Successful DB query durations",
	},
		[]string{"query_type"},
	),
	FailureQueryDurationTag: prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: DefaultNamespace, Subsystem: string(DBSubservice), Name: string(FailureQueryDurationTag),
		Help: "Failure DB query durations",
	},
		[]string{"query_type"},
	),
}

var CounterMetrics = map[MetricTag]prometheus.Counter{}

var HistogramVecMetrics = map[MetricTag]*prometheus.HistogramVec{
	CircleAPIRequestDurationTag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: DefaultNamespace, Subsystem: string(CircleSubservice), Name: string(CircleAPIRequestDurationTag),
		Help: "A histogram of the Circle API request durations",
	},
		CircleLabelNames,
	),
}

var CounterVecMetrics = map[MetricTag]*prometheus.CounterVec{
	DisbursementsCounterTag: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: DefaultNamespace, Subsystem: string(BusinessSubservice), Name: string(DisbursementsCounterTag),
		Help: "Disbursements Counter",
	},
		[]string{"asset", "wallet", "tenant_name"},
	),
	CircleAPIRequestsTotalTag: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: DefaultNamespace, Subsystem: string(CircleSubservice), Name: string(CircleAPIRequestsTotalTag),
		Help: "A counter of the Circle API requests",
	},
		CircleLabelNames,
	),
}
