package httphandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ListRoles(t *testing.T) {
	r := chi.NewRouter()

	r.Get("/users/roles", ListRolesHandler{}.GetRoles)

	req, err := http.NewRequest(http.MethodGet, "/users/roles", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	resp := w.Result()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.JSONEq(t, `{"roles": ["owner", "financial_controller",  "developer", "business"]}`, string(respBody))
}
