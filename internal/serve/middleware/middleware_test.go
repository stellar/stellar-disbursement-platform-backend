package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_RecoverHandler(t *testing.T) {
	// setup logger to assert the logged texts later
	buf := new(strings.Builder)
	log.DefaultLogger.SetOutput(buf)
	log.DefaultLogger.SetLevel(logrus.TraceLevel)

	// setup
	r := chi.NewRouter()
	r.Use(RecoverHandler)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// test
	req, err := http.NewRequest("GET", "/", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// assert response
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	wantJSON := `{
		"error": "An internal error occurred while processing this request.",
		"error_code": "500_0"
	}`
	assert.JSONEq(t, wantJSON, rr.Body.String())

	// assert logged text
	assert.Contains(t, buf.String(), "panic: test panic", "should log the panic message")
}

func Test_RecoverHandler_doesNotRecoverFromErrAbortHandler(t *testing.T) {
	// setup logger to assert the logged texts later
	buf := new(strings.Builder)
	log.DefaultLogger.SetOutput(buf)
	log.DefaultLogger.SetLevel(logrus.TraceLevel)

	// setup
	r := chi.NewRouter()
	r.Use(RecoverHandler)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})

	// test
	require.Panics(t, func() {
		req, err := http.NewRequest("GET", "/", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
	}, "http.ErrAbortHandler is supposed to panic")
}

func Test_MetricsRequestHandler(t *testing.T) {
	mMonitorService := monitorMocks.NewMockMonitorService(t)

	// setup
	r := chi.NewRouter()
	r.Use(MetricsRequestHandler(mMonitorService))
	r.Get("/mock", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status": "OK"}`))
		require.NoError(t, err)
	})

	t.Run("monitor request with valid route", func(t *testing.T) {
		mLabels := monitor.HTTPRequestLabels{
			Status: "200",
			Route:  "/mock",
			Method: "GET",
			CommonLabels: monitor.CommonLabels{
				TenantName: "no_tenant",
			},
		}

		mMonitorService.On("MonitorHTTPRequestDuration", mock.AnythingOfType("time.Duration"), mLabels).Return(nil).Once()

		// test
		req, err := http.NewRequest("GET", "/mock", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)
		wantBody := `{"status": "OK"}`
		assert.JSONEq(t, wantBody, rr.Body.String())
	})

	t.Run("monitor request with invalid route", func(t *testing.T) {
		mLabels := monitor.HTTPRequestLabels{
			Status: "404",
			Route:  "undefined",
			Method: "GET",
			CommonLabels: monitor.CommonLabels{
				TenantName: "no_tenant",
			},
		}

		mMonitorService.On("MonitorHTTPRequestDuration", mock.AnythingOfType("time.Duration"), mLabels).Return(nil).Once()

		// test
		req, err := http.NewRequest("GET", "/invalid-route", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("monitor request with method not allowed", func(t *testing.T) {
		mLabels := monitor.HTTPRequestLabels{
			Status: "405",
			Route:  "undefined",
			Method: "POST",
			CommonLabels: monitor.CommonLabels{
				TenantName: "no_tenant",
			},
		}

		mMonitorService.
			On("MonitorHTTPRequestDuration", mock.AnythingOfType("time.Duration"), mLabels).
			Return(nil).
			Once()

		// test
		req, err := http.NewRequest("POST", "/mock", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func Test_AuthenticateMiddleware(t *testing.T) {
	r := chi.NewRouter()

	mAuthManager := &auth.AuthManagerMock{}
	mTenantManager := &tenant.TenantManagerMock{}

	r.Group(func(r chi.Router) {
		r.Use(AuthenticateMiddleware(mAuthManager, mTenantManager))

		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Assert that the tenant is properly saved to the context
				ctx := r.Context()
				savedTenant, err := sdpcontext.GetTenantFromContext(ctx)
				require.NoError(t, err)
				assert.Equal(t, "test_tenant_id", savedTenant.ID)
				assert.Equal(t, "test_tenant", savedTenant.Name)
				next.ServeHTTP(w, r)
			})
		})

		r.Get("/authenticated", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
			log.Ctx(r.Context()).Info("authenticated route")
		})
	})

	r.Get("/unauthenticated", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
		require.NoError(t, err)
	})

	t.Run("returns Unauthorized error when no header is sent", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized error when a invalid header is sent", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)

		// Only one part
		req.Header.Set("Authorization", "BearerToken")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		req, err = http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)

		// More than two parts
		req.Header.Set("Authorization", "Bearer token token")

		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp = w.Result()
		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized when a unexpected error occurs validating the token", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)

		req.Header.Set("Authorization", "Bearer token")

		mAuthManager.
			On("GetUserID", mock.Anything, "token").
			Return("", errors.New("unexpected error")).
			Once()

		getEntries := log.DefaultLogger.StartTest(log.ErrorLevel)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))

		entries := getEntries()
		assert.NotEmpty(t, entries)
		assert.Equal(t, `error validating auth token: unexpected error`, entries[0].Message)
	})

	t.Run("returns Unauthorized when the token is invalid", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)

		req.Header.Set("Authorization", "Bearer token")

		mAuthManager.
			On("GetUserID", mock.Anything, "token").
			Return("", auth.ErrInvalidToken).
			Once()

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns the response successfully", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)

		req.Header.Set("Authorization", "Bearer token")

		mAuthManager.
			On("GetUserID", mock.Anything, "token").
			Return("test_user_id", nil).
			Once()
		mAuthManager.
			On("GetTenantID", mock.Anything, "token").
			Return("test_tenant_id", nil).
			Once()
		mTenantManager.
			On("GetTenantByID", mock.Anything, "test_tenant_id").
			Return(&schema.Tenant{
				ID:   "test_tenant_id",
				Name: "test_tenant",
			}, nil).
			Once()

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(respBody))

		// assert if user_id is in the logs:
		entries := getEntries()
		assert.NotEmpty(t, entries)
		assert.Contains(t, entries[0].Message, "authenticated route")
		assert.Equal(t, entries[0].Data["user_id"], "test_user_id")
	})

	t.Run("doesn't return Unauthorized for unauthenticated routes", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/unauthenticated", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(respBody))
	})
}

func Test_AnyRoleMiddleware(t *testing.T) {
	jwtManagerMock := &auth.JWTManagerMock{}
	roleManagerMock := &auth.RoleManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomJWTManagerOption(jwtManagerMock),
		auth.WithCustomRoleManagerOption(roleManagerMock),
	)

	const url = "/restricted"

	setRestrictedEndpoint := func(r *chi.Mux, roles ...data.UserRole) {
		r.With(AnyRoleMiddleware(authManager, roles...)).
			Get(url, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
				require.NoError(t, err)
			})
	}

	t.Run("returns Unauthorized when no token is in the request context", func(t *testing.T) {
		ctx := context.Background()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(r, "role1", "role2")

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized when the token is expired and (no error is returned)", func(t *testing.T) {
		token := "mytoken"
		ctx := sdpcontext.SetTokenInContext(context.Background(), token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(r, "role1", "role2")

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(false, nil).
			Once()

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized when the token is expired and (no auth.ErrInvalidToken error is returned)", func(t *testing.T) {
		token := "mytoken"
		ctx := sdpcontext.SetTokenInContext(context.Background(), token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(r, "role1", "role2")

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(false, auth.ErrInvalidToken).
			Once()

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized when the token is expired and (no auth.ErrUserNotFound error is returned)", func(t *testing.T) {
		token := "mytoken"
		ctx := sdpcontext.SetTokenInContext(context.Background(), token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(r, "role1", "role2")

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(false, auth.ErrUserNotFound).
			Once()

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Internal Server Error when an unexpected error occurs", func(t *testing.T) {
		token := "mytoken"
		ctx := sdpcontext.SetTokenInContext(context.Background(), token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(r, "role1", "role2")

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(false, errors.New("unexpected error")).
			Once()

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error":"An internal error occurred while processing this request."}`, string(respBody))
	})

	t.Run("returns Forbidden error when the user does not have the required roles", func(t *testing.T) {
		token := "mytoken"
		ctx := sdpcontext.SetTokenInContext(context.Background(), token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		requiredRoles := []data.UserRole{data.BusinessUserRole, data.FinancialControllerUserRole}

		r := chi.NewRouter()
		setRestrictedEndpoint(r, requiredRoles...)

		user := &auth.User{
			ID:    "user-id",
			Email: "email@email.com",
			Roles: []string{data.DeveloperUserRole.String()},
		}

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", mock.Anything, token).
			Return(user, nil).
			Once()

		roleManagerMock.
			On("HasAnyRoles", mock.Anything, user, data.FromUserRoleArrayToStringArray(requiredRoles)).
			Return(false, nil).
			Once()

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.JSONEq(t, `{"error":"You don't have permission to perform this action."}`, string(respBody))
	})

	t.Run("returns Status Ok when user has the required roles", func(t *testing.T) {
		token := "mytoken"
		ctx := sdpcontext.SetTokenInContext(context.Background(), token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		requiredRoles := []data.UserRole{data.BusinessUserRole, data.DeveloperUserRole}

		r := chi.NewRouter()
		setRestrictedEndpoint(r, requiredRoles...)

		user := &auth.User{
			ID:    "user-id",
			Email: "email@email",
			Roles: []string{data.DeveloperUserRole.String()},
		}

		jwtManagerMock.
			On("ValidateToken", mock.Anything, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", mock.Anything, token).
			Return(user, nil).
			Once()

		roleManagerMock.
			On("HasAnyRoles", mock.Anything, user, data.FromUserRoleArrayToStringArray(requiredRoles)).
			Return(true, nil).
			Once()

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(respBody))
	})

	t.Run("returns Status Ok when no roles is required", func(t *testing.T) {
		token := "mytoken"
		ctx := sdpcontext.SetTokenInContext(context.Background(), token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		requiredRoles := []data.UserRole{}

		r := chi.NewRouter()
		setRestrictedEndpoint(r, requiredRoles...)

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(respBody))
	})
}

func Test_CorsMiddleware(t *testing.T) {
	t.Run("Should work with an expected origin", func(t *testing.T) {
		r := chi.NewRouter()
		requestBaseURL := "http://myserver.com/*"
		expectedRespBody := "ok"

		r.Use(CorsMiddleware([]string{requestBaseURL}))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(expectedRespBody))
			require.NoError(t, err)
		})

		expectedReqOrigin := "http://myserver.com/custompage"
		req, err := http.NewRequest("GET", "/", nil)
		require.NoError(t, err)
		req.Header.Add("Origin", expectedReqOrigin)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, expectedReqOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, expectedRespBody, string(respBody))
	})

	t.Run("Should not return Access-Control-Allow-Origin header with unexpected origin", func(t *testing.T) {
		r := chi.NewRouter()
		requestBaseURL := "http://myserver.com"
		expectedRespBody := "ok"

		r.Use(CorsMiddleware([]string{requestBaseURL}))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(expectedRespBody))
			require.NoError(t, err)
		})

		reqOrigin := "http://locahost:8080"
		req, err := http.NewRequest("GET", "/", nil)
		require.NoError(t, err)
		req.Header.Add("Origin", reqOrigin)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, expectedRespBody, string(respBody))
	})
}

func Test_LoggingMiddleware(t *testing.T) {
	mTenantManager := &tenant.TenantManagerMock{}

	t.Run("emits request started and finished logs with tenant info if tenant derived from context", func(t *testing.T) {
		r := chi.NewRouter()
		expectedRespBody := "ok"

		debugEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		tenantName := "tenant123"
		tenantID := "tenant_id"
		token := "valid_token"
		mTenantManager.
			On("GetTenantByName", mock.Anything, tenantName).
			Return(&schema.Tenant{ID: tenantID, Name: tenantName}, nil).
			Once()
		r.Use(ResolveTenantFromRequestMiddleware(mTenantManager, false))
		r.Use(EnsureTenantMiddleware)
		r.Use(LoggingMiddleware)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(expectedRespBody))
			require.NoError(t, err)
		})

		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		req.Header.Set(TenantHeaderKey, tenantName)

		ctx := sdpcontext.SetTokenInContext(req.Context(), token)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, expectedRespBody, string(respBody))

		requestLogs := debugEntries()
		assert.Len(t, requestLogs, 2)

		for i, e := range requestLogs {
			entry, err := e.String()
			require.NoError(t, err)

			assert.Contains(t, entry, fmt.Sprintf("tenant_name=%s", tenantName))
			assert.Contains(t, entry, fmt.Sprintf("tenant_id=%s", tenantID))

			switch i {
			case 0:
				assert.Contains(t, e.Message, "starting request")
			case 1:
				assert.Contains(t, e.Message, "finished request")
			default:
				require.Fail(t, "unexpected log entry")
			}
			assert.Equal(t, logrus.InfoLevel, e.Level)
		}
	})

	t.Run("emits warning if tenant cannot be derived from the context", func(t *testing.T) {
		r := chi.NewRouter()
		expectedRespBody := "ok"

		debugEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		r.Use(LoggingMiddleware)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(expectedRespBody))
			require.NoError(t, err)
		})

		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		ctx := context.Background()
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, expectedRespBody, string(respBody))

		logEntries := debugEntries()

		assert.Len(t, logEntries, 3)
		for i, e := range logEntries {
			entry, err := e.String()
			require.NoError(t, err)

			assert.NotContains(t, entry, "tenant_name")
			assert.NotContains(t, entry, "tenant_id")

			switch i {
			case 0:
				assert.Contains(t, e.Message, "tenant cannot be derived from context")
				assert.Equal(t, log.DebugLevel, e.Level)
			case 1:
				assert.Contains(t, e.Message, "starting request")
				assert.Equal(t, log.InfoLevel, e.Level)
			case 2:
				assert.Contains(t, e.Message, "finished request")
				assert.Equal(t, log.InfoLevel, e.Level)
			default:
				require.Fail(t, "unexpected log entry")
			}
		}
	})

	mTenantManager.AssertExpectations(t)
}

func Test_CSPMiddleware(t *testing.T) {
	t.Run("Should populate the Content-Security-Policy header correctly", func(t *testing.T) {
		r := chi.NewRouter()
		expectedRespBody := "ok"

		r.Use(CSPMiddleware())
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(expectedRespBody))
			require.NoError(t, err)
		})

		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		wantCSP := "script-src 'self' https://www.google.com/recaptcha/ https://www.gstatic.com/recaptcha/;style-src 'self' https://www.google.com/recaptcha/ https://fonts.googleapis.com/css2 'unsafe-inline';connect-src 'self' https://www.google.com/recaptcha/ https://ipapi.co/json;font-src 'self' https://fonts.gstatic.com;default-src 'self';frame-src 'self' https://www.google.com/recaptcha/;frame-ancestors 'self';form-action 'self';"
		gotCSP := resp.Header.Get("Content-Security-Policy")
		assert.Equal(t, wantCSP, gotCSP)
		assert.Equal(t, expectedRespBody, string(respBody))
	})
}

func Test_ResolveTenantFromRequestMiddleware(t *testing.T) {
	validTnt := &schema.Tenant{ID: "tenant_id", Name: "tenant_name"}

	testCases := []struct {
		name              string
		tenantHeaderValue string
		hostnamePrefix    string
		singleTenantMode  bool
		prepareMocksFn    func(mTenantManager *tenant.TenantManagerMock)
		expectedStatus    int
		expectedRespBody  string
		expectedTenant    *schema.Tenant
	}{
		{
			name:              "🔴 tenant name from the header cannot be found in GetTenantByName",
			tenantHeaderValue: "tenant_name",
			hostnamePrefix:    "",
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				expectedErr := errors.New("error fetching tenant from its name")
				mTenantManager.
					On("GetTenantByName", mock.Anything, "tenant_name").
					Return(nil, expectedErr).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   nil,
		},
		{
			name:              "🔴 tenant name from the host prefix cannot be found in GetTenantByName",
			tenantHeaderValue: "",
			hostnamePrefix:    "tenant_hostname",
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				expectedErr := errors.New("error fetching tenant from its name")
				mTenantManager.
					On("GetTenantByName", mock.Anything, "tenant_hostname").
					Return(nil, expectedErr).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   nil,
		},
		{
			name:              "🟢 successfully grabs the tenant from the request HEADER",
			tenantHeaderValue: "tenant_name",
			hostnamePrefix:    "",
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				mTenantManager.
					On("GetTenantByName", mock.Anything, "tenant_name").
					Return(validTnt, nil).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   validTnt,
		},
		{
			name:              "🟢 successfully grabs the tenant from the request host prefix",
			tenantHeaderValue: "",
			hostnamePrefix:    "tenant_hostname",
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				mTenantManager.
					On("GetTenantByName", mock.Anything, "tenant_hostname").
					Return(validTnt, nil).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   validTnt,
		},
		{
			name:              "🟢 no default tenant is found",
			tenantHeaderValue: "",
			hostnamePrefix:    "",
			singleTenantMode:  true,
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				mTenantManager.
					On("GetDefault", mock.Anything).
					Return(nil, tenant.ErrTenantDoesNotExist).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   nil,
		},
		{
			name:              "🔴 too many default tenants",
			tenantHeaderValue: "",
			hostnamePrefix:    "",
			singleTenantMode:  true,
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				mTenantManager.
					On("GetDefault", mock.Anything).
					Return(nil, tenant.ErrTooManyDefaultTenants).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedRespBody: `{"error":"Too many default tenants configured"}`,
			expectedTenant:   nil,
		},
		{
			name:              "🟢 successfully gets the default tenant",
			tenantHeaderValue: "",
			hostnamePrefix:    "",
			singleTenantMode:  true,
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				mTenantManager.
					On("GetDefault", mock.Anything).
					Return(validTnt, nil).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   validTnt,
		},
		{
			name:              "🟢 successfully gets the default tenant regardless the header value",
			tenantHeaderValue: "some_tenant_name",
			hostnamePrefix:    "",
			singleTenantMode:  true,
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				mTenantManager.
					On("GetDefault", mock.Anything).
					Return(validTnt, nil).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   validTnt,
		},
		{
			name:              "🟢 successfully gets the default tenant regardless the host name prefix",
			tenantHeaderValue: "",
			hostnamePrefix:    "some_tenant_hostname",
			singleTenantMode:  true,
			prepareMocksFn: func(mTenantManager *tenant.TenantManagerMock) {
				mTenantManager.
					On("GetDefault", mock.Anything).
					Return(validTnt, nil).
					Once()
			},
			expectedStatus:   http.StatusOK,
			expectedRespBody: `{"status":"ok"}`,
			expectedTenant:   validTnt,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mTenantManager := &tenant.TenantManagerMock{}
			defer mTenantManager.AssertExpectations(t)

			// prepare mocks
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(mTenantManager)
			}

			updatedCtx := context.Background()
			// prepare router
			r := chi.NewRouter()
			r.
				With(ResolveTenantFromRequestMiddleware(mTenantManager, tc.singleTenantMode)).
				Get("/test", func(w http.ResponseWriter, r *http.Request) {
					updatedCtx = r.Context()
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte(`{"status":"ok"}`))
					require.NoError(t, err)
				})

			// prepare request
			req, err := http.NewRequest(http.MethodGet, "/test", nil)
			require.NoError(t, err)
			if tc.tenantHeaderValue != "" {
				req.Header.Set(TenantHeaderKey, tc.tenantHeaderValue)
			}
			if tc.hostnamePrefix != "" {
				req.Host = fmt.Sprintf("%s.example.com", tc.hostnamePrefix)
			}

			// execute the request
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			resp := w.Result()

			// assert the response
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.JSONEq(t, tc.expectedRespBody, string(respBody))

			// assert tenant in context
			tnt, err := sdpcontext.GetTenantFromContext(updatedCtx)
			if tc.expectedTenant != nil {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedTenant, tnt)
			} else {
				assert.Error(t, err)
				assert.Nil(t, tnt)
			}
		})
	}
}

func Test_EnsureTenantMiddleware(t *testing.T) {
	validTnt := &schema.Tenant{ID: "tenant_id", Name: "tenant_name"}

	testCases := []struct {
		name                 string
		hasTenantInCtx       bool
		expectedStatus       int
		expectedBodyContains string
		expectedTenant       *schema.Tenant
	}{
		{
			name:                 "🔴 fails if there's no tenant in the context",
			hasTenantInCtx:       false,
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: `{"error":"Tenant not found in context"}`,
			expectedTenant:       nil,
		},
		{
			name:                 "🟢 when there's a tenant in the context",
			hasTenantInCtx:       true,
			expectedStatus:       http.StatusOK,
			expectedBodyContains: `{"status":"ok"}`,
			expectedTenant:       nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// prepare router
			r := chi.NewRouter()
			r.
				With(EnsureTenantMiddleware).
				Get("/test", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte(`{"status":"ok"}`))
					require.NoError(t, err)
				})

			// prepare request
			req, err := http.NewRequest(http.MethodGet, "/test", nil)
			require.NoError(t, err)
			if tc.hasTenantInCtx {
				ctx := sdpcontext.SetTenantInContext(req.Context(), validTnt)
				req = req.WithContext(ctx)
			}

			// execute the request
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			resp := w.Result()

			// assert the response
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.JSONEq(t, tc.expectedBodyContains, string(respBody))
		})
	}
}

func Test_BasicAuthMiddleware(t *testing.T) {
	r := chi.NewRouter()

	adminAccount := "admin"
	adminAPIKey := "secret"

	r.Group(func(r chi.Router) {
		r.Use(BasicAuthMiddleware(adminAccount, adminAPIKey))

		r.Get("/authenticated", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"message":"🔐 secured content"}`))
			require.NoError(t, err)
		})
	})

	r.Get("/open", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(json.RawMessage(`{"message":"🔓 open content"}`))
		require.NoError(t, err)
	})

	t.Run("returns 401 error when no auth header is sent", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		assert.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns 401 error for incorrect credentials", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		assert.NoError(t, err)
		req.SetBasicAuth("wrongUser", "wrongPass")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("🎉 200 response for correct credentials", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		assert.NoError(t, err)
		req.SetBasicAuth(adminAccount, adminAPIKey)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message":"🔐 secured content"}`, string(respBody))
	})

	t.Run("🎉 200 response for open routes with no auth", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/open", nil)
		assert.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message":"🔓 open content"}`, string(respBody))
	})
}

func Test_MaxBodySize(t *testing.T) {
	testCases := []struct {
		name           string
		maxBytes       int64
		bodySize       int
		expectedStatus int
	}{
		{
			name:           "rejects body exceeding limit",
			maxBytes:       100,
			bodySize:       200,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:           "allows body at exactly the limit",
			maxBytes:       100,
			bodySize:       100,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "allows body under the limit",
			maxBytes:       100,
			bodySize:       50,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "allows empty body",
			maxBytes:       100,
			bodySize:       0,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "rejects when limit is zero and body is non-empty",
			maxBytes:       0,
			bodySize:       1,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Use(MaxBodySize(tc.maxBytes))
			r.Post("/", func(w http.ResponseWriter, r *http.Request) {
				_, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
					return
				}
				w.WriteHeader(http.StatusOK)
			})

			body := strings.NewReader(strings.Repeat("x", tc.bodySize))
			req, err := http.NewRequest(http.MethodPost, "/", body)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
		})
	}

	t.Run("GET requests with no body are unaffected", func(t *testing.T) {
		r := chi.NewRouter()
		r.Use(MaxBodySize(100))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func Test_ExtractTenantNameFromRequest(t *testing.T) {
	t.Run("extract tenant name from header", func(t *testing.T) {
		expectedTenant := "tenant123"
		r, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)
		r.Header.Add(TenantHeaderKey, expectedTenant)

		tenantName, err := extractTenantNameFromRequest(r)
		require.NoError(t, err)
		require.Equal(t, expectedTenant, tenantName)
	})

	t.Run("extract tenant name from hostname", func(t *testing.T) {
		expectedTenant := "tenantfromhost"
		r, err := http.NewRequest("GET", "http://tenantfromhost.example.com", nil)
		require.NoError(t, err)

		tenantName, err := extractTenantNameFromRequest(r)
		require.NoError(t, err)
		require.Equal(t, expectedTenant, tenantName)
	})

	t.Run("error extracting tenant from hostname", func(t *testing.T) {
		r, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		name, err := extractTenantNameFromRequest(r)
		require.ErrorIs(t, err, utils.ErrTenantNameNotFound)
		require.Empty(t, name)
	})

	t.Run("extract tenant name with port", func(t *testing.T) {
		expectedTenant := "tenantwithport"
		r, err := http.NewRequest("GET", "http://tenantwithport.example.com:8080", nil)
		require.NoError(t, err)

		tenantName, err := extractTenantNameFromRequest(r)
		require.NoError(t, err)
		require.Equal(t, expectedTenant, tenantName)
	})
}
