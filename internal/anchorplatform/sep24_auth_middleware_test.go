package anchorplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/require"
)

func Test_GetSEP24Claims(t *testing.T) {
	ctx := context.Background()
	gotClaims := GetSEP24Claims(ctx)
	require.Nil(t, gotClaims)

	wantClaims := &SEP24JWTClaims{
		ClientDomainClaim: "test.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444:123456",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Second)),
		},
	}
	ctx = context.WithValue(ctx, SEP24ClaimsContextKey, wantClaims)

	gotClaims = GetSEP24Claims(ctx)
	require.Equal(t, wantClaims, gotClaims)
}

func Test_SEP24UnauthenticatedRoutes(t *testing.T) {
	r := chi.NewRouter()

	r.Get("/unauthenticated", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
		require.NoError(t, err)
	})

	t.Run("doesn't return Unauthorized for unauthenticated routes", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/unauthenticated", nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.JSONEq(t, `{"status":"ok"}`, string(respBody))
	})
}

func Test_SEP24QueryTokenAuthenticateMiddleware(t *testing.T) {
	tokenSecret := "jwt_secret_1234567890"
	r := chi.NewRouter()
	jwtManager, err := NewJWTManager(tokenSecret, 5000)
	require.NoError(t, err)

	r.Group(func(r chi.Router) {
		r.Use(SEP24QueryTokenAuthenticateMiddleware(jwtManager, network.TestNetworkPassphrase))

		r.Get("/authenticated", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})
	})

	t.Run("returns Unauthorized for authenticated routes without token", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "no token was provided in the request")
	})

	t.Run("returns Unauthorized if the jwt could not be parsed", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodGet, "/authenticated?token=123", nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: token contains an invalid number of segments")
	})

	t.Run("returns Unauthorized if the jwt is expired", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		expiredToken := "eyJjbGllbnRfZG9tYWluIjoidGVzdC5jb20iLCJzdWIiOiJHQkxUWEY0NkpUQ0dNV0ZKQVNRTFZYTU1BMzZJUFlURENONEVONzNIUlhDR0RDR1lCWk0zQTQ0NCIsImV4cCI6MTY4MTQxMDkzMiwianRpIjoidGVzdC10cmFuc2FjdGlvbi1pZCJ9.RThqCuWkjBr1xw8LOBogDmw8RyMnrELDkA-w4Jv5x_E"
		req, err := http.NewRequest(http.MethodGet, "/authenticated?token="+expiredToken, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: token contains an invalid number of segments")
	})

	t.Run("returns Unauthorized if the token is valid but the transaction_id is not different from what's expected", func(t *testing.T) {
		validToken, err := jwtManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "test.com", "test-transaction-id")
		require.NoError(t, err)

		urlStr := fmt.Sprintf("/authenticated?transaction_id=%s&token=%s", "invalid-transaction-id", validToken)
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.JSONEq(t, `{"error":"The request was invalid in some way."}`, string(respBody))
	})

	t.Run("returns Unauthorized if the jwt expiration is good but another parameter (stellar account) is weird", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// create a token with an odd subject (stellar_account:memo)
		badClaims := SEP24JWTClaims{
			ClientDomainClaim: "test.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "bad-subject",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Second)),
			},
		}
		tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, badClaims)
		badToken, err := tokenObj.SignedString([]byte(tokenSecret))
		require.NoError(t, err)

		urlStr := fmt.Sprintf("/authenticated?transaction_id=%s&token=%s", "test-transaction-id", badToken)
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: stellar account is invalid: non-canonical strkey; unused leftover character")
	})

	t.Run("returns Unauthorized if the jwt was signed with a different secret", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// create a token with an odd subject (stellar_account:memo)
		anotherTokenSecret := tokenSecret + "another"
		anotherJWTManager, err := NewJWTManager(anotherTokenSecret, 5000)
		require.NoError(t, err)
		tokenWithDifferentSigner, err := anotherJWTManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "test.com", "valid-transaction-id")
		require.NoError(t, err)

		urlStr := fmt.Sprintf("/authenticated?transaction_id=%s&token=%s", "valid-transaction-id", tokenWithDifferentSigner)
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: signature is invalid")
	})

	t.Run("both the token and the transaction_id are valid 🎉", func(t *testing.T) {
		var contextClaims *SEP24JWTClaims
		require.Nil(t, contextClaims)
		r.With(SEP24QueryTokenAuthenticateMiddleware(jwtManager, network.TestNetworkPassphrase)).Get("/authenticated_success", func(w http.ResponseWriter, r *http.Request) {
			contextClaims = r.Context().Value(SEP24ClaimsContextKey).(*SEP24JWTClaims)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})

		now := time.Now()
		validTransactionID := "valid-transaction-id"
		validToken, err := jwtManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "test.com", validTransactionID)
		require.NoError(t, err)

		urlStr := fmt.Sprintf("/authenticated_success?transaction_id=%s&token=%s", validTransactionID, validToken)
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.JSONEq(t, `{"status":"ok"}`, string(respBody))

		// validate the context claims
		require.NotNil(t, contextClaims)
		require.Equal(t, "test.com", contextClaims.ClientDomain())
		require.Equal(t, "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", contextClaims.SEP10StellarAccount())
		require.Equal(t, validTransactionID, contextClaims.TransactionID())
		require.Empty(t, contextClaims.SEP10StellarMemo())
		require.True(t, contextClaims.ExpiresAt().After(now.Add(time.Duration(4000*time.Millisecond))))
		require.True(t, contextClaims.ExpiresAt().Before(now.Add(time.Duration(5000*time.Millisecond))))
	})

	t.Run("token with empty client domain but valid in testnet 🎉", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.DefaultLogger.SetLevel(log.WarnLevel)

		var contextClaims *SEP24JWTClaims
		require.Nil(t, contextClaims)
		r.With(SEP24QueryTokenAuthenticateMiddleware(jwtManager, network.TestNetworkPassphrase)).Get("/authenticated_testnet", func(w http.ResponseWriter, r *http.Request) {
			contextClaims = r.Context().Value(SEP24ClaimsContextKey).(*SEP24JWTClaims)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})

		validTransactionID := "valid-transaction-id"
		validToken, err := jwtManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "", validTransactionID)
		require.NoError(t, err)

		urlStr := fmt.Sprintf("/authenticated_testnet?transaction_id=%s&token=%s", validTransactionID, validToken)
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.JSONEq(t, `{"status":"ok"}`, string(respBody))

		// check client domain
		require.Empty(t, contextClaims.ClientDomain())

		// validate logs
		require.Contains(t, buf.String(), "missing client domain in the token claims")
	})

	t.Run("token with empty client domain returns error in pubnet 🎉", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		r.With(SEP24QueryTokenAuthenticateMiddleware(jwtManager, network.PublicNetworkPassphrase)).Get("/authenticated_pubnet", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})

		validTransactionID := "valid-transaction-id"
		validToken, err := jwtManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "", validTransactionID)
		require.NoError(t, err)

		urlStr := fmt.Sprintf("/authenticated_pubnet?transaction_id=%s&token=%s", validTransactionID, validToken)
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.JSONEq(t, `{"error":"The request was invalid in some way."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "missing client domain in the token claims")
	})
}

func Test_SEP24HeaderTokenAuthenticateMiddleware(t *testing.T) {
	tokenSecret := "jwt_secret_1234567890"
	r := chi.NewRouter()
	jwtManager, err := NewJWTManager(tokenSecret, 5000)
	require.NoError(t, err)

	r.Group(func(r chi.Router) {
		r.Use(SEP24HeaderTokenAuthenticateMiddleware(jwtManager, network.TestNetworkPassphrase))

		r.Get("/authenticated", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})
	})

	t.Run("returns Unauthorized for authenticated routes without token", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "no token was provided in the Authorization header")
	})

	t.Run("returns Unauthorized if the authorization header is invalid", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "InvalidToken")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "invalid Authorization header provided")
	})

	t.Run("returns Unauthorized if the jwt could not be parsed", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer 123")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: token contains an invalid number of segments")
	})

	t.Run("returns Unauthorized if the jwt is expired", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)
		expiredToken := "eyJjbGllbnRfZG9tYWluIjoidGVzdC5jb20iLCJzdWIiOiJHQkxUWEY0NkpUQ0dNV0ZKQVNRTFZYTU1BMzZJUFlURENONEVONzNIUlhDR0RDR1lCWk0zQTQ0NCIsImV4cCI6MTY4MTQxMDkzMiwianRpIjoidGVzdC10cmFuc2FjdGlvbi1pZCJ9.RThqCuWkjBr1xw8LOBogDmw8RyMnrELDkA-w4Jv5x_E"
		authHeader := "Bearer " + expiredToken
		req.Header.Set("Authorization", authHeader)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: token contains an invalid number of segments")
	})

	t.Run("returns Unauthorized if the jwt expiration is good but another parameter (stellar account) is weird", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// create a token with an odd subject (stellar_account:memo)
		badClaims := SEP24JWTClaims{
			ClientDomainClaim: "test.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "bad-subject",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Second)),
			},
		}
		tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, badClaims)
		badToken, err := tokenObj.SignedString([]byte(tokenSecret))
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)
		authHeader := "Bearer " + badToken
		req.Header.Set("Authorization", authHeader)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: stellar account is invalid: non-canonical strkey; unused leftover character")
	})

	t.Run("returns Unauthorized if the jwt was signed with a different secret", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// create a token signed with a different secret
		anotherTokenSecret := tokenSecret + "another"
		anotherJWTManager, err := NewJWTManager(anotherTokenSecret, 5000)
		require.NoError(t, err)
		tokenWithDifferentSigner, err := anotherJWTManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "test.com", "valid-transaction-id")
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, "/authenticated", nil)
		require.NoError(t, err)
		authHeader := "Bearer " + tokenWithDifferentSigner
		req.Header.Set("Authorization", authHeader)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "parsing the token claims: parsing SEP24 token: signature is invalid")
	})

	t.Run("token is valid 🎉", func(t *testing.T) {
		var contextClaims *SEP24JWTClaims
		require.Nil(t, contextClaims)
		r.With(SEP24HeaderTokenAuthenticateMiddleware(jwtManager, network.TestNetworkPassphrase)).Get("/authenticated_success", func(w http.ResponseWriter, r *http.Request) {
			contextClaims = r.Context().Value(SEP24ClaimsContextKey).(*SEP24JWTClaims)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})

		now := time.Now()
		validTransactionID := "valid-transaction-id"
		validToken, err := jwtManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "test.com", validTransactionID)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, "/authenticated_success", nil)
		require.NoError(t, err)
		authHeader := "Bearer " + validToken
		req.Header.Set("Authorization", authHeader)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.JSONEq(t, `{"status":"ok"}`, string(respBody))

		// validate the context claims
		require.NotNil(t, contextClaims)
		require.Equal(t, "test.com", contextClaims.ClientDomain())
		require.Equal(t, "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", contextClaims.SEP10StellarAccount())
		require.Equal(t, validTransactionID, contextClaims.TransactionID())
		require.Empty(t, contextClaims.SEP10StellarMemo())
		require.True(t, contextClaims.ExpiresAt().After(now.Add(time.Duration(4000*time.Millisecond))))
		require.True(t, contextClaims.ExpiresAt().Before(now.Add(time.Duration(5000*time.Millisecond))))
	})

	t.Run("token with empty client domain is valid in testnet 🎉", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.DefaultLogger.SetLevel(log.WarnLevel)

		var contextClaims *SEP24JWTClaims
		require.Nil(t, contextClaims)
		r.With(SEP24HeaderTokenAuthenticateMiddleware(jwtManager, network.TestNetworkPassphrase)).Get("/authenticated_testnet", func(w http.ResponseWriter, r *http.Request) {
			contextClaims = r.Context().Value(SEP24ClaimsContextKey).(*SEP24JWTClaims)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})

		validTransactionID := "valid-transaction-id"
		validToken, err := jwtManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "", validTransactionID)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, "/authenticated_testnet", nil)
		require.NoError(t, err)
		authHeader := "Bearer " + validToken
		req.Header.Set("Authorization", authHeader)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.JSONEq(t, `{"status":"ok"}`, string(respBody))

		// check client domain
		require.Empty(t, contextClaims.ClientDomain())

		// validate logs
		require.Contains(t, buf.String(), "missing client domain in the token claims")
	})

	t.Run("token with empty client domain returns error in pubnet 🎉", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		r.With(SEP24HeaderTokenAuthenticateMiddleware(jwtManager, network.PublicNetworkPassphrase)).Get("/authenticated_testnet", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(json.RawMessage(`{"status":"ok"}`))
			require.NoError(t, err)
		})

		validTransactionID := "valid-transaction-id"
		validToken, err := jwtManager.GenerateSEP24Token("GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", "", "", validTransactionID)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, "/authenticated_testnet", nil)
		require.NoError(t, err)
		authHeader := "Bearer " + validToken
		req.Header.Set("Authorization", authHeader)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.JSONEq(t, `{"error":"The request was invalid in some way."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "missing client domain in the token claims")
	})
}
