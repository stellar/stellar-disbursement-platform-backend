package sepauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetWebAuthClaims(t *testing.T) {
	t.Run("returns nil when no claims in context", func(t *testing.T) {
		assert.Nil(t, GetWebAuthClaims(context.Background()))
	})

	t.Run("returns claims when present in context", func(t *testing.T) {
		expected := &WebAuthClaims{
			Subject:      "subject",
			ClientDomain: "client.example.com",
			HomeDomain:   "home.example.com",
			TokenType:    "sep10",
		}

		ctx := context.WithValue(context.Background(), WebAuthClaimsContextKey, expected)
		got := GetWebAuthClaims(ctx)

		require.NotNil(t, got)
		assert.Equal(t, expected, got)
	})
}

func Test_WebAuthHeaderTokenAuthenticateMiddleware(t *testing.T) {
	jwtManager, err := NewJWTManager("test-secret-key-123", 5000)
	require.NoError(t, err)

	now := time.Now()

	sep10Token, sep10Err := jwtManager.GenerateSEP10Token(
		"https://home.example.com/sep10/auth",
		"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
		"sep10-jti",
		"client.example.com",
		"home.example.com",
		now,
		now.Add(time.Hour),
	)
	require.NoError(t, sep10Err)

	sep45Token, sep45Err := jwtManager.GenerateSEP45Token(
		"https://home.example.com/sep45/auth",
		"CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
		"sep45-jti",
		"client.example.com",
		"home.example.com",
		now,
		now.Add(time.Hour),
	)
	require.NoError(t, sep45Err)

	tests := []struct {
		name               string
		authHeader         string
		expectedStatusCode int
		expectClaims       *WebAuthClaims
	}{
		{
			name:               "missing authorization header",
			authHeader:         "",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "invalid authorization header format",
			authHeader:         "InvalidToken",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "invalid token",
			authHeader:         "Bearer not.a.valid.token",
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "valid SEP-10 token",
			authHeader:         "Bearer " + sep10Token,
			expectedStatusCode: http.StatusOK,
			expectClaims: &WebAuthClaims{
				Subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
				ClientDomain: "client.example.com",
				HomeDomain:   "home.example.com",
				TokenType:    WebAuthTokenTypeSEP10,
			},
		},
		{
			name:               "valid SEP-45 token",
			authHeader:         "Bearer " + sep45Token,
			expectedStatusCode: http.StatusOK,
			expectClaims: &WebAuthClaims{
				Subject:      "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
				ClientDomain: "client.example.com",
				HomeDomain:   "home.example.com",
				TokenType:    WebAuthTokenTypeSEP45,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var claimsFromContext *WebAuthClaims

			m := WebAuthHeaderTokenAuthenticateMiddleware(jwtManager)
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claimsFromContext = GetWebAuthClaims(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			handler := m(next)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatusCode, rec.Code)
			if tt.expectClaims == nil {
				assert.Nil(t, claimsFromContext)
			} else {
				require.NotNil(t, claimsFromContext)
				assert.Equal(t, tt.expectClaims, claimsFromContext)
			}
		})
	}
}
