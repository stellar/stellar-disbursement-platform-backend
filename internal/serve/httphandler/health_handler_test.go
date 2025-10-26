package httphandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

// test HealthHandler:
func TestHealthHandler(t *testing.T) {
	// create database connection pool
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	r := chi.NewRouter()
	handler := HealthHandler{
		Version:          "x.y.z",
		ServiceID:        "my-api",
		ReleaseID:        "1234567890abcdef",
		DBConnectionPool: dbConnectionPool,
	}
	r.Get("/health", handler.ServeHTTP)

	t.Run("✅SDP healthy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{
			"status": "pass",
			"version": "x.y.z",
			"service_id": "my-api",
			"release_id": "1234567890abcdef",
			"services": {
				"database": "pass"
			}
		}`, w.Body.String())
	})

	t.Run("❌SDP unhealthy because DB is down", func(t *testing.T) {
		// Close the ConnectionPool to simulate a DB failure
		closedConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		err = closedConnectionPool.Close()
		require.NoError(t, err)

		handler.DBConnectionPool = closedConnectionPool
		r.Get("/health", handler.ServeHTTP)

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.JSONEq(t, `{
			"status": "fail",
			"version": "x.y.z",
			"service_id": "my-api",
			"release_id": "1234567890abcdef",
			"services": {
				"database": "fail"
			}
		}`, w.Body.String())
	})
}
