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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_WalletsHandlerGetWallets(t *testing.T) {
	dbConnectionPool := getDBConnectionPool(t)

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

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

	t.Run("successfully returns a list of enabled wallets", func(t *testing.T) {
		wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		// enable first wallet and disable all others
		data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallets[0].ID)
		for _, wallet := range wallets[1:] {
			data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallet.ID)
		}

		expected, err := models.Wallets.Get(ctx, wallets[0].ID)
		require.NoError(t, err)

		expectedJSON, err := json.Marshal([]data.Wallet{*expected})
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallets?enabled=true", nil)
		http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.JSONEq(t, string(expectedJSON), string(respBody))
	})

	t.Run("successfully returns a list of disabled wallets", func(t *testing.T) {
		wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		// disable first wallet and enable all others
		data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[0].ID)
		for _, wallet := range wallets[1:] {
			data.EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallet.ID)
		}

		expected, err := models.Wallets.Get(ctx, wallets[0].ID)
		require.NoError(t, err)

		expectedJSON, err := json.Marshal([]data.Wallet{*expected})
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallets?enabled=false", nil)
		http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		require.JSONEq(t, string(expectedJSON), string(respBody))
	})

	t.Run("successfully returns a list of user managed wallets", func(t *testing.T) {
		wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		// make first wallet user managed
		data.MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallets?user_managed=true", nil)
		http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		respWallets := []data.Wallet{}
		err = json.Unmarshal(respBody, &respWallets)
		require.NoError(t, err)
		assert.Equal(t, 1, len(respWallets))
		assert.Equal(t, wallets[0].ID, respWallets[0].ID)
		assert.Equal(t, wallets[0].Name, respWallets[0].Name)
	})

	t.Run("bad request when user_managed parameter isn't a bool", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallets?user_managed=xxx", nil)
		http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var httpErr httperror.HTTPError
		err = json.Unmarshal(respBody, &httpErr)
		require.NoError(t, err)
		assert.Equal(t, "invalid 'user_managed' parameter value", httpErr.Extras["validation_error"])
	})

	t.Run("bad request when enabled parameter isn't a bool", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallets?enabled=xxx", nil)
		http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var httpErr httperror.HTTPError
		err = json.Unmarshal(respBody, &httpErr)
		require.NoError(t, err)
		assert.Equal(t, "invalid 'enabled' parameter value", httpErr.Extras["validation_error"])
	})
}

func Test_WalletsHandlerPostWallets(t *testing.T) {
	dbConnectionPool := getDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	assetResolver := services.NewWalletAssetResolver(models.Assets)
	handler := &WalletsHandler{Models: models, WalletAssetResolver: assetResolver}

	// Fixture setup
	wallet := data.ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)[0]
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

	// Define test cases
	testCases := []struct {
		name           string
		payload        string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "🔴-400-BadRequest when payload is invalid",
			payload:        `invalid`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error": "The request was invalid in some way."}`,
		},
		{
			name:           "🔴-400-BadRequest when payload is missing required fields",
			payload:        `{}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"name": "name is required",
					"homepage": "homepage is required",
					"deep_link_schema": "deep_link_schema is required",
					"sep_10_client_domain": "sep_10_client_domain is required",
					"assets": "provide at least one 'assets_ids' or 'assets'"
				}
			}`,
		},
		{
			name: "🔴-400-BadRequest when assets_ids is missing",
			payload: `{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": "newwallet://sdp",
				"sep_10_client_domain": "https://newwallet.com"
			}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"assets": "provide at least one 'assets_ids' or 'assets'"
				}
			}`,
		},
		{
			name: "🔴-400-BadRequest when URLs are invalid",
			payload: fmt.Sprintf(`{
				"name": "New Wallet",
				"homepage": "newwallet.com",
				"deep_link_schema": "deeplink/sdp",
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": [%q]
			}`, asset.ID),
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"deep_link_schema": "invalid deep link schema provided",
					"homepage": "invalid homepage URL provided"
				}
			}`,
		},
		{
			name: "🔴-400-BadRequest when creating a wallet with an invalid asset ID",
			payload: `{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": "newwallet://sdp",
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": ["invalid-asset-id"]
			}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error": "invalid asset ID"}`,
		},
		{
			name: "🔴-409-Conflict when creating a duplicated wallet (name)",
			payload: fmt.Sprintf(`{
				"name": %q,
				"homepage": "https://newwallet.com",
				"deep_link_schema": "newwallet://sdp",
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": [%q]
			}`, wallet.Name, asset.ID),
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error": "a wallet with this name already exists"}`,
		},
		{
			name: "🔴-409-Conflict when creating a duplicated wallet (homepage)",
			payload: fmt.Sprintf(`{
				"name": "New Wallet",
				"homepage": %q,
				"deep_link_schema": "newwallet://sdp",
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": [%q]
			}`, wallet.Homepage, asset.ID),
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error": "a wallet with this homepage already exists"}`,
		},
		{
			name: "🔴-409-Conflict when creating a duplicated wallet (deep_link_schema)",
			payload: fmt.Sprintf(`{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": %q,
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": [%q]
			}`, wallet.DeepLinkSchema, asset.ID),
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error": "a wallet with this deep link schema already exists"}`,
		},
		{
			name: "🟢-successfully creates wallet",
			payload: fmt.Sprintf(`{
				"name": "New Wallet",
				"homepage": "https://newwallet.com",
				"deep_link_schema": "newwallet://deeplink/sdp",
				"sep_10_client_domain": "https://newwallet.com",
				"assets_ids": [%q]
			}`, asset.ID),
			expectedStatus: http.StatusCreated,
			expectedBody:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(tc.payload))
			require.NoError(t, err)

			http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			if tc.expectedBody != "" {
				assert.JSONEq(t, tc.expectedBody, string(respBody))
			} else if tc.expectedStatus == http.StatusCreated {
				wallet, err := models.Wallets.GetByWalletName(ctx, "New Wallet")
				require.NoError(t, err)

				walletAssets, err := models.Wallets.GetAssets(ctx, wallet.ID)
				require.NoError(t, err)

				assert.Equal(t, "https://newwallet.com", wallet.Homepage)
				assert.Equal(t, "newwallet://deeplink/sdp", wallet.DeepLinkSchema)
				assert.Equal(t, "newwallet.com", wallet.SEP10ClientDomain)
				assert.Len(t, walletAssets, 1)
			}
		})
	}
}

func Test_WalletsHandlerPostWallets_WithNewAssetFormat(t *testing.T) {
	dbConnectionPool := getDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	assetResolver := services.NewWalletAssetResolver(models.Assets)

	handler := &WalletsHandler{
		Models:              models,
		NetworkType:         utils.PubnetNetworkType,
		WalletAssetResolver: assetResolver,
	}

	xlm := data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.XLMAssetCode, "")
	usdc, err := models.Assets.GetOrCreate(ctx, assets.USDCAssetCode, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")
	require.NoError(t, err)

	testCases := []struct {
		name           string
		payload        string
		expectedStatus int
		expectedBody   string
		validateResult func(t *testing.T, wallet *data.Wallet)
	}{
		{
			name: "🟢 successfully creates wallet with new assets format - ID reference",
			payload: fmt.Sprintf(`{
				"name": "New Format Wallet ID",
				"homepage": "https://newformat-id.com",
				"deep_link_schema": "newformat-id://sdp",
				"sep_10_client_domain": "newformat-id.com",
				"assets": [
					{"id": %q},
					{"id": %q}
				]
			}`, xlm.ID, usdc.ID),
			expectedStatus: http.StatusCreated,
			validateResult: func(t *testing.T, wallet *data.Wallet) {
				assert.Equal(t, "New Format Wallet ID", wallet.Name)
				assert.Len(t, wallet.Assets, 2)

				assetCodes := []string{wallet.Assets[0].Code, wallet.Assets[1].Code}
				assert.Contains(t, assetCodes, assets.XLMAssetCode)
				assert.Contains(t, assetCodes, assets.USDCAssetCode)
			},
		},
		{
			name: "🟢 successfully creates wallet with native asset reference",
			payload: `{
				"name": "Native Asset Wallet",
				"homepage": "https://native-wallet.com",
				"deep_link_schema": "native://sdp",
				"sep_10_client_domain": "native-wallet.com",
				"assets": [
					{"type": "native"}
				]
			}`,
			expectedStatus: http.StatusCreated,
			validateResult: func(t *testing.T, wallet *data.Wallet) {
				assert.Len(t, wallet.Assets, 1)
				assert.Equal(t, assets.XLMAssetCode, wallet.Assets[0].Code)
				assert.Equal(t, "", wallet.Assets[0].Issuer)
			},
		},
		{
			name: "🟢 successfully creates wallet with classic asset reference",
			payload: `{
				"name": "Classic Asset Wallet",
				"homepage": "https://classic-wallet.com",
				"deep_link_schema": "classic://sdp",
				"sep_10_client_domain": "classic-wallet.com",
				"assets": [
					{
						"type": "classic",
						"code": "USDC",
						"issuer": "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
					}
				]
			}`,
			expectedStatus: http.StatusCreated,
			validateResult: func(t *testing.T, wallet *data.Wallet) {
				assert.Len(t, wallet.Assets, 1)
				assert.Equal(t, assets.USDCAssetCode, wallet.Assets[0].Code)
				assert.Equal(t, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", wallet.Assets[0].Issuer)
			},
		},
		{
			name: "🟢 successfully creates wallet with mixed asset references",
			payload: fmt.Sprintf(`{
				"name": "Mixed Assets Wallet",
				"homepage": "https://mixed-wallet.com",
				"deep_link_schema": "mixed://sdp",
				"sep_10_client_domain": "mixed-wallet.com",
				"assets": [
					{"id": %q},
					{"type": "native"}
				]
			}`, usdc.ID),
			expectedStatus: http.StatusCreated,
			validateResult: func(t *testing.T, wallet *data.Wallet) {
				assert.Len(t, wallet.Assets, 2)

				assetMap := make(map[string]string)
				for _, asset := range wallet.Assets {
					assetMap[asset.Code] = asset.Issuer
				}

				assert.Equal(t, "", assetMap[assets.XLMAssetCode])
				assert.Equal(t, "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", assetMap["USDC"])
			},
		},
		{
			name: "🟢 successfully creates wallet with enabled=false",
			payload: fmt.Sprintf(`{
				"name": "Disabled Wallet",
				"homepage": "https://disabled-wallet.com",
				"deep_link_schema": "disabled://sdp",
				"sep_10_client_domain": "disabled-wallet.com",
				"assets": [{"id": %q}],
				"enabled": false
			}`, xlm.ID),
			expectedStatus: http.StatusCreated,
			validateResult: func(t *testing.T, wallet *data.Wallet) {
				assert.False(t, wallet.Enabled)
			},
		},
		{
			name: "🔴 fails when mixing assets_ids and assets",
			payload: fmt.Sprintf(`{
				"name": "Mixed Format Wallet",
				"homepage": "https://mixed-format.com",
				"deep_link_schema": "mixed-format://sdp",
				"sep_10_client_domain": "mixed-format.com",
				"assets_ids": [%q],
				"assets": [{"type": "native"}]
			}`, xlm.ID),
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"assets": "cannot use both 'assets_ids' and 'assets' fields simultaneously"
				}
			}`,
		},
		{
			name: "🔴 fails with invalid asset reference",
			payload: `{
				"name": "Invalid Asset Wallet",
				"homepage": "https://invalid-asset.com",
				"deep_link_schema": "invalid-asset://sdp",
				"sep_10_client_domain": "invalid-asset.com",
				"assets": [
					{"type": "classic", "code": "MISSING_ISSUER"}
				]
			}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"assets[0]": "'issuer' is required for classic asset"
				}
			}`,
		},
		{
			name: "🔴 fails with non-existent asset ID",
			payload: `{
				"name": "Non-existent Asset Wallet",
				"homepage": "https://nonexistent.com",
				"deep_link_schema": "nonexistent://sdp",
				"sep_10_client_domain": "nonexistent.com",
				"assets": [
					{"id": "non-existent-id"}
				]
			}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error": "failed to resolve asset references"}`,
		},
		{
			name: "🔴 fails with contract asset (not implemented)",
			payload: `{
				"name": "Contract Asset Wallet",
				"homepage": "https://contract.com",
				"deep_link_schema": "contract://sdp",
				"sep_10_client_domain": "contract.com",
				"assets": [
					{"type": "contract", "code": "USDC", "contract_id": "CA..."}
				]
			}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"assets[0]": "assets are not implemented yet"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/wallets", strings.NewReader(tc.payload))
			require.NoError(t, err)

			http.HandlerFunc(handler.PostWallets).ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectedBody != "" {
				assert.JSONEq(t, tc.expectedBody, string(respBody))
			} else if tc.expectedStatus == http.StatusCreated && tc.validateResult != nil {
				var wallet data.Wallet
				err = json.Unmarshal(respBody, &wallet)
				require.NoError(t, err)

				tc.validateResult(t, &wallet)
			}
		})
	}
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
