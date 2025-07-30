package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_SEP24Handler_GetTransaction(t *testing.T) {
	t.Parallel()
	models := data.SetupModels(t)
	ctx := context.Background()
	handler := &SEP24Handler{
		Models:             models,
		InteractiveBaseURL: "https://example.com",
	}

	t.Run("missing id parameter", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/transaction", nil)
		http.HandlerFunc(handler.GetTransaction).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errResp httperror.HTTPError
		err := json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "id parameter is required", errResp.Message)
	})

	t.Run("transaction not found - returns incomplete status", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/transaction?id=non-existent-id", nil)
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
		assert.Contains(t, transaction["more_info_url"], "https://example.com/wallet-registration/start?transaction_id=non-existent-id")
		assert.NotEmpty(t, transaction["started_at"])
	})

	t.Run("database error", func(t *testing.T) {
		// Create a handler with a mock that returns an error
		mockHandler := &SEP24Handler{
			Models:             models,
			InteractiveBaseURL: "https://example.com",
		}

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/transaction?id=test-id", nil)
		http.HandlerFunc(mockHandler.GetTransaction).ServeHTTP(rr, req)

		assert.NotEqual(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("registered receiver wallet", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, models.DBConnectionPool, "Luminary", "https://luminary.com", "luminary.com", "luminary://")
		receiver := data.CreateReceiverFixture(t, ctx, models.DBConnectionPool, &data.Receiver{})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		update := data.ReceiverWalletUpdate{
			AnchorPlatformTransactionID: "test-transaction-id",
			StellarAddress:              "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			StellarMemo:                 &[]string{"memo123"}[0],
			StellarMemoType:             &[]schema.MemoType{schema.MemoTypeID}[0],
		}
		err := models.ReceiverWallet.Update(ctx, receiverWallet.ID, update, models.DBConnectionPool)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/transaction?id=test-transaction-id", nil)
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

	t.Run("ready receiver wallet", func(t *testing.T) {
		// Create test data
		wallet := data.CreateWalletFixture(t, ctx, models.DBConnectionPool, "Nexus", "https://nexus.com", "nexus.com", "nexus://")
		receiver := data.CreateReceiverFixture(t, ctx, models.DBConnectionPool, &data.Receiver{})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		// Update the receiver wallet using the existing Update method
		update := data.ReceiverWalletUpdate{
			AnchorPlatformTransactionID: "test-transaction-id-2",
		}
		err := models.ReceiverWallet.Update(ctx, receiverWallet.ID, update, models.DBConnectionPool)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/transaction?id=test-transaction-id-2", nil)
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
		assert.Contains(t, transaction["more_info_url"], "https://example.com/wallet-registration/start?transaction_id=test-transaction-id-2")
		assert.NotEmpty(t, transaction["started_at"])
	})

	t.Run("draft receiver wallet - returns error status", func(t *testing.T) {
		wallet := data.CreateWalletFixture(t, ctx, models.DBConnectionPool, "Pulse", "https://pulse.com", "pulse.com", "pulse://")
		receiver := data.CreateReceiverFixture(t, ctx, models.DBConnectionPool, &data.Receiver{})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

		update := data.ReceiverWalletUpdate{
			AnchorPlatformTransactionID: "test-transaction-id-3",
		}
		err := models.ReceiverWallet.Update(ctx, receiverWallet.ID, update, models.DBConnectionPool)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/transaction?id=test-transaction-id-3", nil)
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

	t.Run("successfully returns SEP-24 info", func(t *testing.T) {
		data.CreateAssetFixture(t, ctx, models.DBConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
		data.CreateAssetFixture(t, ctx, models.DBConnectionPool, "XLM", "")

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/info", nil)
		http.HandlerFunc(handler.GetInfo).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response SEP24InfoResponse
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Len(t, response.Deposit, 2)
		assert.True(t, response.Deposit["USDC"].Enabled)
		assert.Equal(t, 1, response.Deposit["USDC"].MinAmount)
		assert.Equal(t, 10000, response.Deposit["USDC"].MaxAmount)
		assert.True(t, response.Deposit["native"].Enabled)
		assert.Equal(t, 1, response.Deposit["native"].MinAmount)
		assert.Equal(t, 10000, response.Deposit["native"].MaxAmount)

		assert.Empty(t, response.Withdraw)

		// Check fee
		assert.False(t, response.Fee.Enabled)

		// Check features
		assert.False(t, response.Features.AccountCreation)
		assert.False(t, response.Features.ClaimableBalances)
	})
}

func Test_SEP24Handler_PostDepositInteractive(t *testing.T) {
	t.Parallel()
	models := data.SetupModels(t)

	// Create a JWT manager for testing
	jwtManager, err := anchorplatform.NewJWTManager("test-secret-key-for-testing-purposes-only", 300000)
	require.NoError(t, err)

	handler := &SEP24Handler{
		Models:             models,
		SEP24JWTManager:    jwtManager,
		InteractiveBaseURL: "https://example.com",
	}

	t.Run("missing authorization header", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", nil)
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var errResp httperror.HTTPError
		err := json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "Missing or invalid authorization header", errResp.Message)
	})

	t.Run("invalid authorization header format", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", nil)
		req.Header.Set("Authorization", "InvalidFormat token")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var errResp httperror.HTTPError
		err := json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "Missing or invalid authorization header", errResp.Message)
	})

	t.Run("invalid token", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var errResp httperror.HTTPError
		err := json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "Invalid token", errResp.Message)
	})

	t.Run("missing asset_code in JSON request", func(t *testing.T) {
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

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errResp httperror.HTTPError
		err = json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "asset_code is required", errResp.Message)
	})

	t.Run("missing asset_code in form request", func(t *testing.T) {
		token, err := jwtManager.GenerateSEP10Token(
			"https://example.com",
			"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			"jti-123",
			"titan.example.com",
			"example.com",
			time.Now(),
			time.Now().Add(time.Hour),
		)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errResp httperror.HTTPError
		err = json.Unmarshal(rr.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "asset_code is required", errResp.Message)
	})

	t.Run("successful JSON request", func(t *testing.T) {
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

		requestBody := map[string]string{
			"asset_code": "USDC",
			"account":    "GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			"lang":       "en",
		}
		requestBodyBytes, _ := json.Marshal(requestBody)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", bytes.NewReader(requestBodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "interactive_customer_info_needed", response["type"])
		assert.Contains(t, response["url"], "https://example.com/wallet-registration/start")
		assert.Contains(t, response["url"], "transaction_id=")
		assert.Contains(t, response["url"], "token=")
		assert.Contains(t, response["url"], "lang=en")
		assert.NotEmpty(t, response["id"])
	})

	t.Run("successful form request", func(t *testing.T) {
		token, err := jwtManager.GenerateSEP10Token(
			"https://example.com",
			"GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU",
			"jti-123",
			"necron.example.com",
			"example.com",
			time.Now(),
			time.Now().Add(time.Hour),
		)
		require.NoError(t, err)

		formData := "asset_code=XLM&account=GBVFTZL5HIPT4PFQVTZVIWR77V7LWYCXU4CLYWWHHOEXB64XPG5LDMTU&lang=es"

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", strings.NewReader(formData))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		http.HandlerFunc(handler.PostDepositInteractive).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]any
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "interactive_customer_info_needed", response["type"])
		assert.Contains(t, response["url"], "https://example.com/wallet-registration/start")
		assert.Contains(t, response["url"], "transaction_id=")
		assert.Contains(t, response["url"], "token=")
		assert.Contains(t, response["url"], "lang=es")
		assert.NotEmpty(t, response["id"])
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

		requestBody := map[string]string{
			"asset_code": "USDC",
			"lang":       "en",
		}
		requestBodyBytes, _ := json.Marshal(requestBody)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", bytes.NewReader(requestBodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
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

		requestBody := map[string]string{
			"asset_code": "XLM",
		}
		requestBodyBytes, _ := json.Marshal(requestBody)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", bytes.NewReader(requestBodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
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

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/deposit/interactive", strings.NewReader(`{"invalid": json`))
		req.Header.Set("Authorization", "Bearer "+token)
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
