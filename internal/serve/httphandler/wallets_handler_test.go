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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_WalletsHandlerGetWallets(t *testing.T) {
	models := data.SetupModels(t)
	dbPool := models.DBConnectionPool
	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	testCases := []struct {
		name           string
		setupFn        func(t *testing.T) *testWalletSetup
		queryParams    string
		expectedStatus int
		expectedBody   string
		validateResult func(t *testing.T, setup *testWalletSetup, respBody []byte)
	}{
		{
			name: "successfully returns all wallets",
			setupFn: func(t *testing.T) *testWalletSetup {
				data.DeleteAllFixtures(t, ctx, dbPool)
				wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbPool)
				return &testWalletSetup{wallets: wallets}
			},
			queryParams:    "",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, len(setup.wallets))
			},
		},

		{
			name: "successfully returns enabled wallets",
			setupFn: func(t *testing.T) *testWalletSetup {
				data.DeleteAllFixtures(t, ctx, dbPool)
				wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbPool)
				data.EnableOrDisableWalletFixtures(t, ctx, dbPool, true, wallets[0].ID)
				for _, wallet := range wallets[1:] {
					data.EnableOrDisableWalletFixtures(t, ctx, dbPool, false, wallet.ID)
				}
				return &testWalletSetup{wallets: wallets}
			},
			queryParams:    "enabled=true",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 1)
				assert.Equal(t, setup.wallets[0].ID, resultWallets[0].ID)
				assert.True(t, resultWallets[0].Enabled)
			},
		},

		{
			name: "successfully returns disabled wallets",
			setupFn: func(t *testing.T) *testWalletSetup {
				data.DeleteAllFixtures(t, ctx, dbPool)
				wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbPool)
				data.EnableOrDisableWalletFixtures(t, ctx, dbPool, false, wallets[0].ID)
				for _, wallet := range wallets[1:] {
					data.EnableOrDisableWalletFixtures(t, ctx, dbPool, true, wallet.ID)
				}
				return &testWalletSetup{wallets: wallets}
			},
			queryParams:    "enabled=false",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 1)
				assert.Equal(t, setup.wallets[0].ID, resultWallets[0].ID)
				assert.False(t, resultWallets[0].Enabled)
			},
		},
		{
			name: "successfully returns user managed wallets",
			setupFn: func(t *testing.T) *testWalletSetup {
				data.DeleteAllFixtures(t, ctx, dbPool)
				wallets := data.ClearAndCreateWalletFixtures(t, ctx, dbPool)
				data.MakeWalletUserManaged(t, ctx, dbPool, wallets[0].ID)
				return &testWalletSetup{wallets: wallets}
			},
			queryParams:    "user_managed=true",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 1)
				assert.Equal(t, setup.wallets[0].ID, resultWallets[0].ID)
				assert.Equal(t, setup.wallets[0].Name, resultWallets[0].Name)
			},
		},
		{
			name: "successfully returns wallets filtered by single supported asset",
			setupFn: func(t *testing.T) *testWalletSetup {
				return createWalletAssetsTestSetup(t, ctx, dbPool)
			},
			queryParams:    "supported_assets=USDC",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 2)
				walletNames := []string{resultWallets[0].Name, resultWallets[1].Name}
				assert.Contains(t, walletNames, "Wallet1")
				assert.Contains(t, walletNames, "Wallet2")
			},
		},
		{
			name: "successfully returns wallets filtered by multiple supported assets",
			setupFn: func(t *testing.T) *testWalletSetup {
				return createWalletAssetsTestSetup(t, ctx, dbPool)
			},
			queryParams:    "supported_assets=USDC,XLM",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 1)
				assert.Equal(t, "Wallet1", resultWallets[0].Name)
			},
		},
		{
			name: "successfully returns wallets filtered by asset ID",
			setupFn: func(t *testing.T) *testWalletSetup {
				return createWalletAssetsTestSetup(t, ctx, dbPool)
			},
			queryParams:    "supported_assets={{USDC_ID}}",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 2)
			},
		},
		{
			name: "successfully combines asset filtering with enabled filtering",
			setupFn: func(t *testing.T) *testWalletSetup {
				setup := createWalletAssetsTestSetup(t, ctx, dbPool)
				data.EnableOrDisableWalletFixtures(t, ctx, dbPool, false, setup.wallet2.ID)
				return setup
			},
			queryParams:    "supported_assets=USDC&enabled=true",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 1)
				assert.Equal(t, "Wallet1", resultWallets[0].Name)
			},
		},
		{
			name: "handles whitespace in asset list",
			setupFn: func(t *testing.T) *testWalletSetup {
				return createWalletAssetsTestSetup(t, ctx, dbPool)
			},
			queryParams:    "supported_assets= USDC , XLM ",
			expectedStatus: http.StatusOK,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var resultWallets []data.Wallet
				err := json.Unmarshal(respBody, &resultWallets)
				require.NoError(t, err)
				assert.Len(t, resultWallets, 1)
				assert.Equal(t, "Wallet1", resultWallets[0].Name)
			},
		},
		{
			name: "returns bad request for invalid user_managed parameter",
			setupFn: func(t *testing.T) *testWalletSetup {
				data.DeleteAllFixtures(t, ctx, dbPool)
				return &testWalletSetup{}
			},
			queryParams:    "user_managed=xxx",
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "Error parsing request filters",
				"extras": {
					"validation_error": "invalid 'user_managed' parameter value"
				}
			}`,
		},
		{
			name: "returns bad request for invalid enabled parameter",
			setupFn: func(t *testing.T) *testWalletSetup {
				data.DeleteAllFixtures(t, ctx, dbPool)
				return &testWalletSetup{}
			},
			queryParams:    "enabled=xxx",
			expectedStatus: http.StatusBadRequest,
			expectedBody: `{
				"error": "Error parsing request filters",
				"extras": {
					"validation_error": "invalid 'enabled' parameter value"
				}
			}`,
		},
		{
			name: "returns bad request for non-existent asset",
			setupFn: func(t *testing.T) *testWalletSetup {
				data.DeleteAllFixtures(t, ctx, dbPool)
				return &testWalletSetup{}
			},
			queryParams:    "supported_assets=NONEXISTENT",
			expectedStatus: http.StatusBadRequest,
			validateResult: func(t *testing.T, setup *testWalletSetup, respBody []byte) {
				var httpErr httperror.HTTPError
				err := json.Unmarshal(respBody, &httpErr)
				require.NoError(t, err)
				assert.Contains(t, httpErr.Extras["validation_error"], "asset 'NONEXISTENT' not found")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setup := tc.setupFn(t)

			queryParams := tc.queryParams
			if strings.Contains(queryParams, "{{USDC_ID}}") && setup.assetUSDC != nil {
				queryParams = strings.ReplaceAll(queryParams, "{{USDC_ID}}", setup.assetUSDC.ID)
			}

			rr := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/wallets?"+queryParams, nil)
			require.NoError(t, err)

			http.HandlerFunc(handler.GetWallets).ServeHTTP(rr, req)

			resp := rr.Result()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectedBody != "" {
				assert.JSONEq(t, tc.expectedBody, string(respBody))
			} else if tc.validateResult != nil {
				tc.validateResult(t, setup, respBody)
			}
		})
	}
}

type testWalletSetup struct {
	wallets   []data.Wallet
	wallet1   *data.Wallet
	wallet2   *data.Wallet
	wallet3   *data.Wallet
	assetUSDC *data.Asset
	assetXLM  *data.Asset
	assetEURT *data.Asset
}

func createWalletAssetsTestSetup(t *testing.T, ctx context.Context, dbPool db.SQLExecuter) *testWalletSetup {
	data.DeleteAllFixtures(t, ctx, dbPool)

	assetUSDC := data.CreateAssetFixture(t, ctx, dbPool, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")
	assetXLM := data.CreateAssetFixture(t, ctx, dbPool, "XLM", "")
	assetEURT := data.CreateAssetFixture(t, ctx, dbPool, "EURT", "GAP5LETOV6YIE62YAM56STDANPRDO7ZFDBGSNHJQIYGGKSMOZAHOOS2S")

	wallet1 := data.CreateWalletFixture(t, ctx, dbPool, "Wallet1", "https://wallet1.com", "wallet1.com", "wallet1://")
	wallet2 := data.CreateWalletFixture(t, ctx, dbPool, "Wallet2", "https://wallet2.com", "wallet2.com", "wallet2://")
	wallet3 := data.CreateWalletFixture(t, ctx, dbPool, "Wallet3", "https://wallet3.com", "wallet3.com", "wallet3://")

	data.CreateWalletAssets(t, ctx, dbPool, wallet1.ID, []string{assetUSDC.ID, assetXLM.ID})
	data.CreateWalletAssets(t, ctx, dbPool, wallet2.ID, []string{assetUSDC.ID, assetEURT.ID})
	data.CreateWalletAssets(t, ctx, dbPool, wallet3.ID, []string{assetXLM.ID})

	return &testWalletSetup{
		wallets:   []data.Wallet{*wallet1, *wallet2, *wallet3},
		wallet1:   wallet1,
		wallet2:   wallet2,
		wallet3:   wallet3,
		assetUSDC: assetUSDC,
		assetXLM:  assetXLM,
		assetEURT: assetEURT,
	}
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
					"homepage": "invalid URL format"
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
		Models:              models,
		NetworkType:         utils.TestnetNetworkType,
		WalletAssetResolver: services.NewWalletAssetResolver(models.Assets),
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

func Test_WalletsHandler_parseFilters(t *testing.T) {
	models := data.SetupModels(t)
	dbPool := models.DBConnectionPool
	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	// Create test assets for validation
	data.DeleteAllAssetFixtures(t, ctx, dbPool)
	assetUSDC := data.CreateAssetFixture(t, ctx, dbPool, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")
	_ = data.CreateAssetFixture(t, ctx, dbPool, "XLM", "")

	testCases := []struct {
		name           string
		queryParams    string
		expectedError  string
		expectedCount  int
		validateFilter func(t *testing.T, filters []data.Filter)
	}{
		{
			name:          "no query parameters",
			queryParams:   "",
			expectedCount: 0,
		},
		{
			name:          "enabled=true",
			queryParams:   "enabled=true",
			expectedCount: 1,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				assert.Equal(t, data.FilterEnabledWallets, filters[0].Key)
				assert.Equal(t, true, filters[0].Value)
			},
		},
		{
			name:          "enabled=false",
			queryParams:   "enabled=false",
			expectedCount: 1,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				assert.Equal(t, data.FilterEnabledWallets, filters[0].Key)
				assert.Equal(t, false, filters[0].Value)
			},
		},
		{
			name:          "user_managed=true",
			queryParams:   "user_managed=true",
			expectedCount: 1,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				assert.Equal(t, data.FilterUserManaged, filters[0].Key)
				assert.Equal(t, true, filters[0].Value)
			},
		},
		{
			name:          "supported_assets single asset code",
			queryParams:   "supported_assets=USDC",
			expectedCount: 1,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filters[0].Key)
				assets, ok := filters[0].Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC"}, assets)
			},
		},
		{
			name:          "supported_assets multiple asset codes",
			queryParams:   "supported_assets=USDC,XLM",
			expectedCount: 1,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filters[0].Key)
				assets, ok := filters[0].Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC", "XLM"}, assets)
			},
		},
		{
			name:          "supported_assets with asset ID",
			queryParams:   fmt.Sprintf("supported_assets=%s", assetUSDC.ID),
			expectedCount: 1,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filters[0].Key)
				assets, ok := filters[0].Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{assetUSDC.ID}, assets)
			},
		},
		{
			name:          "supported_assets with whitespace",
			queryParams:   "supported_assets= USDC , XLM ",
			expectedCount: 1,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filters[0].Key)
				assets, ok := filters[0].Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC", "XLM"}, assets)
			},
		},
		{
			name:          "multiple filters combined",
			queryParams:   "enabled=true&user_managed=false&supported_assets=USDC",
			expectedCount: 3,
			validateFilter: func(t *testing.T, filters []data.Filter) {
				// Check that all filters are present
				filterKeys := make([]data.FilterKey, len(filters))
				for i, filter := range filters {
					filterKeys[i] = filter.Key
				}
				assert.Contains(t, filterKeys, data.FilterEnabledWallets)
				assert.Contains(t, filterKeys, data.FilterUserManaged)
				assert.Contains(t, filterKeys, data.FilterSupportedAssets)
			},
		},
		{
			name:          "supported_assets empty string (should be ignored)",
			queryParams:   "supported_assets=",
			expectedCount: 0,
		},
		{
			name:          "invalid enabled parameter",
			queryParams:   "enabled=invalid",
			expectedError: "invalid 'enabled' parameter value",
		},
		{
			name:          "invalid user_managed parameter",
			queryParams:   "user_managed=xyz",
			expectedError: "invalid 'user_managed' parameter value",
		},
		{
			name:          "supported_assets with non-existent asset",
			queryParams:   "supported_assets=NONEXISTENT",
			expectedError: "parsing supported_assets parameter: invalid asset reference in supported_assets: asset 'NONEXISTENT' not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(ctx, "GET", "/wallets?"+tc.queryParams, nil)
			require.NoError(t, err)

			filters, err := handler.parseFilters(ctx, req)

			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Len(t, filters, tc.expectedCount)

			if tc.validateFilter != nil {
				tc.validateFilter(t, filters)
			}
		})
	}
}

func Test_WalletsHandler_parseSupportedAssetsParam(t *testing.T) {
	models := data.SetupModels(t)
	dbPool := models.DBConnectionPool
	ctx := context.Background()

	handler := &WalletsHandler{
		Models: models,
	}

	// Create test assets
	data.DeleteAllAssetFixtures(t, ctx, dbPool)
	assetUSDC := data.CreateAssetFixture(t, ctx, dbPool, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")
	assetXLM := data.CreateAssetFixture(t, ctx, dbPool, "XLM", "")

	testCases := []struct {
		name           string
		input          string
		expectedError  string
		validateResult func(t *testing.T, filter data.Filter)
	}{
		{
			name:  "empty string returns empty filter",
			input: "",
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterKey(""), filter.Key)
				assert.Nil(t, filter.Value)
			},
		},
		{
			name:  "single asset code",
			input: "USDC",
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filter.Key)
				assets, ok := filter.Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC"}, assets)
			},
		},
		{
			name:  "multiple asset codes",
			input: "USDC,XLM",
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filter.Key)
				assets, ok := filter.Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC", "XLM"}, assets)
			},
		},
		{
			name:  "asset ID",
			input: assetUSDC.ID,
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filter.Key)
				assets, ok := filter.Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{assetUSDC.ID}, assets)
			},
		},
		{
			name:  "mixed asset codes and IDs",
			input: fmt.Sprintf("USDC,%s", assetXLM.ID),
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filter.Key)
				assets, ok := filter.Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC", assetXLM.ID}, assets)
			},
		},
		{
			name:  "whitespace handling",
			input: " USDC , XLM ",
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filter.Key)
				assets, ok := filter.Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC", "XLM"}, assets)
			},
		},
		{
			name:  "empty entries filtered out",
			input: "USDC,,XLM,",
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterSupportedAssets, filter.Key)
				assets, ok := filter.Value.([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"USDC", "XLM"}, assets)
			},
		},
		{
			name:  "all empty entries result in empty filter",
			input: ",,",
			validateResult: func(t *testing.T, filter data.Filter) {
				assert.Equal(t, data.FilterKey(""), filter.Key)
				assert.Nil(t, filter.Value)
			},
		},
		{
			name:          "non-existent asset code",
			input:         "NONEXISTENT",
			expectedError: "invalid asset reference in supported_assets: asset 'NONEXISTENT' not found",
		},
		{
			name:          "mixed valid and invalid assets",
			input:         "USDC,INVALID",
			expectedError: "invalid asset reference in supported_assets: asset 'INVALID' not found",
		},
		{
			name:          "too many assets exceeds limit",
			input:         strings.Repeat("USDC,", 21)[:len(strings.Repeat("USDC,", 21))-1], // 21 "USDC" entries
			expectedError: "too many assets specified (max 20)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := handler.parseSupportedAssetsParam(ctx, tc.input)

			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedError)
				return
			}

			assert.NoError(t, err)
			if tc.validateResult != nil {
				tc.validateResult(t, filter)
			}
		})
	}
}
