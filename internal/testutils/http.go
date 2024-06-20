package testutils

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func Request(t *testing.T, ctx context.Context, r *chi.Mux, url, httpMethod string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, httpMethod, url, body)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}
