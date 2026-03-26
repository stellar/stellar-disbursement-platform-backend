package sepauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
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

	validSEP10Token, tokenErr := jwtManager.GenerateSEP10Token(
		"https://home.example.com/sep10/auth",
		"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
		"sep10-jti",
		"client.example.com",
		"home.example.com",
		now,
		now.Add(time.Hour),
	)
	require.NoError(t, tokenErr)

	validSEP45Token, tokenErr := jwtManager.GenerateSEP45Token(
		"https://home.example.com/sep45/auth",
		"CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
		"sep45-jti",
		"client.example.com",
		"home.example.com",
		now,
		now.Add(time.Hour),
	)
	require.NoError(t, tokenErr)

	expiredSEP10Claims := &Sep10JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://home.example.com/sep10/auth",
			Subject:   "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			ID:        "sep10-jti-expired",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
		},
	}
	expiredSEP10Token, tokenErr := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredSEP10Claims).SignedString(jwtManager.secret)
	require.NoError(t, tokenErr)

	expiredSEP45Claims := &Sep45JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://home.example.com/sep45/auth",
			Subject:   "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
			ID:        "sep45-jti-expired",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
		},
	}
	expiredSEP45Token, tokenErr := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredSEP45Claims).SignedString(jwtManager.secret)
	require.NoError(t, tokenErr)

	wrongSecretSEP10Claims := &Sep10JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://home.example.com/sep10/auth",
			Subject:   "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			ID:        "sep10-jti-wrong-secret",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	wrongSecretSEP10Token, tokenErr := jwt.NewWithClaims(jwt.SigningMethodHS256, wrongSecretSEP10Claims).SignedString([]byte("wrong-secret-key"))
	require.NoError(t, tokenErr)

	wrongSecretSEP45Claims := &Sep45JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://home.example.com/sep45/auth",
			Subject:   "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
			ID:        "sep45-jti-wrong-secret",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	wrongSecretSEP45Token, tokenErr := jwt.NewWithClaims(jwt.SigningMethodHS256, wrongSecretSEP45Claims).SignedString([]byte("wrong-secret-key"))
	require.NoError(t, tokenErr)

	tests := []struct {
		name               string
		authHeader         string
		expectedStatusCode int
		expectClaims       *WebAuthClaims
		expectedError      string
	}{
		{
			name:               "missing authorization header",
			authHeader:         "",
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Missing authorization header",
		},
		{
			name:               "invalid authorization header format",
			authHeader:         "InvalidToken",
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "Invalid authorization header",
		},
		{
			name:               "invalid authorization header format (lowercase bearer)",
			authHeader:         "bearer " + validSEP10Token,
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "Invalid authorization header",
		},
		{
			name:               "empty token after Bearer",
			authHeader:         "Bearer ",
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Invalid token",
		},
		{
			name:               "invalid token",
			authHeader:         "Bearer not.a.valid.token",
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Invalid token",
		},
		{
			name:               "expired SEP-10 token",
			authHeader:         "Bearer " + expiredSEP10Token,
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Expired token",
		},
		{
			name:               "expired SEP-45 token",
			authHeader:         "Bearer " + expiredSEP45Token,
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Expired token",
		},
		{
			name:               "SEP-10 token with wrong secret",
			authHeader:         "Bearer " + wrongSecretSEP10Token,
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Invalid token",
		},
		{
			name:               "SEP-45 token with wrong secret",
			authHeader:         "Bearer " + wrongSecretSEP45Token,
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "Invalid token",
		},
		{
			name:               "valid SEP-10 token",
			authHeader:         "Bearer " + validSEP10Token,
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
			authHeader:         "Bearer " + validSEP45Token,
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
				_, writeErr := w.Write([]byte("success"))
				require.NoError(t, writeErr)
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
				if tt.expectedError != "" {
					assert.Contains(t, rec.Body.String(), tt.expectedError)
				}
			} else {
				require.NotNil(t, claimsFromContext)
				assert.Equal(t, tt.expectClaims, claimsFromContext)
				assert.Equal(t, "success", rec.Body.String())
			}
		})
	}
}
