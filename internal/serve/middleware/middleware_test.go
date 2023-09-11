package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
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
