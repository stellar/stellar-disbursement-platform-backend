package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
)

func Test_APIKeyOrJWTAuthenticate_SuccessfulAPIKey(t *testing.T) {
	t.Parallel()
	apiKeyModel := setupAPIKeyModel(t)

	expiry := time.Now().Add(1 * time.Hour)
	keyObj, err := apiKeyModel.Insert(context.Background(),
		"Ahrimanskey", []data.APIKeyPermission{data.ReadStatistics},
		[]string{"127.0.0.1"}, &expiry, "11111111-1111-1111-1111-111111111111",
	)
	require.NoError(t, err)

	var userID string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err = sdpcontext.GetUserIDFromContext(r.Context())
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	r.Use(APIKeyOrJWTAuthenticate(apiKeyModel, jwtAuthWithID("jwt-user")))
	r.Get("/test", h)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", keyObj.Key)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", userID)
}

func Test_APIKeyOrJWTAuthenticate_ExpiredKey(t *testing.T) {
	t.Parallel()
	apiKeyModel := setupAPIKeyModel(t)

	expiry := time.Now().Add(-1 * time.Hour)
	keyObj, err := apiKeyModel.Insert(context.Background(),
		"Ahrimanskey", []data.APIKeyPermission{data.ReadStatistics},
		nil, &expiry, "22222222-2222-2222-2222-222222222222",
	)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(APIKeyOrJWTAuthenticate(apiKeyModel, jwtAuthWithID("jwt-user")))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", keyObj.Key)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_APIKeyOrJWTAuthenticate_IPRestriction(t *testing.T) {
	t.Parallel()
	apiKeyModel := setupAPIKeyModel(t)

	expiry := time.Now().Add(1 * time.Hour)
	keyObj, err := apiKeyModel.Insert(context.Background(),
		"Ahrimanskey", []data.APIKeyPermission{data.ReadStatistics},
		[]string{"10.0.0.6", "10.0.0.8", "10.0.0.5"}, &expiry, "33333333-3333-3333-3333-333333333333",
	)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(APIKeyOrJWTAuthenticate(apiKeyModel, jwtAuthWithID("user")))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", keyObj.Key)
	req.RemoteAddr = "10.0.0.1:42"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func Test_APIKeyOrJWTAuthenticate_JWTFallback(t *testing.T) {
	t.Parallel()
	apiKeyModel := setupAPIKeyModel(t)

	// handler echoes the user ID from context
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := sdpcontext.GetUserIDFromContext(r.Context())
		_, _ = w.Write([]byte(id))
	})

	// JWT path sets id to 'jwt-user'
	r := chi.NewRouter()
	r.Use(APIKeyOrJWTAuthenticate(apiKeyModel, jwtAuthWithID("jwt-user")))
	r.Get("/test", h)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer token123")
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, "jwt-user", w.Body.String())
}

func setupAPIKeyModel(t *testing.T) *data.APIKeyModel {
	t.Helper()
	dbt := dbtest.Open(t)
	t.Cleanup(func() { dbt.Close() })

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	models, err := data.NewModels(pool)
	require.NoError(t, err)
	return models.APIKeys
}

func jwtAuthWithID(id string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := sdpcontext.SetUserIDInContext(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
