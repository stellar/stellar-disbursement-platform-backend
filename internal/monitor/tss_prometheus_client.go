package monitor

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
)

type tssPrometheusClient struct {
	httpHandler http.Handler
}

// Metrics is a logrus hook-compliant struct that records metrics about logging
// when added to a logrus.Logger
type Metrics map[logrus.Level]prometheus.Counter

// Fire is triggered by logrus, in response to a logging event
func (m *Metrics) Fire(e *logrus.Entry) error {
	(*m)[e.Level].Inc()
	return nil
}

// Levels returns the logging levels that will trigger this hook to run.  In
// this case, all of them.
func (m *Metrics) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.WarnLevel,
		logrus.ErrorLevel,
		logrus.PanicLevel,
	}
}

func (tssPrometheusClient) GetMetricType() MetricType {
	return MetricTypeTSSPrometheus
}

func (p *tssPrometheusClient) GetMetricHttpHandler() http.Handler {
	return p.httpHandler
}

func (p *tssPrometheusClient) MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels) {
	SummaryTSSVecMetrics[HttpRequestDurationTag].With(prometheus.Labels{
		"status": labels.Status,
		"route":  labels.Route,
		"method": labels.Method,
	}).Observe(duration.Seconds())
}

func (p *tssPrometheusClient) MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels) {
	summary := SummaryTSSVecMetrics[tag]
	summary.With(prometheus.Labels{
		"query_type": labels.QueryType,
	}).Observe(duration.Seconds())
}

func (p *tssPrometheusClient) MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string) {
	summary := SummaryTSSVecMetrics[tag]
	summary.With(labels).Observe(duration.Seconds())
}

func (p *tssPrometheusClient) MonitorCounters(tag MetricTag, labels map[string]string) {
	if len(labels) != 0 {
		if counterVecMetric, ok := CounterTSSVecMetrics[tag]; ok {
			counterVecMetric.With(labels).Inc()
		} else {
			log.Errorf("metric not registered in prometheus metrics: %s", tag)
		}
	} else {
		if counterMetric, ok := CounterTSSMetrics[tag]; ok {
			counterMetric.Inc()
		} else {
			log.Errorf("metric not registered in prometheus metrics: %s", tag)
		}
	}
}

func (p *tssPrometheusClient) MonitorHistogram(value float64, tag MetricTag, labels map[string]string) {
	histogram := HistogramTSSVecMetrics[tag]
	histogram.With(labels).Observe(value)
}

// NewTSSPrometheusClient registers Prometheus metrics for the Transaction Submission Service
func NewTSSPrometheusClient() (*tssPrometheusClient, error) {
	// register Prometheus metrics
	metricsRegistry := prometheus.NewRegistry()

	// register default Prometheus metrics
	metricsRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsRegistry.MustRegister(collectors.NewGoCollector())

	var tssMetricTag MetricTag
	for _, tag := range tssMetricTag.ListAllTSSMetricTags() {
		if summaryTSSVecMetric, ok := SummaryTSSVecMetrics[tag]; ok {
			metricsRegistry.MustRegister(summaryTSSVecMetric)
		} else if counterTSSMetric, ok := CounterTSSMetrics[tag]; ok {
			metricsRegistry.MustRegister(counterTSSMetric)
		} else if counterTSSVecMetric, ok := CounterTSSVecMetrics[tag]; ok {
			metricsRegistry.MustRegister(counterTSSVecMetric)
		} else if histogramTSSVecMetric, ok := HistogramTSSVecMetrics[tag]; ok {
			metricsRegistry.MustRegister(histogramTSSVecMetric)
		} else {
			return nil, fmt.Errorf("metric not registered in prometheus metrics: %s", tag)
		}
	}

	// create a logging hook that increments a Prometheus counter for each log level
	logCounterHook := &Metrics{
		logrus.WarnLevel: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "tss", Subsystem: "log", Name: "warn_total",
		}),
		logrus.ErrorLevel: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "tss", Subsystem: "log", Name: "error_total",
		}),
		logrus.PanicLevel: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "tss", Subsystem: "log", Name: "panic_total",
		}),
	}

	for _, metric := range *logCounterHook {
		metricsRegistry.MustRegister(metric)
	}

	// add the logCounterHook to the logger
	log.DefaultLogger.AddHook(logCounterHook)

	return &tssPrometheusClient{httpHandler: promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})}, nil
}

// Ensuring that promtheusClient is implementing MonitorClient interface
var _ MonitorClient = (*tssPrometheusClient)(nil)
