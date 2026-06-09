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
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
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
		id, err := sdpcontext.GetUserIDFromContext(r.Context())
		require.NoError(t, err)
		_, err = w.Write([]byte(id))
		require.NoError(t, err)
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

// Verify that API key issued in one tenant's schema cannot authenticate
// a request routed to another tenant, including after key has been cached
func Test_APIKeyOrJWTAuthenticate_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	apiKeys, ctxA, ctxB, rawKey := setupCrossTenantAPIKeys(t)

	passThroughJWT := func(next http.Handler) http.Handler { return next }
	authn := APIKeyOrJWTAuthenticate(apiKeys, passThroughJWT)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	authenticateAs := func(tenantCtx context.Context) int {
		req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(tenantCtx)
		req.Header.Set("Authorization", rawKey)
		req.RemoteAddr = "127.0.0.1:1234"
		rr := httptest.NewRecorder()
		authn.ServeHTTP(rr, req)
		return rr.Code
	}

	assert.Equal(t, http.StatusUnauthorized, authenticateAs(ctxB),
		"tenant B must not authenticate with tenant A's key (cold path)")

	require.Equal(t, http.StatusOK, authenticateAs(ctxA),
		"tenant A (owner) must authenticate with its own key")

	// Can't force the cache warm, so warm cache bypass is covered by Test_apiKeyAuthenticator_validate_CrossTenantIsolation.
	assert.Equal(t, http.StatusUnauthorized, authenticateAs(ctxB),
		"tenant B must not be served tenant A's cached entry")

	require.Equal(t, http.StatusOK, authenticateAs(ctxA),
		"tenant A must still authenticate after cross-tenant attempts")
}

// White-box test for cross-tenant warm cache bypass
func Test_apiKeyAuthenticator_validate_CrossTenantIsolation(t *testing.T) {
	t.Parallel()

	apiKeys, ctxA, ctxB, rawKey := setupCrossTenantAPIKeys(t)

	auth := newAPIKeyAuthenticator(apiKeys)

	// Warm cache as tenant A, then block until ristretto has applied the write.
	warmed, err := auth.validate(ctxA, rawKey)
	require.NoError(t, err)
	require.NotNil(t, warmed)
	auth.cache.Wait()

	// Precondition: tenant A's entry really is warm before testing tenant B's access
	cached, err := auth.validate(ctxA, rawKey)
	require.NoError(t, err)
	require.Same(t, warmed, cached, "tenant A's second validate must be served from the cache")

	// Attack: same raw key, different tenant
	got, err := auth.validate(ctxB, rawKey)
	require.Error(t, err, "tenant B must not be served tenant A's warm cache entry")
	require.Nil(t, got)
}

func setupCrossTenantAPIKeys(t *testing.T) (apiKeys *data.APIKeyModel, ctxA, ctxB context.Context, rawKey string) {
	t.Helper()

	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	t.Cleanup(func() { dbt.Close() })

	basePool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	t.Cleanup(func() { basePool.Close() })

	tenant.PrepareDBForTenant(t, dbt, "tenant_a")
	tenant.PrepareDBForTenant(t, dbt, "tenant_b")

	router := tenant.NewMultiTenantDataSourceRouter(tenant.NewManager(tenant.WithDatabase(basePool)))
	routedPool, err := db.NewConnectionPoolWithRouter(router)
	require.NoError(t, err)
	t.Cleanup(func() { routedPool.Close() })

	models, err := data.NewModels(routedPool)
	require.NoError(t, err)

	tenantA := &schema.Tenant{ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "tenant_a"}
	tenantB := &schema.Tenant{ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Name: "tenant_b"}
	ctxA = sdpcontext.SetTenantInContext(context.Background(), tenantA)
	ctxB = sdpcontext.SetTenantInContext(context.Background(), tenantB)

	key, err := models.APIKeys.Insert(ctxA,
		"Cross Tenant Probe", []data.APIKeyPermission{data.ReadStatistics},
		nil, nil, "11111111-1111-1111-1111-111111111111",
	)
	require.NoError(t, err)

	return models.APIKeys, ctxA, ctxB, key.Key
}
