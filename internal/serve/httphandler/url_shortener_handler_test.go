package httphandler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_URLShortenerHandler_HandleRedirect(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	handler := URLShortenerHandler{Models: models}

	r := chi.NewRouter()
	r.Get("/r/{code}", handler.HandleRedirect)

	_, ctx := tenant.LoadDefaultTenantInContext(t, dbConnectionPool)

	// 🧪 Creating test data
	existingCode := "exist123"
	originalURL := "https://stellar.org/original"
	data.CreateShortURLFixture(t, ctx, dbConnectionPool, existingCode, originalURL)
	moreCode := "moreCode"
	moreURL := "https://stellar.org/more"
	data.CreateShortURLFixture(t, ctx, dbConnectionPool, moreCode, moreURL)

	testCases := []struct {
		name                string
		code                string
		expectedStatus      int
		expectedErrContains string
	}{
		{
			name:                "🎉successfully redirects to original URL",
			code:                existingCode,
			expectedStatus:      http.StatusMovedPermanently,
			expectedErrContains: "",
		},
		{
			name:                "returns 404 for non-existent code",
			code:                "does-not-exist",
			expectedStatus:      http.StatusNotFound,
			expectedErrContains: "Short URL not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/r/%s", tc.code), nil)
			require.NoError(t, reqErr)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			if tc.expectedErrContains != "" {
				body, readErr := io.ReadAll(rr.Body)
				require.NoError(t, readErr)
				assert.Contains(t, string(body), tc.expectedErrContains)
			}
		})
	}
}
