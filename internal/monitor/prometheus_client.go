package monitor

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stellar/go/support/log"
)

type prometheusClient struct {
	httpHandler http.Handler
}

func (prometheusClient) GetMetricType() MetricType {
	return MetricTypePrometheus
}

func (p *prometheusClient) GetMetricHttpHandler() http.Handler {
	return p.httpHandler
}

func (p *prometheusClient) MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels) {
	SummaryVecMetrics[HttpRequestDurationTag].With(prometheus.Labels{
		"status": labels.Status,
		"route":  labels.Route,
		"method": labels.Method,
	}).Observe(duration.Seconds())
}

func (p *prometheusClient) MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels) {
	summary := SummaryVecMetrics[tag]
	summary.With(prometheus.Labels{
		"query_type": labels.QueryType,
	}).Observe(duration.Seconds())
}

func (p *prometheusClient) MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string) {
	summary := SummaryVecMetrics[tag]
	summary.With(labels).Observe(duration.Seconds())
}

func (p *prometheusClient) MonitorCounters(tag MetricTag, labels map[string]string) {
	if len(labels) != 0 {
		if counterVecMetric, ok := CounterVecMetrics[tag]; ok {
			counterVecMetric.With(labels).Inc()
		} else {
			log.Errorf("metric not registered in Prometheus CounterVecMetrics: %s", tag)
		}
	} else {
		if counterMetric, ok := CounterMetrics[tag]; ok {
			counterMetric.Inc()
		} else {
			log.Errorf("metric not registered in Prometheus CounterMetrics: %s", tag)
		}
	}
}

func (p *prometheusClient) MonitorHistogram(value float64, tag MetricTag, labels map[string]string) {
	histogram := HistogramVecMetrics[tag]
	histogram.With(labels).Observe(value)
}

func NewPrometheusClient() (*prometheusClient, error) {
	// register Prometheus metrics
	metricsRegistry := prometheus.NewRegistry()

	var metricTag MetricTag
	for _, tag := range metricTag.ListAll() {
		if summaryVecMetric, ok := SummaryVecMetrics[tag]; ok {
			metricsRegistry.MustRegister(summaryVecMetric)
		} else if counterMetric, ok := CounterMetrics[tag]; ok {
			metricsRegistry.MustRegister(counterMetric)
		} else if counterVecMetric, ok := CounterVecMetrics[tag]; ok {
			metricsRegistry.MustRegister(counterVecMetric)
		} else {
			return nil, fmt.Errorf("metric not registered in prometheus metrics: %s", tag)
		}
	}

	return &prometheusClient{httpHandler: promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})}, nil
}

// Ensuring that promtheusClient is implementing MonitorClient interface
var _ MonitorClient = (*prometheusClient)(nil)
