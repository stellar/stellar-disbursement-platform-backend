package httphandler

import (
	"net/http"

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
	Status    Status `json:"status"`
	Version   string `json:"version,omitempty"`
	ServiceID string `json:"service_id,omitempty"`
	ReleaseID string `json:"release_id,omitempty"`
}

// HealthHandler implements a simple handler that returns the health response.
type HealthHandler struct {
	Version   string
	ServiceID string
	ReleaseID string
}

// ServeHTTP implements the http.Handler interface.
func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    StatusPass,
		Version:   h.Version,
		ServiceID: h.ServiceID,
		ReleaseID: h.ReleaseID,
	}

	// TODO: after we have a DB connection, we should check if the DB is healthy
	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}
