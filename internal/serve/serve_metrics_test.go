package serve

import (
	"net/http"
	"testing"
	"time"

	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_ServeMetrics(t *testing.T) {
	mMonitorService := &monitor.MockMonitorService{}

	mMonitorService.On("GetMetricHttpHandler").
		Return(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		}), nil).Twice()

	opts := MetricsServeOptions{
		Port:           8002,
		MetricType:     "MOCKMETRICTYPE",
		MonitorService: mMonitorService,
	}

	// Mock supportHTTPRun
	mHTTPServer := mockHTTPServer{}
	mHTTPServer.On("Run", mock.AnythingOfType("http.Config")).Run(func(args mock.Arguments) {
		conf, ok := args.Get(0).(supporthttp.Config)
		require.True(t, ok, "should be of type supporthttp.Config")
		assert.Equal(t, ":8002", conf.ListenAddr)
		assert.Equal(t, time.Second*5, conf.ReadTimeout)
		assert.Equal(t, time.Second*10, conf.WriteTimeout)
		assert.Equal(t, time.Minute*2, conf.IdleTimeout)
		assert.Nil(t, conf.TLS)
		assert.ObjectsAreEqualValues(handleMetricsHttp(opts), conf.Handler)
	}).Once()

	// test and assert
	err := MetricsServe(opts, &mHTTPServer)
	require.NoError(t, err)
	mHTTPServer.AssertExpectations(t)
	mMonitorService.AssertExpectations(t)
}
