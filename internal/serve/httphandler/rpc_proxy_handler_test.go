package httphandler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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
