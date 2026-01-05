package monitor

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stellar/go-stellar-sdk/support/log"
)

type prometheusClient struct {
	httpHandler http.Handler
	registry    *prometheus.Registry
}

func (p *prometheusClient) GetMetricType() MetricType {
	return MetricTypePrometheus
}

func (p *prometheusClient) GetMetricHTTPHandler() http.Handler {
	return p.httpHandler
}

func (p *prometheusClient) MonitorHTTPRequestDuration(duration time.Duration, labels HTTPRequestLabels) {
	SummaryVecMetrics[HTTPRequestDurationTag].With(prometheus.Labels{
		"status":      labels.Status,
		"route":       labels.Route,
		"method":      labels.Method,
		"tenant_name": labels.TenantName,
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

func (p *prometheusClient) RegisterFunctionMetric(metricType FuncMetricType, opts FuncMetricOptions) {
	var metric prometheus.Collector

	switch metricType {
	case FuncGaugeType:
		metric = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: opts.Namespace, Subsystem: opts.Subservice, Name: opts.Name,
				Help:        opts.Help,
				ConstLabels: opts.Labels,
			},
			opts.Function,
		)
	case FuncCounterType:
		metric = prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Namespace: opts.Namespace, Subsystem: opts.Subservice, Name: opts.Name,
				Help:        opts.Help,
				ConstLabels: opts.Labels,
			},
			opts.Function,
		)
	default:
		log.Errorf("Error Registering Function %s metric %s: unsupported metric type", metricType, opts.Name)
		return
	}

	p.registry.MustRegister(metric)
}

func newPrometheusClient() *prometheusClient {
	// register Prometheus metrics
	metricsRegistry := prometheus.NewRegistry()

	// register default Prometheus metrics
	metricsRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsRegistry.MustRegister(collectors.NewGoCollector())

	for _, metric := range PrometheusMetrics() {
		metricsRegistry.MustRegister(metric)
	}

	return &prometheusClient{
		httpHandler: promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}),
		registry:    metricsRegistry,
	}
}

// Ensuring that promtheusClient is implementing MonitorClient interface
var _ MonitorClient = (*prometheusClient)(nil)
