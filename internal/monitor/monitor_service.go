package monitor

import (
	"fmt"
	"net/http"
	"time"
)

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

type MonitorService struct {
	monitorClient MonitorClient
}

func (m *MonitorService) Start(opts MetricOptions) error {
	if m.monitorClient != nil {
		return fmt.Errorf("service already initialized")
	}

	monitorClient, err := GetClient(opts)
	if err != nil {
		return fmt.Errorf("error creating monitor client: %w", err)
	}

	m.monitorClient = monitorClient

	return nil
}

func (m *MonitorService) GetMetricType() (MetricType, error) {
	if m.monitorClient == nil {
		return "", fmt.Errorf("client was not initialized")
	}

	return m.monitorClient.GetMetricType(), nil
}

func (m *MonitorService) GetMetricHttpHandler() (http.Handler, error) {
	if m.monitorClient == nil {
		return nil, fmt.Errorf("client was not initialized")
	}

	return m.monitorClient.GetMetricHttpHandler(), nil
}

func (m *MonitorService) MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels) error {
	if m.monitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.monitorClient.MonitorHttpRequestDuration(duration, labels)

	return nil
}

func (m *MonitorService) MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels) error {
	if m.monitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.monitorClient.MonitorDBQueryDuration(duration, tag, labels)

	return nil
}

func (m *MonitorService) MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string) error {
	if m.monitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.monitorClient.MonitorDuration(duration, tag, labels)

	return nil
}

func (m *MonitorService) MonitorHistogram(value float64, tag MetricTag, labels map[string]string) error {
	if m.monitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.monitorClient.MonitorHistogram(value, tag, labels)

	return nil
}

func (m *MonitorService) MonitorCounters(tag MetricTag, labels map[string]string) error {
	if m.monitorClient == nil {
		return fmt.Errorf("client was not initialized")
	}

	m.monitorClient.MonitorCounters(tag, labels)

	return nil
}
