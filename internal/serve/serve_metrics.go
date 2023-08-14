package serve

import (
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

type MetricsServeOptions struct {
	Port        int
	Environment string

	MonitorService monitor.MonitorServiceInterface
	MetricType     monitor.MetricType
}

func MetricsServe(opts MetricsServeOptions, httpServer HTTPServerInterface) error {
	metricsAddr := fmt.Sprintf(":%d", opts.Port)
	metricsServerConfig := supporthttp.Config{
		ListenAddr:   metricsAddr,
		Handler:      handleMetricsHttp(opts),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  2 * time.Minute,
		OnStarting: func() {
			log.Infof("Starting %s Metrics Server", opts.MetricType)
			log.Infof("Listening on %s", metricsAddr)
		},
		OnStopping: func() {
			log.Infof("Stopping %s Metrics Server", opts.MetricType)
		},
	}

	httpServer.Run(metricsServerConfig)
	return nil
}

func handleMetricsHttp(opts MetricsServeOptions) *chi.Mux {
	mux := chi.NewMux()

	metricHttpHandler, err := opts.MonitorService.GetMetricHttpHandler()
	if err != nil {
		log.Fatalf("Error getting metric http.handler: %s", err.Error())
	}

	mux.Handle("/metrics", metricHttpHandler)
	return mux
}
