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

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	wantJson := `{
		"error": "An internal error occurred while processing this request."
	}`
	assert.JSONEq(t, wantJson, rr.Body.String())

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
	mMonitorService := &monitor.MockMonitorService{}

	// setup
	r := chi.NewRouter()
	r.Use(MetricsRequestHandler(mMonitorService))
	r.Get("/mock", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status": "OK"}`))
		require.NoError(t, err)
	})

	t.Run("monitor request with valid route", func(t *testing.T) {
		mLabels := monitor.HttpRequestLabels{
			Status: "200",
			Route:  "/mock",
			Method: "GET",
		}

		mMonitorService.On("MonitorHttpRequestDuration", mock.AnythingOfType("time.Duration"), mLabels).Return(nil).Once()

		// test
		req, err := http.NewRequest("GET", "/mock", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusOK, rr.Code)
		wantBody := `{"status": "OK"}`
		assert.JSONEq(t, wantBody, rr.Body.String())

		mMonitorService.AssertExpectations(t)
	})

	t.Run("monitor request with invalid route", func(t *testing.T) {
		mLabels := monitor.HttpRequestLabels{
			Status: "404",
			Route:  "undefined",
			Method: "GET",
		}

		mMonitorService.On("MonitorHttpRequestDuration", mock.AnythingOfType("time.Duration"), mLabels).Return(nil).Once()

		// test
		req, err := http.NewRequest("GET", "/invalid-route", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusNotFound, rr.Code)

		mMonitorService.AssertExpectations(t)
	})

	t.Run("monitor request with method not allowed", func(t *testing.T) {
		mLabels := monitor.HttpRequestLabels{
			Status: "405",
			Route:  "undefined",
			Method: "POST",
		}

		mMonitorService.On("MonitorHttpRequestDuration", mock.AnythingOfType("time.Duration"), mLabels).Return(nil).Once()

		// test
		req, err := http.NewRequest("POST", "/mock", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// assert response
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)

		mMonitorService.AssertExpectations(t)
	})
}

func Test_AuthenticateMiddleware(t *testing.T) {
	r := chi.NewRouter()

	jwtManagerMock := &auth.JWTManagerMock{}
	authManager := auth.NewAuthManager(auth.WithCustomJWTManagerOption(jwtManagerMock))

	r.Group(func(r chi.Router) {
		r.Use(AuthenticateMiddleware(authManager))

		r.Get("/authenticated", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
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

		jwtManagerMock.
			On("ValidateToken", mock.Anything, "token").
			Return(false, errors.New("unexpected error")).
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
		assert.Equal(t, `error validating auth token: validating token: unexpected error`, entries[0].Message)
	})

	t.Run("returns Unauthorized when the token is invalid", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)

		req.Header.Set("Authorization", "Bearer token")

		jwtManagerMock.
			On("ValidateToken", mock.Anything, "token").
			Return(false, nil).
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

		jwtManagerMock.
			On("ValidateToken", mock.Anything, "token").
			Return(true, nil).
			Once()

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"status":"ok"}`, string(respBody))
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

	setRestrictedEndpoint := func(ctx context.Context, r *chi.Mux, roles ...data.UserRole) {
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
		setRestrictedEndpoint(ctx, r, "role1", "role2")

		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Unauthorized when the token is expired and (no error is returned)", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), TokenContextKey, token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(ctx, r, "role1", "role2")

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
		ctx := context.WithValue(context.Background(), TokenContextKey, token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(ctx, r, "role1", "role2")

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
		ctx := context.WithValue(context.Background(), TokenContextKey, token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(ctx, r, "role1", "role2")

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
		ctx := context.WithValue(context.Background(), TokenContextKey, token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r := chi.NewRouter()
		setRestrictedEndpoint(ctx, r, "role1", "role2")

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

	t.Run("returns Unauthorized error when the user does not have the required roles", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), TokenContextKey, token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		requiredRoles := []data.UserRole{data.BusinessUserRole, data.FinancialControllerUserRole}

		r := chi.NewRouter()
		setRestrictedEndpoint(ctx, r, requiredRoles...)

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

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns Status Ok when user has the required roles", func(t *testing.T) {
		token := "mytoken"
		ctx := context.WithValue(context.Background(), TokenContextKey, token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		requiredRoles := []data.UserRole{data.BusinessUserRole, data.DeveloperUserRole}

		r := chi.NewRouter()
		setRestrictedEndpoint(ctx, r, requiredRoles...)

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
		ctx := context.WithValue(context.Background(), TokenContextKey, token)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()

		requiredRoles := []data.UserRole{}

		r := chi.NewRouter()
		setRestrictedEndpoint(ctx, r, requiredRoles...)

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
	mAuthManager := &auth.AuthManagerMock{}

	t.Run("emits request started and finished logs with tenant info if tenant derived from context", func(t *testing.T) {
		r := chi.NewRouter()
		expectedRespBody := "ok"

		infoEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		tenantName := "tenant123"
		tenantID := "tenant_id"
		token := "valid_token"
		mAuthManager.On("GetTenantID", mock.Anything, token).Return(tenantID, nil).Once()
		mTenantManager.On("GetTenantByID", mock.Anything, tenantID).Return(&tenant.Tenant{
			ID:   "tenant_id",
			Name: tenantName,
		}, nil).Once()

		r.Use(TenantMiddleware(mTenantManager, mAuthManager))
		r.Use(LoggingMiddleware())
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(expectedRespBody))
			require.NoError(t, err)
		})

		req, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)

		ctx := context.WithValue(req.Context(), TokenContextKey, token)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, expectedRespBody, string(respBody))

		logEntries := infoEntries()
		for i, e := range logEntries {
			entry, err := e.String()
			require.NoError(t, err)

			assert.Contains(t, entry, fmt.Sprintf("tenant_name=%s", tenantName))
			assert.Contains(t, entry, fmt.Sprintf("tenant_id=%s", tenantID))

			if i == 0 {
				assert.Contains(t, e.Message, "starting request")
			} else if i == 1 {
				assert.Contains(t, e.Message, "finished request")
			}
		}
	})

	t.Run("emits warning if tenant cannot be derived from the context", func(t *testing.T) {
		r := chi.NewRouter()
		expectedRespBody := "ok"

		infoEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		r.Use(LoggingMiddleware())
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

		logEntries := infoEntries()
		for i, e := range logEntries {
			entry, err := e.String()
			require.NoError(t, err)

			assert.NotContains(t, entry, "tenant_name")
			assert.NotContains(t, entry, "tenant_id")

			if i == 0 {
				assert.Contains(t, e.Message, "tenant cannot be derived from context")
			} else if i == 1 {
				assert.Contains(t, e.Message, "starting request")
			} else if i == 2 {
				assert.Contains(t, e.Message, "finished request")
			}
		}
	})

	mTenantManager.AssertExpectations(t)
	mAuthManager.AssertExpectations(t)
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

func Test_TenantMiddleware(t *testing.T) {
	r := chi.NewRouter()

	mTenantManager := &tenant.TenantManagerMock{}
	mAuthManager := &auth.AuthManagerMock{}

	r.Use(TenantMiddleware(mTenantManager, mAuthManager))

	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status":"ok"}`))
		require.NoError(t, err)
	})

	t.Run("failed to fetch tenant ID from token", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/test", nil)
		require.NoError(t, err)

		ctx := context.WithValue(req.Context(), TokenContextKey, "valid_token")
		req = req.WithContext(ctx)

		expectedErr := errors.New("error fetching tenant ID from token")
		mAuthManager.
			On("GetTenantID", mock.Anything, "valid_token").
			Return("", expectedErr).
			Once()

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		require.Contains(t, w.Body.String(), "Failed to get tenant ID from token")
	})

	t.Run("tenant name not found in request", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/test", nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Contains(t, w.Body.String(), "Tenant name not found in request or invalid")
	})

	t.Run("failed to load tenant by name", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/test", nil)
		require.NoError(t, err)

		req.Header.Set(TenantHeaderKey, "tenant_name")

		expectedErr := errors.New("error fetching tenant ID from token")
		mTenantManager.
			On("GetTenantByName", mock.Anything, "tenant_name").
			Return(nil, expectedErr).
			Once()

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		require.Contains(t, w.Body.String(), "Failed to load tenant by name")
	})

	t.Run("successfully extracts tenant ID from token", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/test", nil)
		require.NoError(t, err)

		ctx := context.WithValue(req.Context(), TokenContextKey, "valid_token")
		req = req.WithContext(ctx)

		mAuthManager.On("GetTenantID", mock.Anything, "valid_token").Return("tenant_id", nil)
		mTenantManager.On("GetTenantByID", mock.Anything, "tenant_id").Return(&tenant.Tenant{}, nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func Test_BasicAuthMiddleware(t *testing.T) {
	r := chi.NewRouter()

	adminAccount := "admin"
	adminApiKey := "secret"

	r.Group(func(r chi.Router) {
		r.Use(BasicAuthMiddleware(adminAccount, adminApiKey))

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
		req.SetBasicAuth(adminAccount, adminApiKey)

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

func Test_ExtractTenantNameFromRequest(t *testing.T) {
	t.Run("extract tenant name from header", func(t *testing.T) {
		expectedTenant := "tenant123"
		r, _ := http.NewRequest("GET", "http://example.com", nil)
		r.Header.Add(TenantHeaderKey, expectedTenant)

		tenantName, err := extractTenantNameFromRequest(r)
		require.NoError(t, err)
		require.Equal(t, expectedTenant, tenantName)
	})

	t.Run("extract tenant name from hostname", func(t *testing.T) {
		expectedTenant := "tenantfromhost"
		r, _ := http.NewRequest("GET", "http://tenantfromhost.example.com", nil)

		tenantName, err := extractTenantNameFromRequest(r)
		require.NoError(t, err)
		require.Equal(t, expectedTenant, tenantName)
	})

	t.Run("error extracting tenant from hostname", func(t *testing.T) {
		r, _ := http.NewRequest("GET", "http://example.com", nil)

		name, err := extractTenantNameFromRequest(r)
		require.ErrorIs(t, err, utils.ErrTenantNameNotFound)
		require.Empty(t, name)
	})

	t.Run("extract tenant name with port", func(t *testing.T) {
		expectedTenant := "tenantwithport"
		r, _ := http.NewRequest("GET", "http://tenantwithport.example.com:8080", nil)

		tenantName, err := extractTenantNameFromRequest(r)
		require.NoError(t, err)
		require.Equal(t, expectedTenant, tenantName)
	})
}
