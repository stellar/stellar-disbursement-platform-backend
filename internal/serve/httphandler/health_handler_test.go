package httphandler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stretchr/testify/mock"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test HealthHandler:
func TestHealthHandler(t *testing.T) {
	// create database connection pool
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	producerMock := events.NewMockProducer(t)

	r := chi.NewRouter()
	r.Get("/health", HealthHandler{
		Version:          "x.y.z",
		ServiceID:        "my-api",
		ReleaseID:        "1234567890abcdef",
		DBConnectionPool: dbConnectionPool,
		Producer:         producerMock,
		EventBrokerType:  events.KafkaEventBrokerType,
	}.ServeHTTP)

	t.Run("✅SDP healthy", func(t *testing.T) {
		producerMock.
			On("Ping", mock.Anything).
			Return(nil).
			Once()

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
				"database": "pass",
				"kafka": "pass"
			}
		}`, w.Body.String())
	})

	t.Run("❌SDP unhealthy", func(t *testing.T) {
		producerMock.
			On("Ping", mock.Anything).
			Return(errors.New("error")).
			Once()

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
				"database": "pass",	
				"kafka": "fail"
			}
		}`, w.Body.String())
	})

	t.Run("No healthcheck for Kafka event broker", func(t *testing.T) {
		r.Get("/health", HealthHandler{
			Version:          "x.y.z",
			ServiceID:        "my-api",
			ReleaseID:        "1234567890abcdef",
			DBConnectionPool: dbConnectionPool,
			Producer:         producerMock,
			EventBrokerType:  events.NoneEventBrokerType,
		}.ServeHTTP)

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
}
