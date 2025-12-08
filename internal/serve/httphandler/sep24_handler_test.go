package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_SEP24Handler_GetTransaction(t *testing.T) {
	t.Parallel()
	models := data.SetupModels(t)
	ctx := context.Background()

	jwtManager, err := sepauth.NewJWTManager("test_secret_1234567890", 15000)
	require.NoError(t, err)

	handler := &SEP24Handler{
		Models:             models,
		SEP24JWTManager:    jwtManager,
		InteractiveBaseURL: "https://example.com",
	}

	t.Run("missing id parameter", func(t *testing.T) {
		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			ClientDomain: "example.com",
			HomeDomain:   "example.com",
			TokenType:    sepauth.WebAuthTokenTypeSEP10,
		}

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("GET", "/transaction", nil, webAuthClaims)
		http.HandlerFunc(handler.GetTransaction).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errResp httperror.HTTPError
		err := json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "id parameter is required", errResp.Message)
	})

	t.Run("transaction not found returns incomplete status", func(t *testing.T) {
		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			ClientDomain: "example.com",
			HomeDomain:   "example.com",
			TokenType:    sepauth.WebAuthTokenTypeSEP10,
		}

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("GET", "/transaction?id=non-existent-id", nil, webAuthClaims)
		http.HandlerFunc(handler.GetTransaction).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		transaction := response["transaction"].(map[string]any)
		assert.Equal(t, "non-existent-id", transaction["id"])
		assert.Equal(t, "deposit", transaction["kind"])
		assert.Equal(t, false, transaction["refunded"])
		assert.Equal(t, "incomplete", transaction["status"])
		moreInfoURL := transaction["more_info_url"].(string)
		assert.Contains(t, moreInfoURL, "https://example.com/wallet-registration/start?transaction_id=non-existent-id&token=")
		assert.NotEmpty(t, transaction["started_at"])
	})

	t.Run("registered receiver wallet returns completed status", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, models.DBConnectionPool, "Luminary", "https://luminary.com", "luminary.com", "luminary://")
		receiver := data.CreateReceiverFixture(t, ctx, models.DBConnectionPool, &data.Receiver{})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		update := data.ReceiverWalletUpdate{
			SEP24TransactionID: "test-transaction-id",
			StellarAddress:     "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			StellarMemo:        &[]string{"memo123"}[0],
			StellarMemoType:    &[]schema.MemoType{schema.MemoTypeID}[0],
		}
		err := models.ReceiverWallet.Update(ctx, receiverWallet.ID, update, models.DBConnectionPool)
		require.NoError(t, err)

		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			ClientDomain: "example.com",
			HomeDomain:   "example.com",
			TokenType:    sepauth.WebAuthTokenTypeSEP10,
		}

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("GET", "/transaction?id=test-transaction-id", nil, webAuthClaims)
		http.HandlerFunc(handler.GetTransaction).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		transaction := response["transaction"].(map[string]any)
		assert.Equal(t, "test-transaction-id", transaction["id"])
		assert.Equal(t, "deposit", transaction["kind"])
		assert.Equal(t, false, transaction["refunded"])
		assert.Equal(t, "completed", transaction["status"])
		assert.Equal(t, "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU", transaction["to"])
		assert.Equal(t, "memo123", transaction["deposit_memo"])
		assert.Equal(t, "id", transaction["deposit_memo_type"])
		assert.Equal(t, "", transaction["stellar_transaction_id"])
		assert.NotEmpty(t, transaction["completed_at"])
		assert.NotEmpty(t, transaction["started_at"])
	})

	t.Run("ready receiver wallet returns pending status", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, models.DBConnectionPool, "Nexus", "https://nexus.com", "nexus.com", "nexus://")
		receiver := data.CreateReceiverFixture(t, ctx, models.DBConnectionPool, &data.Receiver{})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		update := data.ReceiverWalletUpdate{
			SEP24TransactionID: "test-transaction-id-2",
		}
		err := models.ReceiverWallet.Update(ctx, receiverWallet.ID, update, models.DBConnectionPool)
		require.NoError(t, err)

		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			ClientDomain: "example.com",
			HomeDomain:   "example.com",
			TokenType:    sepauth.WebAuthTokenTypeSEP10,
		}

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("GET", "/transaction?id=test-transaction-id-2", nil, webAuthClaims)
		http.HandlerFunc(handler.GetTransaction).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		transaction := response["transaction"].(map[string]any)
		assert.Equal(t, "test-transaction-id-2", transaction["id"])
		assert.Equal(t, "deposit", transaction["kind"])
		assert.Equal(t, false, transaction["refunded"])
		assert.Equal(t, "pending_user_info_update", transaction["status"])
		moreInfoURL := transaction["more_info_url"].(string)
		assert.Contains(t, moreInfoURL, "https://example.com/wallet-registration/start?transaction_id=test-transaction-id-2&token=")
		assert.NotEmpty(t, transaction["started_at"])
	})

	t.Run("draft receiver wallet returns error status", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, models.DBConnectionPool, "Pulse", "https://pulse.com", "pulse.com", "pulse://")
		receiver := data.CreateReceiverFixture(t, ctx, models.DBConnectionPool, &data.Receiver{})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

		update := data.ReceiverWalletUpdate{
			SEP24TransactionID: "test-transaction-id-3",
		}
		err := models.ReceiverWallet.Update(ctx, receiverWallet.ID, update, models.DBConnectionPool)
		require.NoError(t, err)

		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			ClientDomain: "example.com",
			HomeDomain:   "example.com",
			TokenType:    "sep10",
		}

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("GET", "/transaction?id=test-transaction-id-3", nil, webAuthClaims)
		http.HandlerFunc(handler.GetTransaction).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		transaction := response["transaction"].(map[string]any)
		assert.Equal(t, "test-transaction-id-3", transaction["id"])
		assert.Equal(t, "deposit", transaction["kind"])
		assert.Equal(t, false, transaction["refunded"])
		assert.Equal(t, "error", transaction["status"])
		assert.NotEmpty(t, transaction["started_at"])
	})
}

func Test_SEP24Handler_GetInfo(t *testing.T) {
	t.Parallel()
	models := data.SetupModels(t)
	ctx := context.Background()
	handler := &SEP24Handler{
		Models: models,
	}

	t.Run("returns SEP-24 info with assets", func(t *testing.T) {
		data.CreateAssetFixture(t, ctx, models.DBConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		data.CreateAssetFixture(t, ctx, models.DBConnectionPool, "XLM", "")

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/info", nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetInfo).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response SEP24InfoResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Len(t, response.Deposit, 2)
		assert.True(t, response.Deposit["USDC"].Enabled)
		assert.Equal(t, 1, response.Deposit["USDC"].MinAmount)
		assert.Equal(t, 10000, response.Deposit["USDC"].MaxAmount)
		assert.True(t, response.Deposit["native"].Enabled)
		assert.Equal(t, 1, response.Deposit["native"].MinAmount)
		assert.Equal(t, 10000, response.Deposit["native"].MaxAmount)

		assert.Empty(t, response.Withdraw)
		assert.False(t, response.Fee.Enabled)
		assert.False(t, response.Features.AccountCreation)
		assert.False(t, response.Features.ClaimableBalances)
	})
}

func Test_SEP24Handler_PostDepositInteractive(t *testing.T) {
	t.Parallel()
	models := data.SetupModels(t)

	jwtManager, err := sepauth.NewJWTManager("test-secret-key-for-testing-purposes-only", 300000)
	require.NoError(t, err)

	handler := &SEP24Handler{
		Models:             models,
		SEP24JWTManager:    jwtManager,
		InteractiveBaseURL: "https://example.com",
	}

	t.Run("missing authorization header", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("POST", "/deposit/interactive", nil, nil)
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var errResp httperror.HTTPError
		err := json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "Missing or invalid authorization header", errResp.Message)
	})

	t.Run("missing asset_code in JSON or form request", func(t *testing.T) {
		buildAuthClaims := func() *sepauth.WebAuthClaims {
			token, err := jwtManager.GenerateSEP10Token(
				"https://example.com",
				"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
				"jti-123",
				"mars.example.com",
				"example.com",
				time.Now(),
				time.Now().Add(time.Hour),
			)
			require.NoError(t, err)

			sep10Claims, err := jwtManager.ParseSEP10TokenClaims(token)
			require.NoError(t, err)
			return &sepauth.WebAuthClaims{
				Subject:      sep10Claims.Subject,
				ClientDomain: sep10Claims.ClientDomain,
				HomeDomain:   sep10Claims.HomeDomain,
				TokenType:    sepauth.WebAuthTokenTypeSEP10,
			}
		}

		testCases := []struct {
			name        string
			contentType string
			body        string
		}{
			{
				name:        "json",
				contentType: "application/json",
				body:        `{}`,
			},
			{
				name:        "form",
				contentType: "application/x-www-form-urlencoded",
				body:        "",
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				rr := httptest.NewRecorder()
				req := setupRequestWithWebAuthClaims("POST", "/deposit/interactive", strings.NewReader(tc.body), buildAuthClaims())
				req.Header.Set("Content-Type", tc.contentType)

				http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

				resp := rr.Result()
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

				var errResp httperror.HTTPError
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				require.NoError(t, err)
				assert.Equal(t, "asset_code is required", errResp.Message)
			})
		}
	})

	t.Run("successful deposit requests", func(t *testing.T) {
		buildAuthClaims := func() *sepauth.WebAuthClaims {
			token, err := jwtManager.GenerateSEP10Token(
				"https://example.com",
				"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
				"jti-123",
				"terra.example.com",
				"example.com",
				time.Now(),
				time.Now().Add(time.Hour),
			)
			require.NoError(t, err)

			sep10Claims, err := jwtManager.ParseSEP10TokenClaims(token)
			require.NoError(t, err)
			return &sepauth.WebAuthClaims{
				Subject:      sep10Claims.Subject,
				ClientDomain: sep10Claims.ClientDomain,
				HomeDomain:   sep10Claims.HomeDomain,
				TokenType:    sepauth.WebAuthTokenTypeSEP10,
			}
		}

		testCases := []struct {
			name        string
			contentType string
			body        string
			expectLang  string
		}{
			{
				name:        "json with explicit account/lang",
				contentType: "application/json",
				body:        `{"asset_code":"USDC","account":"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU","lang":"en"}`,
				expectLang:  "lang=en",
			},
			{
				name:        "form with explicit account/lang",
				contentType: "application/x-www-form-urlencoded",
				body:        "asset_code=XLM&account=GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU&lang=es",
				expectLang:  "lang=es",
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				rr := httptest.NewRecorder()
				req := setupRequestWithWebAuthClaims("POST", "/deposit/interactive", strings.NewReader(tc.body), buildAuthClaims())
				req.Header.Set("Content-Type", tc.contentType)
				http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

				resp := rr.Result()
				assert.Equal(t, http.StatusOK, resp.StatusCode)

				var response map[string]any
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, "interactive_customer_info_needed", response["type"])
				assert.Contains(t, response["url"], "https://example.com/wallet-registration/start")
				assert.Contains(t, response["url"], "transaction_id=")
				assert.Contains(t, response["url"], "token=")
				assert.Contains(t, response["url"], tc.expectLang)
				assert.NotEmpty(t, response["id"])
			})
		}
	})

	t.Run("uses account from token when not provided", func(t *testing.T) {
		token, err := jwtManager.GenerateSEP10Token(
			"https://example.com",
			"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			"jti-123",
			"eldar.example.com",
			"example.com",
			time.Now(),
			time.Now().Add(time.Hour),
		)
		require.NoError(t, err)

		sep10Claims, err := jwtManager.ParseSEP10TokenClaims(token)
		require.NoError(t, err)
		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      sep10Claims.Subject,
			ClientDomain: sep10Claims.ClientDomain,
			HomeDomain:   sep10Claims.HomeDomain,
			TokenType:    "sep10",
		}

		requestBody := map[string]string{
			"asset_code": "USDC",
			"lang":       "en",
		}
		requestBodyBytes, err := json.Marshal(requestBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("POST", "/deposit/interactive", bytes.NewReader(requestBodyBytes), webAuthClaims)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "interactive_customer_info_needed", response["type"])
		assert.NotEmpty(t, response["id"])
	})

	t.Run("uses default language when not provided", func(t *testing.T) {
		token, err := jwtManager.GenerateSEP10Token(
			"https://example.com",
			"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			"jti-123",
			"ork.example.com",
			"example.com",
			time.Now(),
			time.Now().Add(time.Hour),
		)
		require.NoError(t, err)

		sep10Claims, err := jwtManager.ParseSEP10TokenClaims(token)
		require.NoError(t, err)
		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      sep10Claims.Subject,
			ClientDomain: sep10Claims.ClientDomain,
			HomeDomain:   sep10Claims.HomeDomain,
			TokenType:    "sep10",
		}

		requestBody := map[string]string{
			"asset_code": "XLM",
		}
		requestBodyBytes, err := json.Marshal(requestBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("POST", "/deposit/interactive", bytes.NewReader(requestBodyBytes), webAuthClaims)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "interactive_customer_info_needed", response["type"])
		assert.Contains(t, response["url"], "lang=en")
		assert.NotEmpty(t, response["id"])
	})

	t.Run("accepts webauth (SEP-10/45) claims", func(t *testing.T) {
		testCases := []struct {
			name         string
			claims       *sepauth.WebAuthClaims
			expectedLang string
		}{
			{
				name: "SEP-10 pubkey subject",
				claims: &sepauth.WebAuthClaims{
					Subject:      "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
					ClientDomain: "client.example.com",
					HomeDomain:   "home.example.com",
					TokenType:    sepauth.WebAuthTokenTypeSEP10,
				},
				expectedLang: "en",
			},
			{
				name: "SEP-45 contract subject",
				claims: &sepauth.WebAuthClaims{
					Subject:      "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4",
					ClientDomain: "client.example.com",
					HomeDomain:   "home.example.com",
					TokenType:    sepauth.WebAuthTokenTypeSEP45,
				},
				expectedLang: "en",
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				requestBody := map[string]string{
					"asset_code": "USDC",
				}
				bodyBytes, err := json.Marshal(requestBody)
				require.NoError(t, err)

				rr := httptest.NewRecorder()
				req := setupRequestWithWebAuthClaims("POST", "/deposit/interactive", bytes.NewReader(bodyBytes), tc.claims)
				req.Header.Set("Content-Type", "application/json")
				http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

				assert.Equal(t, http.StatusOK, rr.Code)

				var response map[string]any
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, "interactive_customer_info_needed", response["type"])
				assert.Contains(t, response["url"], "lang="+tc.expectedLang)
			})
		}
	})

	t.Run("invalid JSON in request body", func(t *testing.T) {
		token, err := jwtManager.GenerateSEP10Token(
			"https://example.com",
			"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			"jti-123",
			"chaos.example.com",
			"example.com",
			time.Now(),
			time.Now().Add(time.Hour),
		)
		require.NoError(t, err)

		sep10Claims, err := jwtManager.ParseSEP10TokenClaims(token)
		require.NoError(t, err)
		webAuthClaims := &sepauth.WebAuthClaims{
			Subject:      sep10Claims.Subject,
			ClientDomain: sep10Claims.ClientDomain,
			HomeDomain:   sep10Claims.HomeDomain,
			TokenType:    "sep10",
		}

		rr := httptest.NewRecorder()
		req := setupRequestWithWebAuthClaims("POST", "/deposit/interactive", strings.NewReader(`{"invalid": json`), webAuthClaims)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errResp httperror.HTTPError
		err = json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "Invalid JSON", errResp.Message)
	})
}

func setupRequestWithWebAuthClaims(method, url string, body io.Reader, webAuthClaims *sepauth.WebAuthClaims) *http.Request {
	ctx := context.Background()
	if webAuthClaims != nil {
		ctx = context.WithValue(ctx, sepauth.WebAuthClaimsContextKey, webAuthClaims)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		panic(err)
	}
	return req
}
