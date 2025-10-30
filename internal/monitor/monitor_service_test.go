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

func Test_MetricsService_Start(t *testing.T) {
	monitorService := &MonitorService{}
	metricOptions := MetricOptions{}

	t.Run("start prometheus service metric", func(t *testing.T) {
		metricOptions.MetricType = "PROMETHEUS"
		err := monitorService.Start(metricOptions)
		require.NoError(t, err)

		require.IsType(t, &prometheusClient{}, monitorService.MonitorClient)
		assert.NotNil(t, monitorService.MonitorClient)
	})

	t.Run("error monitor service already initialized", func(t *testing.T) {
		metricOptions.MetricType = "MOCK_METRIC_TYPE"

		err := monitorService.Start(metricOptions)
		require.EqualError(t, err, "service already initialized")
	})

	t.Run("error unknown metric type", func(t *testing.T) {
		monitorService.MonitorClient = nil

		metricOptions.MetricType = "MOCK_METRIC_TYPE"
		err := monitorService.Start(metricOptions)
		require.EqualError(t, err, "error creating monitor client: unknown metric type: \"MOCK_METRIC_TYPE\"")
	})
}

func Test_MetricsService_GetMetricHTTPHandler(t *testing.T) {
	mMonitorClient := NewMockMonitorClient(t)
	monitorService := &MonitorService{MonitorClient: mMonitorClient}

	t.Run("running HttpServe with metric http handler", func(t *testing.T) {
		mHTTPHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"status": "OK"}`))
			require.NoError(t, err)
		})
		mMonitorClient.On("GetMetricHTTPHandler").Return(mHTTPHandler).Once()

		httpHandler, err := monitorService.GetMetricHTTPHandler()
		require.NoError(t, err)

		r := chi.NewRouter()
		r.Get("/metrics", httpHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		wantJSON := `{"status": "OK"}`
		assert.JSONEq(t, wantJSON, rr.Body.String())
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.MonitorClient = nil

		_, err := monitorService.GetMetricHTTPHandler()
		require.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_GetMetricType(t *testing.T) {
	mMonitorClient := NewMockMonitorClient(t)
	monitorService := &MonitorService{MonitorClient: mMonitorClient}

	t.Run("returns metric type", func(t *testing.T) {
		mMonitorClient.On("GetMetricType").Return(MetricType("MOCKMETRICTYPE")).Once()

		metricType, err := monitorService.GetMetricType()
		require.NoError(t, err)

		assert.Equal(t, MetricType("MOCKMETRICTYPE"), metricType)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.MonitorClient = nil

		_, err := monitorService.GetMetricType()
		require.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_MonitorRequestTime(t *testing.T) {
	mMonitorClient := NewMockMonitorClient(t)
	monitorService := &MonitorService{MonitorClient: mMonitorClient}

	mLabels := HTTPRequestLabels{
		Status: "200",
		Route:  "/mock",
		Method: "get",
	}

	mDuration := time.Duration(1)

	t.Run("monitor request time is called", func(t *testing.T) {
		mMonitorClient.On("MonitorHTTPRequestDuration", mDuration, mLabels).Once()
		err := monitorService.MonitorHTTPRequestDuration(mDuration, mLabels)

		assert.NoError(t, err)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.MonitorClient = nil

		err := monitorService.MonitorHTTPRequestDuration(mDuration, mLabels)
		assert.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_MonitorDBQueryDuration(t *testing.T) {
	mMonitorClient := NewMockMonitorClient(t)
	monitorService := &MonitorService{MonitorClient: mMonitorClient}

	mLabels := DBQueryLabels{
		QueryType: "SELECT",
	}

	mDuration := time.Duration(1)

	mMetricTag := MetricTag("mock")

	t.Run("monitor db query duration is called", func(t *testing.T) {
		mMonitorClient.On("MonitorDBQueryDuration", mDuration, mMetricTag, mLabels).Once()
		err := monitorService.MonitorDBQueryDuration(mDuration, mMetricTag, mLabels)

		require.NoError(t, err)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.MonitorClient = nil

		err := monitorService.MonitorDBQueryDuration(mDuration, mMetricTag, mLabels)
		require.EqualError(t, err, "client was not initialized")
	})
}

func Test_MetricsService_MonitorCounter(t *testing.T) {
	mMonitorClient := NewMockMonitorClient(t)
	monitorService := &MonitorService{MonitorClient: mMonitorClient}

	mMetricTag := MetricTag("mock")

	t.Run("monitor counter is called without labels", func(t *testing.T) {
		mMonitorClient.On("MonitorCounters", mMetricTag, map[string]string{}).Once()
		err := monitorService.MonitorCounters(mMetricTag, map[string]string{})

		assert.NoError(t, err)
	})

	t.Run("monitor counter is called with labels", func(t *testing.T) {
		labelsMock := map[string]string{
			"mock": "mock_value",
		}

		mMonitorClient.On("MonitorCounters", mMetricTag, labelsMock).Once()
		err := monitorService.MonitorCounters(mMetricTag, labelsMock)

		assert.NoError(t, err)
	})

	t.Run("error monitor client not initialized", func(t *testing.T) {
		monitorService.MonitorClient = nil

		err := monitorService.MonitorCounters(mMetricTag, nil)
		assert.EqualError(t, err, "client was not initialized")
	})
}

func Test_MonitorService_RegisterFunctionMetric(t *testing.T) {
	testCases := []struct {
		name        string
		metricType  FuncMetricType
		opts        FuncMetricOptions
		expectError bool
	}{
		{
			name:       "gauge metric",
			metricType: FuncGaugeType,
			opts: FuncMetricOptions{
				Namespace:  "test",
				Name:       "test_gauge",
				Help:       "Test gauge metric",
				Subservice: "test_service",
				Labels:     map[string]string{"env": "test"},
				Function:   func() float64 { return 42.0 },
			},
			expectError: false,
		},
		{
			name:       "counter metric",
			metricType: FuncCounterType,
			opts: FuncMetricOptions{
				Namespace:  "test",
				Name:       "test_counter",
				Help:       "Test counter metric",
				Subservice: "test_service",
				Labels:     map[string]string{"env": "test"},
				Function:   func() float64 { return 100.0 },
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mMonitorClient := NewMockMonitorClient(t)
			monitorService := &MonitorService{MonitorClient: mMonitorClient}

			// Expect the RegisterFunctionMetric call with custom matcher for opts
			mMonitorClient.On("RegisterFunctionMetric", tc.metricType,
				mock.MatchedBy(func(opts FuncMetricOptions) bool {
					return opts.Namespace == tc.opts.Namespace &&
						opts.Name == tc.opts.Name &&
						opts.Help == tc.opts.Help &&
						opts.Subservice == tc.opts.Subservice &&
						len(opts.Labels) == len(tc.opts.Labels) &&
						opts.Function != nil
				})).Once()

			// Call the method
			monitorService.RegisterFunctionMetric(tc.metricType, tc.opts)
		})
	}

	t.Run("client not initialized", func(t *testing.T) {
		monitorService := &MonitorService{}
		// MonitorClient is nil

		opts := FuncMetricOptions{
			Namespace:  "test",
			Name:       "test_metric",
			Help:       "Test metric",
			Subservice: "test_service",
			Function:   func() float64 { return 1.0 },
		}

		// Should not panic, just log error
		monitorService.RegisterFunctionMetric(FuncGaugeType, opts)
		// No assertions needed as this should just log and return
	})
}

func Test_FuncMetricOptions_Validation(t *testing.T) {
	t.Run("valid options", func(t *testing.T) {
		opts := FuncMetricOptions{
			Namespace:  "sdp",
			Name:       "test_metric",
			Help:       "Test metric help text",
			Subservice: "core",
			Labels:     map[string]string{"pool": "main"},
			Function:   func() float64 { return 123.45 },
		}

		// Verify all fields are set correctly
		assert.Equal(t, "sdp", opts.Namespace)
		assert.Equal(t, "test_metric", opts.Name)
		assert.Equal(t, "Test metric help text", opts.Help)
		assert.Equal(t, "core", opts.Subservice)
		assert.Equal(t, "main", opts.Labels["pool"])
		assert.NotNil(t, opts.Function)
		assert.Equal(t, 123.45, opts.Function())
	})
}

func Test_FuncMetricType_Constants(t *testing.T) {
	// Verify the constants are defined correctly
	assert.Equal(t, FuncMetricType("gauge"), FuncGaugeType)
	assert.Equal(t, FuncMetricType("counter"), FuncCounterType)
}
