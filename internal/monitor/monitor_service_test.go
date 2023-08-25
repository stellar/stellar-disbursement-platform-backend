package monitor

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockMonitorClient struct {
	mock.Mock
}

func (m *mockMonitorClient) GetMetricHttpHandler() http.Handler {
	return m.Called().Get(0).(http.Handler)
}

func (m *mockMonitorClient) GetMetricType() MetricType {
	return m.Called().Get(0).(MetricType)
}

func (m *mockMonitorClient) MonitorHttpRequestDuration(duration time.Duration, labels HttpRequestLabels) {
	m.Called(duration, labels)
}

func (m *mockMonitorClient) MonitorDBQueryDuration(duration time.Duration, tag MetricTag, labels DBQueryLabels) {
	m.Called(duration, tag, labels)
}

func (m *mockMonitorClient) MonitorCounters(tag MetricTag, labels map[string]string) {
	m.Called(tag, labels)
}

func (m *mockMonitorClient) MonitorDuration(duration time.Duration, tag MetricTag, labels map[string]string) {
	m.Called(duration, tag, labels)
}

func (m *mockMonitorClient) MonitorHistogram(value float64, tag MetricTag, labels map[string]string) {
	m.Called(value, tag, labels)
}

var _ MonitorClient = &mockMonitorClient{}

func Test_MetricsService_Start(t *testing.T) {
	monitorService := &MonitorService{}
	metricOptions := MetricOptions{}

	t.Run("start prometheus service metric", func(t *testing.T) {
		metricOptions.MetricType = "PROMETHEUS"
		err := monitorService.Start(metricOptions)
		require.NoError(t, err)

		require.IsType(t, &prometheusClient{}, monitorService.monitorClient)
		assert.NotNil(t, monitorService.monitorClient)
	})

	t.Run("error monitor service already initialized", func(t *testing.T) {
		metricOptions.MetricType = "MOCK_METRIC_TYPE"

		err := monitorService.Start(metricOptions)
		require.EqualError(t, err, "service already initialized")
	})

	t.Run("error unknown metric type", func(t *testing.T) {
		monitorService.monitorClient = nil

		metricOptions.MetricType = "MOCK_METRIC_TYPE"
		err := monitorService.Start(metricOptions)
		require.EqualError(t, err, "error creating monitor client: unknown metric type: \"MOCK_METRIC_TYPE\"")
	})
}

func Test_MetricsService_GetMetricHttpHandler(t *testing.T) {
	monitorService := &MonitorService{}

	mMonitorClient := &mockMonitorClient{}
	monitorService.monitorClient = mMonitorClient

	t.Run("running HttpServe with metric http handler", func(t *testing.T) {
		mHttpHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"status": "OK"}`))
			require.NoError(t, err)
		})
		mMonitorClient.On("GetMetricHttpHandler").Return(mHttpHandler).Once()

		httpHandler, err := monitorService.GetMetricHttpHandler()
		require.NoError(t, err)

		r := chi.NewRouter()
		r.Get("/metrics", httpHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantJson := `{"status": "OK"}`
		assert.JSONEq(t, wantJson, rr.Body.String())
		mMonitorClient.AssertExpectations(t)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.monitorClient = nil

		_, err := monitorService.GetMetricHttpHandler()
		require.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_GetMetricType(t *testing.T) {
	monitorService := &MonitorService{}

	mMonitorClient := &mockMonitorClient{}
	monitorService.monitorClient = mMonitorClient

	t.Run("returns metric type", func(t *testing.T) {
		mMonitorClient.On("GetMetricType").Return(MetricType("MOCKMETRICTYPE")).Once()

		metricType, err := monitorService.GetMetricType()
		require.NoError(t, err)

		assert.Equal(t, MetricType("MOCKMETRICTYPE"), metricType)
		mMonitorClient.AssertExpectations(t)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.monitorClient = nil

		_, err := monitorService.GetMetricType()
		require.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_MonitorRequestTime(t *testing.T) {
	monitorService := &MonitorService{}

	mMonitorClient := &mockMonitorClient{}
	monitorService.monitorClient = mMonitorClient

	mLabels := HttpRequestLabels{
		Status: "200",
		Route:  "/mock",
		Method: "get",
	}

	mDuration := time.Duration(1)

	t.Run("monitor request time is called", func(t *testing.T) {
		mMonitorClient.On("MonitorHttpRequestDuration", mDuration, mLabels).Once()
		err := monitorService.MonitorHttpRequestDuration(mDuration, mLabels)

		require.NoError(t, err)
		mMonitorClient.AssertExpectations(t)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.monitorClient = nil

		err := monitorService.MonitorHttpRequestDuration(mDuration, mLabels)
		require.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_MonitorDBQueryDuration(t *testing.T) {
	monitorService := &MonitorService{}

	mMonitorClient := &mockMonitorClient{}
	monitorService.monitorClient = mMonitorClient

	mLabels := DBQueryLabels{
		QueryType: "SELECT",
	}

	mDuration := time.Duration(1)

	mMetricTag := MetricTag("mock")

	t.Run("monitor db query duration is called", func(t *testing.T) {
		mMonitorClient.On("MonitorDBQueryDuration", mDuration, mMetricTag, mLabels).Once()
		err := monitorService.MonitorDBQueryDuration(mDuration, mMetricTag, mLabels)

		require.NoError(t, err)
		mMonitorClient.AssertExpectations(t)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.monitorClient = nil

		err := monitorService.MonitorDBQueryDuration(mDuration, mMetricTag, mLabels)
		require.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_MonitorCounter(t *testing.T) {
	monitorService := &MonitorService{}

	mMonitorClient := &mockMonitorClient{}
	monitorService.monitorClient = mMonitorClient

	mMetricTag := MetricTag("mock")

	t.Run("monitor counter is called without labels", func(t *testing.T) {
		mMonitorClient.On("MonitorCounters", mMetricTag, map[string]string{}).Once()
		err := monitorService.MonitorCounters(mMetricTag, map[string]string{})

		require.NoError(t, err)
		mMonitorClient.AssertExpectations(t)
	})

	t.Run("monitor counter is called with labels", func(t *testing.T) {
		labelsMock := map[string]string{
			"mock": "mock_value",
		}

		mMonitorClient.On("MonitorCounters", mMetricTag, labelsMock).Once()
		err := monitorService.MonitorCounters(mMetricTag, labelsMock)

		require.NoError(t, err)
		mMonitorClient.AssertExpectations(t)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.monitorClient = nil

		err := monitorService.MonitorCounters(mMetricTag, nil)
		require.EqualError(t, err, "client was not initialized")
	})
}
