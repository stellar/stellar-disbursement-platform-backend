package httphandler

import (
	"net/http"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"

	"github.com/stellar/go/support/render/httpjson"
)

// Status indicates whether the service is health or not.
type Status string

const (
	// StatusPass indicates that the service is healthy.
	StatusPass Status = "pass"
	// StatusFail indicates that the service is unhealthy.
	StatusFail Status = "fail"
)

// HealthResponse follows the health check response format for HTTP APIs,
// based on the format defined in the draft IETF network working group
// standard, Health Check Response Format for HTTP APIs.
//
// https://datatracker.ietf.org/doc/html/draft-inadarei-api-health-check-06#name-api-health-response
type HealthResponse struct {
	Status    Status            `json:"status"`
	Version   string            `json:"version,omitempty"`
	ServiceID string            `json:"service_id,omitempty"`
	ReleaseID string            `json:"release_id,omitempty"`
	Services  map[string]Status `json:"services,omitempty"`
}

// HealthHandler implements a simple handler that returns the health response.
type HealthHandler struct {
	Version          string
	ServiceID        string
	ReleaseID        string
	DBConnectionPool db.DBConnectionPool
	Producer         events.Producer
}

// ServeHTTP implements the http.Handler interface.
func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	context := r.Context()

	dbStatus, responseStatus := StatusPass, StatusPass
	if err := h.DBConnectionPool.Ping(context); err != nil {
		dbStatus = StatusFail
		responseStatus = StatusFail
	}

	services := map[string]Status{
		"database": dbStatus,
	}

	if h.Producer != nil && h.Producer.BrokerType() == events.KafkaEventBrokerType {
		eventBrokerStatus := StatusPass
		if err := h.Producer.Ping(context); err != nil {
			eventBrokerStatus = StatusFail
			responseStatus = StatusFail
		}
		services["kafka"] = eventBrokerStatus
	}

	response := HealthResponse{
		Status:    responseStatus,
		Version:   h.Version,
		ServiceID: h.ServiceID,
		ReleaseID: h.ReleaseID,
		Services:  services,
	}

	// If any of the services are unhealthy, return a 503 Service Unavailable status.
	// This signals to the orchestrator that the service is not healthy.
	if response.Status == StatusFail {
		httpjson.RenderStatus(w, http.StatusServiceUnavailable, response, httpjson.JSON)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}
