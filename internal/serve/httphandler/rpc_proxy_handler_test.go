package httphandler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RPCProxyHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		name               string
		rpcURL             string
		rpcAuthHeaderKey   string
		rpcAuthHeaderValue string
		requestMethod      string
		requestBody        string
		setupMockRPC       func(t *testing.T) *httptest.Server
		expectedStatus     int
		expectedBodyMatch  string
	}{
		{
			name:              "returns error when RPC URL not configured",
			rpcURL:            "",
			requestMethod:     http.MethodPost,
			requestBody:       `{"jsonrpc":"2.0","method":"getHealth","id":1}`,
			setupMockRPC:      nil,
			expectedStatus:    http.StatusInternalServerError,
			expectedBodyMatch: "",
		},
		{
			name:              "returns error for non-POST requests",
			rpcURL:            "https://rpc.example.com",
			requestMethod:     http.MethodGet,
			requestBody:       "",
			setupMockRPC:      nil,
			expectedStatus:    http.StatusBadRequest,
			expectedBodyMatch: "",
		},
		{
			name:              "returns error for empty request body",
			rpcURL:            "https://rpc.example.com",
			requestMethod:     http.MethodPost,
			requestBody:       "",
			setupMockRPC:      nil,
			expectedStatus:    http.StatusBadRequest,
			expectedBodyMatch: "",
		},
		{
			name:              "returns error when request body exceeds max size",
			rpcURL:            "https://rpc.example.com",
			requestMethod:     http.MethodPost,
			requestBody:       strings.Repeat("x", MaxRPCRequestBodySize+1),
			setupMockRPC:      nil,
			expectedStatus:    http.StatusBadRequest,
			expectedBodyMatch: "request body too large or unreadable",
		},
		{
			name:          "accepts request body at the max size limit",
			requestMethod: http.MethodPost,
			requestBody:   `{"jsonrpc":"2.0","method":"getHealth","id":1,"params":"` + strings.Repeat("x", MaxRPCRequestBodySize-57) + `"}`,
			setupMockRPC: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Equal(t, MaxRPCRequestBodySize, len(body))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, err = w.Write([]byte(`{"jsonrpc":"2.0","result":{"status":"healthy"},"id":1}`))
					require.NoError(t, err)
				}))
			},
			expectedStatus:    http.StatusOK,
			expectedBodyMatch: "healthy",
		},
		{
			name:          "proxies request to RPC without auth headers",
			requestMethod: http.MethodPost,
			requestBody:   `{"jsonrpc":"2.0","method":"getHealth","id":1}`,
			setupMockRPC: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
					assert.Equal(t, "application/json", r.Header.Get("Accept"))
					assert.Empty(t, r.Header.Get("X-API-Key"))

					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(body), "getHealth")

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, err = w.Write([]byte(`{"jsonrpc":"2.0","result":{"status":"healthy"},"id":1}`))
					require.NoError(t, err)
				}))
			},
			expectedStatus:    http.StatusOK,
			expectedBodyMatch: "healthy",
		},
		{
			name:               "proxies request to RPC with auth headers",
			rpcAuthHeaderKey:   "X-API-Key",
			rpcAuthHeaderValue: "test-token-123",
			requestMethod:      http.MethodPost,
			requestBody:        `{"jsonrpc":"2.0","method":"simulateTransaction","params":{"transaction":"AAAAAgAAAAA..."},"id":1}`,
			setupMockRPC: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
					assert.Equal(t, "application/json", r.Header.Get("Accept"))
					assert.Equal(t, "test-token-123", r.Header.Get("X-API-Key"))

					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(body), "simulateTransaction")

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, err = w.Write([]byte(`{"jsonrpc":"2.0","result":{"transactionData":"AAAA...","minResourceFee":"100"},"id":1}`))
					require.NoError(t, err)
				}))
			},
			expectedStatus:    http.StatusOK,
			expectedBodyMatch: "transactionData",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var mockRPC *httptest.Server
			if tc.setupMockRPC != nil {
				mockRPC = tc.setupMockRPC(t)
				defer mockRPC.Close()
				tc.rpcURL = mockRPC.URL
			}

			handler := RPCProxyHandler{
				RPCUrl:             tc.rpcURL,
				RPCAuthHeaderKey:   tc.rpcAuthHeaderKey,
				RPCAuthHeaderValue: tc.rpcAuthHeaderValue,
			}

			req := httptest.NewRequest(tc.requestMethod, "/rpc", bytes.NewBufferString(tc.requestBody))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			if tc.expectedBodyMatch != "" {
				assert.Contains(t, rr.Body.String(), tc.expectedBodyMatch)
			}
		})
	}
}

func Test_RPCProxyHandler_ServeHTTP_forwardsOnlyAllowListedRequestData(t *testing.T) {
	const requestBody = `{"jsonrpc":"2.0","method":"getHealth","id":1}`

	// Headers every proxied request carries. Accept-Encoding and Content-Length are set by Go's HTTP transport.
	baseHeaders := func() http.Header {
		h := http.Header{}
		h.Set("Content-Type", "application/json")
		h.Set("Accept", "application/json")
		h.Set("User-Agent", rpcProxyUserAgent)
		h.Set("Accept-Encoding", "gzip")
		h.Set("Content-Length", strconv.Itoa(len(requestBody)))
		return h
	}
	withHeader := func(h http.Header, key, value string) http.Header {
		h.Set(key, value)
		return h
	}

	testCases := []struct {
		name               string
		rpcPathAndQuery    string
		rpcAuthHeaderKey   string
		rpcAuthHeaderValue string
		wantPath           string
		wantRawQuery       string
		wantHeaders        http.Header
	}{
		{
			name:        "without RPC auth header",
			wantPath:    "/",
			wantHeaders: baseHeaders(),
		},
		{
			name:               "with RPC auth header",
			rpcAuthHeaderKey:   "X-API-Key",
			rpcAuthHeaderValue: "operator-rpc-key",
			wantPath:           "/",
			wantHeaders:        withHeader(baseHeaders(), "X-API-Key", "operator-rpc-key"),
		},
		{
			name:            "with path and query in the RPC URL",
			rpcPathAndQuery: "/soroban/rpc?apikey=operator-url-key",
			wantPath:        "/soroban/rpc",
			wantRawQuery:    "apikey=operator-url-key",
			wantHeaders:     baseHeaders(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var gotHeaders http.Header
			var gotPath, gotRawQuery, gotBody string
			mockRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeaders = r.Header.Clone()
				gotPath = r.URL.Path
				gotRawQuery = r.URL.RawQuery
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				gotBody = string(body)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err = w.Write([]byte(`{"jsonrpc":"2.0","result":{"status":"healthy"},"id":1}`))
				require.NoError(t, err)
			}))
			defer mockRPC.Close()

			handler := RPCProxyHandler{
				RPCUrl:             mockRPC.URL + tc.rpcPathAndQuery,
				RPCAuthHeaderKey:   tc.rpcAuthHeaderKey,
				RPCAuthHeaderValue: tc.rpcAuthHeaderValue,
			}

			req := httptest.NewRequest(http.MethodPost, "/rpc/user?debug=1&apikey=caller-key", strings.NewReader(requestBody))
			req.Header.Set("Authorization", "Bearer caller-session-jwt")
			req.Header.Set("SDP-Tenant-Name", "caller-tenant")
			req.Header.Set("Cookie", "session=caller-cookie")
			req.Header.Set("X-Forwarded-For", "203.0.113.77")
			req.Header.Set("Forwarded", "for=203.0.113.77")
			req.Header.Set("X-Real-IP", "203.0.113.77")
			req.Header.Set("Origin", "https://caller-tenant.sdp.example.org")
			req.Header.Set("Referer", "https://caller-tenant.sdp.example.org/receivers/123")
			req.Header.Set("User-Agent", "Mozilla/5.0 (caller browser)")
			req.Header.Set("Accept-Encoding", "br")
			req.Header.Set("Accept-Language", "el-GR")
			req.Header.Set("Content-Type", "text/plain")
			req.Header.Set("X-Client-Name", "js-stellar-sdk")
			req.Header.Set("X-API-Key", "caller-supplied-key")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Contains(t, rr.Body.String(), "healthy")
			assert.Equal(t, tc.wantHeaders, gotHeaders)
			assert.Equal(t, tc.wantPath, gotPath)
			assert.Equal(t, tc.wantRawQuery, gotRawQuery)
			assert.Equal(t, requestBody, gotBody)
		})
	}
}
