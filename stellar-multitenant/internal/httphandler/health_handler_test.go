package httphandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_HealthHandler(t *testing.T) {
	// test
	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	http.HandlerFunc(HealthHandler{
		GitCommit: "1234567890abcdef",
		Version:   "x.y.z",
	}.ServeHTTP).ServeHTTP(rr, req)

	// assert response
	assert.Equal(t, http.StatusOK, rr.Code)
	wantJSON := `{
			"status": "pass",
			"version": "x.y.z",
			"release_id": "1234567890abcdef"
		}`
	assert.JSONEq(t, wantJSON, rr.Body.String())
}
