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
	assetResolver := services.NewAssetResolver(models.Assets)
	handler := &WalletsHandler{Models: models, AssetResolver: assetResolver}

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

	assetResolver := services.NewAssetResolver(models.Assets)

	handler := &WalletsHandler{
		Models:        models,
		NetworkType:   utils.PubnetNetworkType,
		AssetResolver: assetResolver,
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
	dbConnectionPool := getDBConnectionPool(t)

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

func Test_WalletsHandlerPatchWallet_Extended(t *testing.T) {
	dbConnectionPool := getDBConnectionPool(t)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

	xlm := data.CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	usdc := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")
	eurc := data.CreateAssetFixture(t, ctx, dbConnectionPool, "EURC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	handler := &WalletsHandler{
		Models:        models,
		NetworkType:   utils.TestnetNetworkType,
		AssetResolver: services.NewAssetResolver(models.Assets),
	}

	r := chi.NewRouter()
	r.Patch("/wallets/{id}", handler.PatchWallets)

	testCases := []struct {
		name           string
		setupFn        func(t *testing.T) *data.Wallet
		payload        string
		walletIDFn     func(wallet *data.Wallet) string
		expectedStatus int
		expectedBody   string
		validateResult func(t *testing.T, wallet *data.Wallet, originalWallet *data.Wallet)
	}{
		{
			name: "🟢 updates all fields successfully",
			setupFn: func(t *testing.T) *data.Wallet {
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Lion El'Jonson's Vault",
					"https://caliban.treasury",
					"caliban.treasury",
					"darkangels://")
				data.CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{xlm.ID})
				return wallet
			},
			payload: fmt.Sprintf(`{
				"name": "Roboute's Treasury",
				"homepage": "https://macragge.funds",
				"deep_link_schema": "ultramarines://sdp",
				"sep_10_client_domain": "macragge.funds",
				"enabled": false,
				"assets": [
					{"id": %q},
					{"type": "native"}
				]
			}`, usdc.ID),
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, wallet *data.Wallet, originalWallet *data.Wallet) {
				assert.Equal(t, "Roboute's Treasury", wallet.Name)
				assert.Equal(t, "https://macragge.funds", wallet.Homepage)
				assert.Equal(t, "ultramarines://sdp", wallet.DeepLinkSchema)
				assert.Equal(t, "macragge.funds", wallet.SEP10ClientDomain)
				assert.False(t, wallet.Enabled)
				assert.Len(t, wallet.Assets, 2)

				assetCodes := make([]string, 0, len(wallet.Assets))
				for _, asset := range wallet.Assets {
					assetCodes = append(assetCodes, asset.Code)
				}
				assert.Contains(t, assetCodes, "USDC")
				assert.Contains(t, assetCodes, "XLM")
			},
		},
		{
			name: "🟢 updates only name",
			setupFn: func(t *testing.T) *data.Wallet {
				return data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Jaghatai Khan's Wallet",
					"https://chogoris.speed",
					"chogoris.speed",
					"whitescars://")
			},
			payload: `{
				"name": "Vulkan's Forge-Vault"
			}`,
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, wallet *data.Wallet, originalWallet *data.Wallet) {
				assert.Equal(t, "Vulkan's Forge-Vault", wallet.Name)
				assert.Equal(t, originalWallet.Homepage, wallet.Homepage)
				assert.Equal(t, originalWallet.DeepLinkSchema, wallet.DeepLinkSchema)
				assert.Equal(t, originalWallet.SEP10ClientDomain, wallet.SEP10ClientDomain)
				assert.Equal(t, originalWallet.Enabled, wallet.Enabled)
			},
		},
		{
			name: "🟢 updates only enabled status",
			setupFn: func(t *testing.T) *data.Wallet {
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Magnus' Library Fund",
					"https://prospero.knowledge",
					"prospero.knowledge",
					"thousandsons://")
				assert.True(t, wallet.Enabled)
				return wallet
			},
			payload: `{
				"enabled": false
			}`,
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, wallet *data.Wallet, originalWallet *data.Wallet) {
				assert.False(t, wallet.Enabled)
				assert.Equal(t, originalWallet.Name, wallet.Name)
				assert.Equal(t, originalWallet.Homepage, wallet.Homepage)
			},
		},
		{
			name: "🟢 replaces assets with new list",
			setupFn: func(t *testing.T) *data.Wallet {
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Perturabo's War Chest",
					"https://olympia.siege",
					"olympia.siege",
					"ironwarriors://")
				data.CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{xlm.ID, usdc.ID})
				return wallet
			},
			payload: fmt.Sprintf(`{
				"assets": [
					{"id": %q},
					{"type": "classic", "code": "USDC", "issuer": "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"}
				]
			}`, eurc.ID),
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, wallet *data.Wallet, originalWallet *data.Wallet) {
				assert.Len(t, wallet.Assets, 2)

				assetCodes := make([]string, 0, len(wallet.Assets))
				for _, asset := range wallet.Assets {
					assetCodes = append(assetCodes, asset.Code)
				}
				assert.Contains(t, assetCodes, "EURC")
				assert.Contains(t, assetCodes, "USDC")
				assert.NotContains(t, assetCodes, "XLM")
			},
		},
		{
			name: "🟢 clears assets with empty array",
			setupFn: func(t *testing.T) *data.Wallet {
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Mortarion's Plague Purse",
					"https://barbarus.decay",
					"barbarus.decay",
					"deathguard://")
				data.CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{xlm.ID, usdc.ID})
				return wallet
			},
			payload: `{
				"assets": []
			}`,
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, wallet *data.Wallet, originalWallet *data.Wallet) {
				assert.Len(t, wallet.Assets, 0)
			},
		},
		{
			name: "🟢 preserves assets when not specified",
			setupFn: func(t *testing.T) *data.Wallet {
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Fulgrim's Art Fund",
					"https://chemos.perfection",
					"chemos.perfection",
					"emperorschildren://")
				data.CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{xlm.ID, usdc.ID})
				return wallet
			},
			payload: `{
				"name": "Rylanor's Memorial Fund"
			}`,
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, wallet *data.Wallet, originalWallet *data.Wallet) {
				assert.Equal(t, "Rylanor's Memorial Fund", wallet.Name)
				assert.Len(t, wallet.Assets, 2)
			},
		},
		{
			name: "🔴 fails when no fields provided",
			setupFn: func(t *testing.T) *data.Wallet {
				return data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Angron's Rage Budget",
					"https://nuceria.anger",
					"nuceria.anger",
					"worldeaters://")
			},
			payload:        `{}`,
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"body": "at least one field must be provided for update"
				}
			}`,
		},
		{
			name: "🔴 fails with invalid asset reference",
			setupFn: func(t *testing.T) *data.Wallet {
				return data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Konrad's Justice Fund",
					"https://nostramo.fear",
					"nostramo.fear",
					"nightlords://")
			},
			payload: `{
				"assets": [
					{"type": "contract", "code": "CHAOS", "contract_id": "test"}
				]
			}`,
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "invalid request body",
				"extras": {
					"assets[0]": "assets are not implemented yet"
				}
			}`,
		},
		{
			name: "🔴 returns not found for non-existent wallet",
			setupFn: func(t *testing.T) *data.Wallet {
				return nil
			},
			payload: `{
				"enabled": false
			}`,
			walletIDFn:     func(wallet *data.Wallet) string { return "lost-primarch" },
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"error": "Resource not found."}`,
		},
		{
			name: "🔴 fails with duplicate wallet name",
			setupFn: func(t *testing.T) *data.Wallet {
				wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Lorgar's Word Bearer Fund",
					"https://colchis.faith",
					"colchis.faith",
					"wordbearers://")

				data.CreateWalletFixture(t, ctx, dbConnectionPool,
					"Erebus' Corruption Account",
					"https://calth.betrayal",
					"calth.betrayal",
					"chaosundivided://")

				return wallet
			},
			payload: `{
				"name": "Erebus' Corruption Account"
			}`,
			walletIDFn:     func(wallet *data.Wallet) string { return wallet.ID },
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error": "a wallet with this name already exists"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wallet *data.Wallet
			if tc.setupFn != nil {
				wallet = tc.setupFn(t)
			}

			walletID := tc.walletIDFn(wallet)

			rr := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
				fmt.Sprintf("/wallets/%s", walletID),
				strings.NewReader(tc.payload))
			require.NoError(t, err)

			r.ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectedBody != "" {
				assert.JSONEq(t, tc.expectedBody, string(respBody))
			} else if tc.validateResult != nil {
				var updatedWallet data.Wallet
				err = json.Unmarshal(respBody, &updatedWallet)
				require.NoError(t, err)
				tc.validateResult(t, &updatedWallet, wallet)
			}
		})
	}
}
