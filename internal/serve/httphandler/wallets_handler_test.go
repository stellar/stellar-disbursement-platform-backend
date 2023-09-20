package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_WalletsHandlerGetWallets(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	t.Run("successfully returns a list of wallets", func(t *testing.T) {
		expected := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)
		expectedJSON, err := json.Marshal(expected)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallets", nil)
		http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.JSONEq(t, string(expectedJSON), string(respBody))
	})
}

func Test_WalletsHandlerPostWallets(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

	t.Run("returns BadRequest when payload is invalid", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(`invalid`))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way."}`, string(respBody))

		rr = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(`{}`))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp = rr.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		expected := `
			{
				"error": "invalid request body",
				"extras": {
					"name": "name is required",
					"homepage": "homepage is required",
					"deep_link_schema": "deep_link_schema is required",
					"sep_10_client_domain": "sep_10_client_domain is required",
					"assets_ids": "provide at least one asset ID"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expected, string(respBody))

		payload := `
			{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": "newwallet://sdp",
				"sep_10_client_domain": "https://newwallet.com"
			}
		`
		rr = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(payload))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp = rr.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		expected = `
			{
				"error": "invalid request body",
				"extras": {
					"assets_ids": "provide at least one asset ID"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expected, string(respBody))
	})

	t.Run("returns BadRequest when the URLs are invalids", func(t *testing.T) {
		payload := fmt.Sprintf(`
			{
				"name": "New Wallet",
				"homepage": "newwallet.com",
				"deep_link_schema": "deeplink/sdp",
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": [%q]
			}
		`, asset.ID)
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(payload))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		expected := `
			{
				"error": "invalid request body",
				"extras": {
					"deep_link_schema": "invalid deep link schema provided",
					"homepage": "invalid homepage URL provided"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expected, string(respBody))
	})

	t.Run("returns Conflict when creating a duplicated wallet", func(t *testing.T) {
		wallet := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)[0]

		// Duplicated Name
		payload := fmt.Sprintf(`
			{
				"name": %q,
				"homepage": %q,
				"deep_link_schema": %q,
				"sep_10_client_domain": %q,
				"assets_ids": [%q]
			}
		`, wallet.Name, wallet.Homepage, wallet.DeepLinkSchema, wallet.SEP10ClientDomain, asset.ID)
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(payload))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.JSONEq(t, `{"error": "a wallet with this name already exists"}`, string(respBody))

		// Duplicated Homepage
		payload = fmt.Sprintf(`
			{
				"name": "New Wallet",
				"homepage": %q,
				"deep_link_schema": %q,
				"sep_10_client_domain": %q,
				"assets_ids": [%q]
			}
		`, wallet.Homepage, wallet.DeepLinkSchema, wallet.SEP10ClientDomain, asset.ID)
		rr = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(payload))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp = rr.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.JSONEq(t, `{"error": "a wallet with this homepage already exists"}`, string(respBody))

		// Duplicated Deep Link Schema
		payload = fmt.Sprintf(`
			{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": %q,
				"sep_10_client_domain": %q,
				"assets_ids": [%q]
			}
		`, wallet.DeepLinkSchema, wallet.SEP10ClientDomain, asset.ID)
		rr = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(payload))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp = rr.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.JSONEq(t, `{"error": "a wallet with this deep link schema already exists"}`, string(respBody))

		// Invalid asset ID
		payload = fmt.Sprintf(`
			{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": "newwallet://sdp",
				"sep_10_client_domain": %q,
				"assets_ids": ["asset-id"]
			}
		`, wallet.SEP10ClientDomain)
		rr = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(payload))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp = rr.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.JSONEq(t, `{"error": "invalid asset ID"}`, string(respBody))
	})

	t.Run("creates wallet successfully", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		payload := fmt.Sprintf(`
			{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": "newwallet://deeplink/sdp",
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": [%q]
			}
		`, asset.ID)
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(payload))
		require.NoError(t, err)

		http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		wallet, err := models.Wallets.GetByWalletName(ctx, "New Wallet")
		require.NoError(t, err)

		walletAssets, err := models.Wallets.GetAssets(ctx, wallet.ID)
		require.NoError(t, err)

		assert.Equal(t, "https://newwallet.com", wallet.Homepage)
		assert.Equal(t, "newwallet://deeplink/sdp", wallet.DeepLinkSchema)
		assert.Equal(t, "newwallet.com", wallet.SEP10ClientDomain)
		assert.Len(t, walletAssets, 1)
	})
}

func Test_WalletsHandlerDeleteWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	r := chi.NewRouter()
	r.Delete("/wallets/{id}", handler.DeleteWallet)

	t.Run("returns NotFound when wallet doesn't exist", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "/wallets/unknown", nil)
		require.NoError(t, err)

		r.ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Resource not found."}`, string(respBody))
	})

	t.Run("deletes wallet successfully", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://mywallet.com", "mywallet.com", "mywallet://")

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("/wallets/%s", wallet.ID), nil)
		require.NoError(t, err)

		r.ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("returns NotFound when tries to delete a wallet already deleted", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://mywallet.com", "mywallet.com", "mywallet://")

		q := `UPDATE wallets SET deleted_at = NOW() WHERE id = $1`
		_, err := dbConnectionPool.ExecContext(ctx, q, wallet.ID)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("/wallets/%s", wallet.ID), nil)
		require.NoError(t, err)

		r.ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Resource not found."}`, string(respBody))
	})
}

func Test_WalletsHandlerPatchWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	r := chi.NewRouter()
	r.Patch("/wallets/{id}", handler.PatchWallets)

	t.Run("returns BadRequest when payload is invalid", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://mywallet.com", "mywallet.com", "mywallet://")

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/wallets/%s", wallet.ID), strings.NewReader(`{}`))
		require.NoError(t, err)

		r.ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "invalid request body", "extras": {"enabled": "enabled is required"}}`, string(respBody))
	})

	t.Run("returns NotFound when wallet doesn't exist", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/wallets/unknown", strings.NewReader(`{"enabled": true}`))
		require.NoError(t, err)

		r.ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Resource not found."}`, string(respBody))
	})

	t.Run("updates wallet successfully", func(t *testing.T) {
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://mywallet.com", "mywallet.com", "mywallet://")
		assert.True(t, wallet.Enabled)

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/wallets/%s", wallet.ID), strings.NewReader(`{"enabled": false}`))
		require.NoError(t, err)

		r.ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "wallet updated successfully"}`, string(respBody))

		wallet, err = models.Wallets.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.False(t, wallet.Enabled)

		rr = httptest.NewRecorder()
		req, err = http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("/wallets/%s", wallet.ID), strings.NewReader(`{"enabled": true}`))
		require.NoError(t, err)

		r.ServeHTTP(rr, req)

		resp = rr.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"message": "wallet updated successfully"}`, string(respBody))

		wallet, err = models.Wallets.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.True(t, wallet.Enabled)
	})
}
