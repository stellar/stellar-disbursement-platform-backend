package httphandler

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
)

func Test_isStaticAsset(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "root path",
			path:     "/",
			expected: false,
		},
		{
			name:     "path without extension in root",
			path:     "/some-page",
			expected: false,
		},
		{
			name:     "file with extension in root",
			path:     "/main.js",
			expected: true,
		},
		{
			name:     "file with extension with leading slash",
			path:     "/favicon.ico",
			expected: true,
		},
		{
			name:     "file with empty extension",
			path:     "/file.",
			expected: true,
		},
		{
			name:     "path in subdirectory without extension",
			path:     "/assets/image",
			expected: true,
		},
		{
			name:     "path in subdirectory with extension",
			path:     "/assets/images/logo.png",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isStaticAsset(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_serveReactApp(t *testing.T) {
	ctx := context.Background()
	validClaims := &anchorplatform.SEP24JWTClaims{
		ClientDomainClaim: "test.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}

	testCases := []struct {
		name                string
		requestURL          string
		ctx                 context.Context
		setupFS             func() fs.FS
		expectedStatus      int
		expectedBody        string
		expectedContentType string
	}{
		{
			name:       "🟢successfully serves index.html",
			requestURL: "/wallet-registration/start?token=test-token",
			ctx:        context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, validClaims),
			setupFS: func() fs.FS {
				return fstest.MapFS{
					"index.html": &fstest.MapFile{
						Data: []byte("<html><body>Test App</body></html>"),
						Mode: 0o644,
					},
				}
			},
			expectedStatus:      http.StatusOK,
			expectedBody:        "<html><body>Test App</body></html>",
			expectedContentType: "text/html; charset=utf-8",
		},
		{
			name:       "🔴returns error when index.html not found",
			requestURL: "/wallet-registration/start?token=test-token",
			ctx:        context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, validClaims),
			setupFS: func() fs.FS {
				return fstest.MapFS{
					"other.html": &fstest.MapFile{
						Data: []byte("<html><body>Other content</body></html>"),
						Mode: 0o644,
					},
				}
			},
			expectedStatus:      http.StatusInternalServerError,
			expectedBody:        "Could not render Registration Page",
			expectedContentType: "application/json; charset=utf-8",
		},
		{
			name:                "🔴returns 401 unauthorized if the token is not in the url",
			requestURL:          "/wallet-registration/start",
			ctx:                 context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, validClaims),
			setupFS:             func() fs.FS { return fstest.MapFS{} },
			expectedStatus:      http.StatusUnauthorized,
			expectedBody:        "Not authorized.",
			expectedContentType: "application/json; charset=utf-8",
		},
		{
			name:                "🔴returns 401 unauthorized if the sep24 claims are not in the request context",
			requestURL:          "/wallet-registration/start?token=test-token",
			ctx:                 ctx,
			setupFS:             func() fs.FS { return fstest.MapFS{} },
			expectedStatus:      http.StatusUnauthorized,
			expectedBody:        "Not authorized.",
			expectedContentType: "application/json; charset=utf-8",
		},
		{
			name:                "🔴returns 401 unauthorized if the token is in the request context but it's not valid",
			requestURL:          "/wallet-registration/start?token=test-token",
			ctx:                 context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, &anchorplatform.SEP24JWTClaims{}),
			setupFS:             func() fs.FS { return fstest.MapFS{} },
			expectedStatus:      http.StatusUnauthorized,
			expectedBody:        "Not authorized.",
			expectedContentType: "application/json; charset=utf-8",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			reqURL, err := url.Parse(tc.requestURL)
			require.NoError(t, err)

			serveReactApp(tc.ctx, reqURL, w, tc.setupFS())

			resp := w.Result()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.Contains(t, string(body), tc.expectedBody)

			if tc.expectedContentType != "" {
				assert.Equal(t, tc.expectedContentType, resp.Header.Get("Content-Type"))
			}
		})
	}
}

func Test_SEP24InteractiveDepositHandler_ServeApp(t *testing.T) {
	validClaims := &anchorplatform.SEP24JWTClaims{
		ClientDomainClaim: "test.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}
	ctx := context.Background()
	ctxWithClaims := context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, validClaims)

	mockFS := createMockFS(t, map[string]string{
		"app/dist/index.html":     "<html><body>SPA content</body></html>",
		"app/dist/assets/main.js": "console.log('Hello');",
		"app/dist/favicon.ico":    "icon data",
	})

	testCases := []struct {
		name             string
		path             string
		expectedStatus   int
		expectedContains string
		handler          SEP24InteractiveDepositHandler
	}{
		{
			name:             "🟢serves SPA for app route",
			path:             "/wallet-registration/start?token=test-token",
			expectedStatus:   http.StatusOK,
			expectedContains: "<html><body>SPA content</body></html>",
			handler: SEP24InteractiveDepositHandler{
				App:      mockFS,
				BasePath: "app/dist",
			},
		},
		{
			name:             "🔴401 serving SPA when no token is provided",
			path:             "/wallet-registration/start",
			expectedStatus:   http.StatusUnauthorized,
			expectedContains: "Not authorized",
			handler: SEP24InteractiveDepositHandler{
				App:      mockFS,
				BasePath: "app/dist",
			},
		},
		{
			name:             "🟢serves static asset",
			path:             "/wallet-registration/assets/main.js",
			expectedStatus:   http.StatusOK,
			expectedContains: "console.log('Hello');",
			handler: SEP24InteractiveDepositHandler{
				App:      mockFS,
				BasePath: "app/dist",
			},
		},
		{
			name:             "🟢serves static asset directly at root",
			path:             "/wallet-registration/favicon.ico",
			expectedStatus:   http.StatusOK,
			expectedContains: "icon data",
			handler: SEP24InteractiveDepositHandler{
				App:      mockFS,
				BasePath: "app/dist",
			},
		},
		{
			name:             "🟠returns 404 for directory listing",
			path:             "/wallet-registration/assets/",
			expectedStatus:   http.StatusNotFound,
			expectedContains: "404 page not found",
			handler: SEP24InteractiveDepositHandler{
				App:      mockFS,
				BasePath: "app/dist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Get("/wallet-registration/*", tc.handler.ServeApp)

			req, err := http.NewRequestWithContext(ctxWithClaims, http.MethodGet, tc.path, nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			body, err := io.ReadAll(rr.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Contains(t, string(body), tc.expectedContains)
		})
	}
}

func createMockFS(t *testing.T, files map[string]string) fstest.MapFS {
	t.Helper()

	mockFS := make(fstest.MapFS)
	for path, content := range files {
		mockFS[path] = &fstest.MapFile{
			Data: []byte(content),
		}
	}
	return mockFS
}
