package httphandler

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	testCases := []struct {
		name           string
		setupFS        func() fs.FS
		expectedStatus int
		expectedBody   string
		checkHeader    bool
	}{
		{
			name: "🟢successfully serves index.html",
			setupFS: func() fs.FS {
				return fstest.MapFS{
					"index.html": &fstest.MapFile{
						Data: []byte("<html><body>Test App</body></html>"),
						Mode: 0o644,
					},
				}
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "<html><body>Test App</body></html>",
			checkHeader:    true,
		},
		{
			name: "🔴returns error when index.html not found",
			setupFS: func() fs.FS {
				return fstest.MapFS{
					"other.html": &fstest.MapFile{
						Data: []byte("<html><body>Other content</body></html>"),
						Mode: 0o644,
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Could not render Registration Page",
			checkHeader:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx := context.Background()
			serveReactApp(ctx, w, tc.setupFS())

			resp := w.Result()
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), tc.expectedBody)

			if tc.checkHeader {
				assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
			}
		})
	}
}

func TestSEP24InteractiveDepositHandler_ServeApp(t *testing.T) {
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
			name:             "🟢serves SPA for root path",
			path:             "/wallet-registration/",
			expectedStatus:   http.StatusOK,
			expectedContains: "<html><body>SPA content</body></html>",
			handler: SEP24InteractiveDepositHandler{
				App:      mockFS,
				BasePath: "app/dist",
			},
		},
		{
			name:             "🟢serves SPA for app route",
			path:             "/wallet-registration/start",
			expectedStatus:   http.StatusOK,
			expectedContains: "<html><body>SPA content</body></html>",
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

			req, err := http.NewRequest(http.MethodGet, tc.path, nil)
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
