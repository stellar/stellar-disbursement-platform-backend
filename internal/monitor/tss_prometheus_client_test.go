package monitor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_TSSPrometheusClient_GetMetricType(t *testing.T) {
	mTSSPrometheusClient := &tssPrometheusClient{}

	metricType := mTSSPrometheusClient.GetMetricType()
	assert.Equal(t, MetricTypeTSSPrometheus, metricType)
}

func Test_TSSPrometheusClient_GetMetricHttpHandler(t *testing.T) {
	mTSSPrometheusClient := &tssPrometheusClient{}

	mHttpHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status": "OK"}`))
		require.NoError(t, err)
	})

	mTSSPrometheusClient.httpHandler = mHttpHandler

	httpHandler := mTSSPrometheusClient.GetMetricHttpHandler()

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

func Test_TSSPrometheusClient_MonitorDBQueryDuration(t *testing.T) {
	mTSSPrometheusClient := &tssPrometheusClient{}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(SummaryTSSVecMetrics[SuccessfulQueryDurationTag])
	metricsRegistry.MustRegister(SummaryTSSVecMetrics[FailureQueryDurationTag])

	mTSSPrometheusClient.httpHandler = promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})

	mLabels := DBQueryLabels{
		QueryType: "SELECT",
	}

	// initializing durations as 1 second
	mDuration := time.Second * 1

	// setup metric handler
	r := chi.NewRouter()
	r.Get("/metrics", mTSSPrometheusClient.httpHandler.ServeHTTP)

	t.Run("successful db query metric", func(t *testing.T) {
		mTSSPrometheusClient.MonitorDBQueryDuration(mDuration, SuccessfulQueryDurationTag, mLabels)
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

		sumMetric := `tss_db_successful_queries_duration_sum{query_type="SELECT"} 1`
		countMetric := `tss_db_successful_queries_duration_count{query_type="SELECT"} 1`

		assert.Contains(t, body, sumMetric)
		assert.Contains(t, body, countMetric)
	})

	t.Run("failure db query metric", func(t *testing.T) {
		mTSSPrometheusClient.MonitorDBQueryDuration(mDuration, FailureQueryDurationTag, mLabels)
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

		sumMetric := `tss_db_failure_queries_duration_sum{query_type="SELECT"} 1`
		countMetric := `tss_db_failure_queries_duration_count{query_type="SELECT"} 1`

		assert.Contains(t, body, sumMetric)
		assert.Contains(t, body, countMetric)
	})
}

func Test_TSSPrometheusClient_MonitorCounters(t *testing.T) {
	mTSSPrometheusClient := &tssPrometheusClient{}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(CounterTSSVecMetrics[TransactionProcessedCounterTag])
	metricsRegistry.MustRegister(CounterTSSVecMetrics[HorizonErrorCounterTag])

	mTSSPrometheusClient.httpHandler = promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})

	r := chi.NewRouter()
	r.Get("/metrics", mTSSPrometheusClient.httpHandler.ServeHTTP)

	t.Run("transactions processed counter metric", func(t *testing.T) {
		labels := map[string]string{
			"result":     "success",
			"error_type": "none",
			"retried":    "false",
		}

		mTSSPrometheusClient.MonitorCounters(TransactionProcessedCounterTag, labels)

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

		metric := `tss_tx_processing_processed_count{error_type="none",result="success",retried="false"} 1`

		assert.Contains(t, body, metric)

		CounterTSSVecMetrics[TransactionProcessedCounterTag].Reset()
	})

	t.Run("horizon errors counter metric", func(t *testing.T) {
		labels := map[string]string{
			"status_code": "123",
			"result_code": "321",
		}

		mTSSPrometheusClient.MonitorCounters(HorizonErrorCounterTag, labels)

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

		metric := `tss_horizon_client_error_count{result_code="321",status_code="123"} 1`

		assert.Contains(t, body, metric)

		CounterTSSVecMetrics[HorizonErrorCounterTag].Reset()
	})
}

func Test_TSSPrometheusClient_MonitorHistogram(t *testing.T) {
	mTSSPrometheusClient := &tssPrometheusClient{}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(HistogramTSSVecMetrics[TransactionRetryCountTag])
	metricsRegistry.MustRegister(HistogramTSSVecMetrics[TransactionQueuedToCompletedLatencyTag])
	metricsRegistry.MustRegister(HistogramTSSVecMetrics[TransactionStartedToCompletedLatencyTag])

	mTSSPrometheusClient.httpHandler = promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})

	r := chi.NewRouter()
	r.Get("/metrics", mTSSPrometheusClient.httpHandler.ServeHTTP)

	t.Run("transactions processed retry_count histogram metric", func(t *testing.T) {
		labels := map[string]string{
			"result":     "success",
			"error_type": "none",
			"retried":    "false",
		}

		mTSSPrometheusClient.MonitorHistogram(float64(3), TransactionRetryCountTag, labels)

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

		metric := `tss_tx_processing_retry_count_bucket{error_type="none",result="success",retried="false",le="3"} 1`

		assert.Contains(t, body, metric)

		HistogramTSSVecMetrics[TransactionRetryCountTag].Reset()
	})

	t.Run("transactions processed queued_to_completed_latency_seconds histogram metric", func(t *testing.T) {
		labels := map[string]string{
			"result":     "success",
			"error_type": "none",
			"retried":    "false",
		}

		mTSSPrometheusClient.MonitorHistogram(float64(15), TransactionQueuedToCompletedLatencyTag, labels)

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

		metric := `tss_tx_processing_queued_to_completed_latency_seconds_bucket{error_type="none",result="success",retried="false",le="15"} 1`

		assert.Contains(t, body, metric)

		HistogramTSSVecMetrics[TransactionQueuedToCompletedLatencyTag].Reset()
	})

	t.Run("transactions processed started_to_completed_latency_seconds histogram metric", func(t *testing.T) {
		labels := map[string]string{
			"result":     "success",
			"error_type": "none",
			"retried":    "false",
		}

		mTSSPrometheusClient.MonitorHistogram(float64(15), TransactionStartedToCompletedLatencyTag, labels)

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

		metric := `tss_tx_processing_started_to_completed_latency_seconds_bucket{error_type="none",result="success",retried="false",le="15"} 1`

		assert.Contains(t, body, metric)

		HistogramTSSVecMetrics[TransactionStartedToCompletedLatencyTag].Reset()
	})
}
