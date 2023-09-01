package monitor

import (
	"net/http"
	"time"

	"github.com/stretchr/testify/mock"
)

type MockMonitorService struct {
	mock.Mock
}

func (m *MockMonitorService) GetMetricHttpHandler() (http.Handler, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(http.Handler), args.Error(1)
}

func (m *MockMonitorService) GetMetricType() (MetricType, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return "", args.Error(1)
	}
	return args.Get(0).(MetricType), args.Error(1)
}

func (m *MockMonitorService) MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels) error {
	return m.Called(duration, labels).Error(0)
}

func (m *MockMonitorService) MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels) error {
	return m.Called(duration, tag, labels).Error(0)
}

func (m *MockMonitorService) MonitorCounters(tag MetricTag, labels map[string]string) error {
	return m.Called(tag, labels).Error(0)
}

func (m *MockMonitorService) MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string) error {
	return m.Called(duration, tag, labels).Error(0)
}

func (m *MockMonitorService) MonitorHistogram(value float64, tag MetricTag, labels map[string]string) error {
	return m.Called(value, tag, labels).Error(0)
}

func (m *MockMonitorService) Start(opts MetricOptions) error {
	return m.Called(opts).Error(0)
}

var _ MonitorServiceInterface = &MockMonitorService{}
