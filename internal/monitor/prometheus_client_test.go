package monitor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PrometheusClient_GetMetricType(t *testing.T) {
	mPrometheusClient := &prometheusClient{}

	metricType := mPrometheusClient.GetMetricType()
	assert.Equal(t, MetricTypePrometheus, metricType)
}

func Test_PrometheusClient_GetMetricHttpHandler(t *testing.T) {
	mPrometheusClient := &prometheusClient{}

	mHttpHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status": "OK"}`))
		require.NoError(t, err)
	})

	mPrometheusClient.httpHandler = mHttpHandler

	httpHandler := mPrometheusClient.GetMetricHttpHandler()

	r := chi.NewRouter()
	r.Get("/metrics", httpHandler.ServeHTTP)

	req, err := http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	wantJson := `{"status": "OK"}`
	assert.JSONEq(t, wantJson, rr.Body.String())
}

func Test_PrometheusClient_MonitorRequestTime(t *testing.T) {
	mPrometheusClient := &prometheusClient{}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(SummaryVecMetrics[HttpRequestDurationTag])

	mPrometheusClient.httpHandler = promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})

	mLabels := HttpRequestLabels{
		Status: "200",
		Route:  "/mock",
		Method: "GET",
		CommonLabels: CommonLabels{
			TenantName: "test-tenant",
		},
	}

	// initializing durations as 1 second
	mDuration := time.Second * 1

	mPrometheusClient.MonitorHttpRequestDuration(mDuration, mLabels)

	r := chi.NewRouter()
	r.Get("/metrics", mPrometheusClient.httpHandler.ServeHTTP)

	req, err := http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	resp := rr.Result()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, data)
	body := string(data)

	sumMetric := `sdp_http_requests_duration_seconds_sum{method="GET",route="/mock",status="200",tenant_name="test-tenant"} 1`
	countMetric := `sdp_http_requests_duration_seconds_count{method="GET",route="/mock",status="200",tenant_name="test-tenant"} 1`

	assert.Contains(t, body, sumMetric)
	assert.Contains(t, body, countMetric)
}

func Test_PrometheusClient_MonitorDBQueryDuration(t *testing.T) {
	mPrometheusClient := &prometheusClient{}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(SummaryVecMetrics[SuccessfulQueryDurationTag])
	metricsRegistry.MustRegister(SummaryVecMetrics[FailureQueryDurationTag])

	mPrometheusClient.httpHandler = promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})

	mLabels := DBQueryLabels{
		QueryType: "SELECT",
	}

	// initializing durations as 1 second
	mDuration := time.Second * 1

	// setup metric handler
	r := chi.NewRouter()
	r.Get("/metrics", mPrometheusClient.httpHandler.ServeHTTP)

	t.Run("successful db query metric", func(t *testing.T) {
		mPrometheusClient.MonitorDBQueryDuration(mDuration, SuccessfulQueryDurationTag, mLabels)
		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotEmpty(t, data)
		body := string(data)

		sumMetric := `sdp_db_successful_queries_duration_sum{query_type="SELECT"} 1`
		countMetric := `sdp_db_successful_queries_duration_count{query_type="SELECT"} 1`

		assert.Contains(t, body, sumMetric)
		assert.Contains(t, body, countMetric)
	})

	t.Run("failure db query metric", func(t *testing.T) {
		mPrometheusClient.MonitorDBQueryDuration(mDuration, FailureQueryDurationTag, mLabels)
		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotEmpty(t, data)
		body := string(data)

		sumMetric := `sdp_db_failure_queries_duration_sum{query_type="SELECT"} 1`
		countMetric := `sdp_db_failure_queries_duration_count{query_type="SELECT"} 1`

		assert.Contains(t, body, sumMetric)
		assert.Contains(t, body, countMetric)
	})
}

func Test_PrometheusClient_MonitorCounters(t *testing.T) {
	mPrometheusClient := &prometheusClient{}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(CounterVecMetrics[DisbursementsCounterTag])

	mPrometheusClient.httpHandler = promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})

	r := chi.NewRouter()
	r.Get("/metrics", mPrometheusClient.httpHandler.ServeHTTP)

	t.Run("disbursements counter metric", func(t *testing.T) {
		labels := DisbursementLabels{
			Asset:  "USDC",
			Wallet: "Mock Wallet",
			CommonLabels: CommonLabels{
				TenantName: "test-tenant",
			},
		}

		mPrometheusClient.MonitorCounters(DisbursementsCounterTag, labels.ToMap())

		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotEmpty(t, data)
		body := string(data)

		metric := `sdp_business_disbursements_counter{asset="USDC",tenant_name="test-tenant",wallet="Mock Wallet"} 1`

		assert.Contains(t, body, metric)

		// redefining disbursements counter metrics to have no influence on other tests
		CounterVecMetrics[DisbursementsCounterTag].Reset()
	})

	t.Run("counter vec metric not mapped on prometheus metrics", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.DefaultLogger.SetLevel(log.ErrorLevel)

		labelsMock := map[string]string{
			"mock": "mock_value",
		}

		mPrometheusClient.MonitorCounters(MetricTag("counter_vec_mock_tag"), labelsMock)

		require.Contains(t, buf.String(), `level=error msg="metric not registered in Prometheus CounterVecMetrics: counter_vec_mock_tag`)

		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, data)
	})

	t.Run("counter metric not mapped on prometheus metrics", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.DefaultLogger.SetLevel(log.ErrorLevel)

		mPrometheusClient.MonitorCounters(MetricTag("counter_mock_tag"), nil)

		require.Contains(t, buf.String(), `level=error msg="metric not registered in Prometheus CounterMetrics: counter_mock_tag`)

		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, data)
	})

	// TO-DO add tests for counter metrics when these metrics are added in the app
}

func Test_PrometheusClient_RegisterFunctionMetric(t *testing.T) {
	client, err := newPrometheusClient()
	require.NoError(t, err)

	t.Run("gauge function metric", func(t *testing.T) {
		called := false
		testFunc := func() float64 {
			called = true
			return 123.45
		}

		opts := FuncMetricOptions{
			Namespace:  "test",
			Subservice: "subsys",
			Name:       "gauge_test",
			Help:       "Test gauge",
			Labels:     map[string]string{"pool": "test"},
			Function:   testFunc,
		}

		// Should not panic
		client.RegisterFunctionMetric(FuncGaugeType, opts)

		// Verify function metric is accessible through HTTP handler
		r := chi.NewRouter()
		r.Get("/metrics", client.httpHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body := string(data)

		// Check that the metric appears in output
		assert.Contains(t, body, "test_subsys_gauge_test")
		assert.Contains(t, body, "123.45")
		assert.True(t, called, "Function should have been called")
	})

	t.Run("counter function metric", func(t *testing.T) {
		called := false
		testFunc := func() float64 {
			called = true
			return 456.78
		}

		opts := FuncMetricOptions{
			Namespace:  "test",
			Subservice: "subsys",
			Name:       "counter_test",
			Help:       "Test counter",
			Labels:     map[string]string{"pool": "test"},
			Function:   testFunc,
		}

		// Should not panic
		client.RegisterFunctionMetric(FuncCounterType, opts)

		// Verify function metric is accessible through HTTP handler
		r := chi.NewRouter()
		r.Get("/metrics", client.httpHandler.ServeHTTP)

		req, err := http.NewRequest("GET", "/metrics", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body := string(data)

		// Check that the metric appears in output
		assert.Contains(t, body, "test_subsys_counter_test")
		assert.Contains(t, body, "456.78")
		assert.True(t, called, "Function should have been called")
	})

	t.Run("unsupported metric type", func(t *testing.T) {
		// Capture log output
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.DefaultLogger.SetLevel(log.ErrorLevel)

		opts := FuncMetricOptions{
			Namespace:  "test",
			Subservice: "subsys",
			Name:       "unsupported_test",
			Help:       "Test unsupported",
			Function:   func() float64 { return 1.0 },
		}

		// Should not panic, but should log error
		client.RegisterFunctionMetric(FuncMetricType("invalid"), opts)

		// Check error was logged
		assert.Contains(t, buf.String(), "Error Registering Function invalid metric unsupported_test: unsupported metric type")
	})
}

func TestPrometheusClient_FunctionMetricIntegration(t *testing.T) {
	client, err := newPrometheusClient()
	require.NoError(t, err)

	// Register multiple function metrics
	value1 := 100.0
	value2 := 200.0

	opts1 := FuncMetricOptions{
		Namespace:  DefaultNamespace,
		Subservice: "db",
		Name:       "test_connections",
		Help:       "Test connections gauge",
		Labels:     map[string]string{"pool": "main"},
		Function:   func() float64 { return value1 },
	}

	opts2 := FuncMetricOptions{
		Namespace:  DefaultNamespace,
		Subservice: "db",
		Name:       "test_queries_total",
		Help:       "Test queries counter",
		Labels:     map[string]string{"pool": "main"},
		Function:   func() float64 { return value2 },
	}

	client.RegisterFunctionMetric(FuncGaugeType, opts1)
	client.RegisterFunctionMetric(FuncCounterType, opts2)

	// Get metrics output
	r := chi.NewRouter()
	r.Get("/metrics", client.httpHandler.ServeHTTP)

	req, err := http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	resp := rr.Result()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := string(data)

	// Verify both metrics appear
	assert.Contains(t, body, "sdp_db_test_connections")
	assert.Contains(t, body, "sdp_db_test_queries_total")
	assert.Contains(t, body, "100")
	assert.Contains(t, body, "200")

	// Update values and verify they change
	value1 = 150.0
	value2 = 250.0

	req2, err := http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)

	resp2 := rr2.Result()
	data2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	body2 := string(data2)

	// Values should be updated
	assert.Contains(t, body2, "150")
	assert.Contains(t, body2, "250")
}
