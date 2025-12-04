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

func Test_GetSEP10Claims(t *testing.T) {
	t.Run("returns nil when no claims in context", func(t *testing.T) {
		ctx := context.Background()
		claims := GetSEP10Claims(ctx)
		assert.Nil(t, claims)
	})

	t.Run("returns claims when present in context", func(t *testing.T) {
		expectedClaims := &Sep10JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "EMPEROR_PROTECTS_123",
				Issuer:  "terra-holy-throne",
			},
			ClientDomain: "test.com",
			HomeDomain:   "home.com",
		}

		ctx := context.WithValue(context.Background(), SEP10ClaimsContextKey, expectedClaims)
		claims := GetSEP10Claims(ctx)

		require.NotNil(t, claims)
		assert.Equal(t, expectedClaims.Subject, claims.Subject)
		assert.Equal(t, expectedClaims.Issuer, claims.Issuer)
		assert.Equal(t, expectedClaims.ClientDomain, claims.ClientDomain)
		assert.Equal(t, expectedClaims.HomeDomain, claims.HomeDomain)
	})
}

func Test_SEP10HeaderTokenAuthenticateMiddleware(t *testing.T) {
	jwtManager, err := NewJWTManager("test-secret-key-123", 5000)
	require.NoError(t, err)

	createValidToken := func() string {
		now := time.Now()
		claims := &Sep10JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "PRIMARCH_GUILLIMAN",
				Issuer:    "imperium-of-man",
				ID:        "codex-astartes-777",
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			},
			ClientDomain: "client.example.com",
			HomeDomain:   "home.example.com",
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString(jwtManager.secret)
		require.NoError(t, err)
		return tokenStr
	}

	createExpiredToken := func() string {
		now := time.Now()
		claims := &Sep10JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "PRIMARCH_GUILLIMAN",
				Issuer:    "imperium-of-man",
				ID:        "codex-astartes-777",
				IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour)),
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString(jwtManager.secret)
		require.NoError(t, err)
		return tokenStr
	}

	createTokenWithWrongSecret := func() string {
		now := time.Now()
		claims := &Sep10JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "PRIMARCH_GUILLIMAN",
				Issuer:    "imperium-of-man",
				ID:        "codex-astartes-777",
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte("wrong-secret-key"))
		require.NoError(t, err)
		return tokenStr
	}

	testCases := []struct {
		name               string
		authHeader         string
		expectedStatusCode int
		expectClaimsInCtx  bool
		expectedError      string
	}{
		{
			name:               "missing authorization header",
			authHeader:         "",
			expectedStatusCode: http.StatusUnauthorized,
			expectClaimsInCtx:  false,
			expectedError:      "Missing authorization header",
		},
		{
			name:               "invalid authorization header format (no Bearer prefix)",
			authHeader:         "InvalidToken123",
			expectedStatusCode: http.StatusBadRequest,
			expectClaimsInCtx:  false,
			expectedError:      "Invalid authorization header",
		},
		{
			name:               "invalid authorization header format (lowercase bearer)",
			authHeader:         "bearer " + createValidToken(),
			expectedStatusCode: http.StatusBadRequest,
			expectClaimsInCtx:  false,
			expectedError:      "Invalid authorization header",
		},
		{
			name:               "empty token after Bearer",
			authHeader:         "Bearer ",
			expectedStatusCode: http.StatusUnauthorized,
			expectClaimsInCtx:  false,
			expectedError:      "Invalid token",
		},
		{
			name:               "malformed JWT token",
			authHeader:         "Bearer not.a.valid.jwt",
			expectedStatusCode: http.StatusUnauthorized,
			expectClaimsInCtx:  false,
			expectedError:      "Invalid token",
		},
		{
			name:               "expired token",
			authHeader:         "Bearer " + createExpiredToken(),
			expectedStatusCode: http.StatusUnauthorized,
			expectClaimsInCtx:  false,
			expectedError:      "Expired token",
		},
		{
			name:               "token with wrong secret",
			authHeader:         "Bearer " + createTokenWithWrongSecret(),
			expectedStatusCode: http.StatusUnauthorized,
			expectClaimsInCtx:  false,
			expectedError:      "Invalid token",
		},
		{
			name:               "valid token",
			authHeader:         "Bearer " + createValidToken(),
			expectedStatusCode: http.StatusOK,
			expectClaimsInCtx:  true,
			expectedError:      "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			middleware := SEP10HeaderTokenAuthenticateMiddleware(jwtManager)

			var claimsFromContext *Sep10JWTClaims
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claimsFromContext = GetSEP10Claims(r.Context())
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("success"))
				require.NoError(t, err)
			})

			handler := middleware(nextHandler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)

			if tc.expectClaimsInCtx {
				assert.NotNil(t, claimsFromContext)
				assert.Equal(t, "PRIMARCH_GUILLIMAN", claimsFromContext.Subject)
				assert.Equal(t, "imperium-of-man", claimsFromContext.Issuer)
				assert.Equal(t, "success", rec.Body.String())
			} else {
				assert.Nil(t, claimsFromContext)
				if tc.expectedError != "" {
					assert.Contains(t, rec.Body.String(), tc.expectedError)
				}
			}
		})
	}
}

func Test_SEP10HeaderTokenAuthenticateMiddleware_Integration(t *testing.T) {
	jwtManager, err := NewJWTManager("test-secret-key-123", 5000)
	require.NoError(t, err)

	t.Run("middleware correctly passes claims to nested handlers", func(t *testing.T) {
		now := time.Now()
		expectedClaims := &Sep10JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "SPACE_MARINE_ULTRAMAR",
				Issuer:    "adeptus-astartes",
				ID:        "battle-barge-42",
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			},
			ClientDomain: "client.example.com",
			HomeDomain:   "home.example.com",
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, expectedClaims)
		tokenStr, err := token.SignedString(jwtManager.secret)
		require.NoError(t, err)

		middleware := SEP10HeaderTokenAuthenticateMiddleware(jwtManager)

		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetSEP10Claims(r.Context())
			require.NotNil(t, claims)

			w.Header().Set("X-Subject", claims.Subject)
			w.Header().Set("X-ClientDomain", claims.ClientDomain)
			w.WriteHeader(http.StatusOK)
		})

		handler := middleware(innerHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "SPACE_MARINE_ULTRAMAR", rec.Header().Get("X-Subject"))
		assert.Equal(t, "client.example.com", rec.Header().Get("X-ClientDomain"))
	})

	t.Run("multiple requests with different tokens", func(t *testing.T) {
		middleware := SEP10HeaderTokenAuthenticateMiddleware(jwtManager)

		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetSEP10Claims(r.Context())
			if claims != nil {
				_, err := w.Write([]byte(claims.Subject))
				require.NoError(t, err)
			}
		}))

		now := time.Now()
		claims1 := &Sep10JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "INQUISITOR_EISENHORN",
				Issuer:    "ordo-xenos",
				ID:        "exterminatus-auth-1",
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			},
		}
		token1 := jwt.NewWithClaims(jwt.SigningMethodHS256, claims1)
		tokenStr1, err := token1.SignedString(jwtManager.secret)
		require.NoError(t, err)

		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req1.Header.Set("Authorization", "Bearer "+tokenStr1)
		rec1 := httptest.NewRecorder()
		handler.ServeHTTP(rec1, req1)

		assert.Equal(t, http.StatusOK, rec1.Code)
		assert.Equal(t, "INQUISITOR_EISENHORN", rec1.Body.String())

		claims2 := &Sep10JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "COMMISSAR_GAUNT",
				Issuer:    "tanith-first",
				ID:        "ghost-regiment-2",
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			},
		}
		token2 := jwt.NewWithClaims(jwt.SigningMethodHS256, claims2)
		tokenStr2, err := token2.SignedString(jwtManager.secret)
		require.NoError(t, err)

		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.Header.Set("Authorization", "Bearer "+tokenStr2)
		rec2 := httptest.NewRecorder()
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, http.StatusOK, rec2.Code)
		assert.Equal(t, "COMMISSAR_GAUNT", rec2.Body.String())
	})
}
