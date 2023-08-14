package monitor

import (
	"fmt"
	"strings"
)

type MetricType string

const (
	MetricTypePrometheus    MetricType = "PROMETHEUS"
	MetricTypeTSSPrometheus MetricType = "TSS_PROMETHEUS"
)

func ParseMetricType(metricTypeStr string) (MetricType, error) {
	metricTypeStrUpper := strings.ToUpper(metricTypeStr)
	mType := MetricType(metricTypeStrUpper)

	switch mType {
	case MetricTypePrometheus:
		return mType, nil
	case MetricTypeTSSPrometheus:
		return mType, nil
	default:
		return "", fmt.Errorf("invalid metric type %q", metricTypeStrUpper)
	}
}

type MetricOptions struct {
	MetricType  MetricType
	Environment string
}

func GetClient(opts MetricOptions) (MonitorClient, error) {
	switch opts.MetricType {
	case MetricTypePrometheus:
		return NewPrometheusClient()
	case MetricTypeTSSPrometheus:
		return NewTSSPrometheusClient()
	default:
		return nil, fmt.Errorf("unknown metric type: %q", opts.MetricType)
	}
}
