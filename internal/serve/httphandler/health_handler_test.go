package httphandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test HealthHandler:
func TestHealthHandler(t *testing.T) {
	// setup
	r := chi.NewRouter()
	r.Get("/health", HealthHandler{
		Version:   "x.y.z",
		ServiceID: "my-api",
		ReleaseID: "1234567890abcdef",
	}.ServeHTTP)

	// test
	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// assert response
	assert.Equal(t, http.StatusOK, rr.Code)
	wantJson := `{
		"status": "pass",
		"version": "x.y.z",
		"service_id": "my-api",
		"release_id": "1234567890abcdef"
	}`
	assert.JSONEq(t, wantJson, rr.Body.String())
}
