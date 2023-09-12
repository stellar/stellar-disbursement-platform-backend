package monitor

import (
	"net/http"
	"time"
)

//go:generate mockery --name=MonitorClient --case=underscore --structname=MockMonitorClient
type MonitorClient interface {
	GetMetricHttpHandler() http.Handler
	GetMetricType() MetricType
	MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels)
	MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels)
	MonitorCounters(tag MetricTag, labels map[string]string)
	MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string)
	MonitorHistogram(value float64, tag MetricTag, labels map[string]string)
}
