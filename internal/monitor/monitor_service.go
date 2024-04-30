package monitor

import (
	"fmt"
	"net/http"
	"time"
)

//go:generate mockery --name=MonitorServiceInterface --case=underscore --structname=MockMonitorService
type MonitorServiceInterface interface {
	Start(opts MetricOptions) error
	GetMetricType() (MetricType, error)
	GetMetricHttpHandler() (http.Handler, error)
	MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels) error
	MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels) error
	MonitorCounters(tag MetricTag, labels map[string]string) error
	MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string) error
	MonitorHistogram(value float64, tag MetricTag, labels map[string]string) error
}

var _ MonitorServiceInterface = (*MonitorService)(nil)

type MonitorService struct {
	MonitorClient MonitorClient
}

func (m *MonitorService) Start(opts MetricOptions) error {
	if m.MonitorClient != nil {
		return fmt.Errorf("service already initialized")
	}

	monitorClient, err := GetClient(opts)
	if err != nil {
		return fmt.Errorf("error creating monitor client: %w", err)
	}

	m.MonitorClient = monitorClient

	return nil
}

func (m *MonitorService) GetMetricType() (MetricType, error) {
	if m.MonitorClient == nil {
		return "", fmt.Errorf("client was not initialized")
	}

	return m.MonitorClient.GetMetricType(), nil
}

func (m *MonitorService) GetMetricHttpHandler() (http.Handler, error) {
	if m.MonitorClient == nil {
		return nil, fmt.Errorf("client was not initialized")
	}

	return m.MonitorClient.GetMetricHttpHandler(), nil
}

func (m *MonitorService) MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels) error {
	if m.MonitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.MonitorClient.MonitorHttpRequestDuration(duration, labels)

	return nil
}

func (m *MonitorService) MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels) error {
	if m.MonitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.MonitorClient.MonitorDBQueryDuration(duration, tag, labels)

	return nil
}

func (m *MonitorService) MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string) error {
	if m.MonitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.MonitorClient.MonitorDuration(duration, tag, labels)

	return nil
}

func (m *MonitorService) MonitorHistogram(value float64, tag MetricTag, labels map[string]string) error {
	if m.MonitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.MonitorClient.MonitorHistogram(value, tag, labels)

	return nil
}

func (m *MonitorService) MonitorCounters(tag MetricTag, labels map[string]string) error {
	if m.MonitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.MonitorClient.MonitorCounters(tag, labels)

	return nil
}
