package monitor

import (
	"net/http"
	"time"
)

//go:generate mockery --name=MonitorClient --case=underscore --structname=MockMonitorClient --inpackage --filename=mocks.go
type MonitorClient interface {
	GetMetricHTTPHandler() http.Handler
	GetMetricType() MetricType
	RegisterFunctionMetric(metricType FuncMetricType, opts FuncMetricOptions)
	MonitorHTTPRequestDuration(duration time.Duration, labels HTTPRequestLabels)
	MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels)
	MonitorCounters(tag MetricTag, labels map[string]string)
	MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string)
	MonitorHistogram(value float64, tag MetricTag, labels map[string]string)
}
