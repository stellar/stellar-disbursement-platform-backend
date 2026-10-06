package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/base"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/problem"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

var defaultPreconditions = txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)}

func Test_AssetsHandlerGetAssets(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	horizonClientMock := &horizonclient.MockClient{}
	signatureService, _, distAccResolver := signing.NewMockSignatureService(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	mockDistAccService := &mocks.MockDistributionAccountService{}

	handler := &AssetsHandler{
		Models: models,
		SubmitterEngine: engine.SubmitterEngine{
			SignatureService:    signatureService,
			HorizonClient:       horizonClientMock,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          txnbuild.MinBaseFee,
		},
		DistributionAccountService: mockDistAccService,
	}

	t.Run("successfully returns a list of assets", func(t *testing.T) {
		expected := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		expectedJSON, err := json.Marshal(expected)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/assets", nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.JSONEq(t, string(expectedJSON), string(respBody))
	})

	t.Run("successfully returns a list of assets by wallet ID", func(t *testing.T) {
		assets := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		require.Equal(t, 2, len(assets))

		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
		require.NotNil(t, wallet)

		data.AssociateAssetWithWalletFixture(t, ctx, dbConnectionPool, assets[0].ID, wallet.ID)

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", fmt.Sprintf("/assets?wallet=%s", wallet.ID), nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		var assetsResponse []data.Asset
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &assetsResponse))
		require.Len(t, assetsResponse, 1)
		require.Equal(t, assets[0].ID, assetsResponse[0].ID)
		require.Equal(t, assets[0].Code, assetsResponse[0].Code)
		require.Equal(t, assets[0].Issuer, assetsResponse[0].Issuer)
	})

	t.Run("returns assets with trustline information", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		assets := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		require.Equal(t, 2, len(assets))

		tnt := &schema.Tenant{
			ID:                         "test-tenant",
			DistributionAccountType:    schema.DistributionAccountStellarDBVault,
			DistributionAccountAddress: &[]string{"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"}[0],
			DistributionAccountStatus:  schema.AccountStatusActive,
		}
		ctxWithTenant := sdpcontext.SetTenantInContext(ctx, tnt)

		distAccount := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
			Status:  schema.AccountStatusActive,
		}
		distAccResolver.On("DistributionAccountFromContext", mock.Anything).Return(distAccount, nil)

		horizonAccount := &horizon.Account{
			AccountID: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Balances: []horizon.Balance{
				{
					Asset: base.Asset{
						Type: "native",
					},
					Balance: "100.0000000",
				},
				{
					Asset: base.Asset{
						Type:   "credit_alphanum4",
						Code:   "EURT",
						Issuer: "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU",
					},
					Balance: "50.0000000",
				},
			},
		}
		horizonClientMock.On("AccountDetail", mock.Anything).Return(*horizonAccount, nil)

		// Mock DistributionAccountService to return success only for EURT (indicating trustline exists)
		mockDistAccService.On("GetBalance", mock.Anything, mock.Anything, mock.MatchedBy(func(asset data.Asset) bool {
			return asset.Code == "EURT" && asset.Issuer == "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU"
		})).Return(decimal.NewFromFloat(50.0), nil)
		mockDistAccService.On("GetBalance", mock.Anything, mock.Anything, mock.MatchedBy(func(asset data.Asset) bool {
			return asset.Code == "USDC" && asset.Issuer == "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE"
		})).Return(decimal.Zero, errors.New("asset not found"))

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/assets?enabled=true", nil)
		require.NoError(t, err)
		req = req.WithContext(ctxWithTenant)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		var responseAssets []AssetWithEnabledInfo
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseAssets))
		require.Len(t, responseAssets, 1)

		for _, asset := range responseAssets {
			assert.NotNil(t, asset.Enabled)
			if asset.Code == "EURT" {
				assert.NotNil(t, asset.Balance)
				expectedBalance := decimal.RequireFromString("50.0")
				assert.True(t, expectedBalance.Equal(*asset.Balance), "expected balance 50.0, got %s", asset.Balance.String())
			}
		}
	})

	t.Run("filters assets by enabled=true", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		assets := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		require.Equal(t, 2, len(assets))

		tnt := &schema.Tenant{
			ID:                         "test-tenant",
			DistributionAccountType:    schema.DistributionAccountStellarDBVault,
			DistributionAccountAddress: &[]string{"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"}[0],
			DistributionAccountStatus:  schema.AccountStatusActive,
		}
		ctxWithTenant := sdpcontext.SetTenantInContext(ctx, tnt)

		distAccount := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
			Status:  schema.AccountStatusActive,
		}
		distAccResolver.On("DistributionAccountFromContext", mock.Anything).Return(distAccount, nil)

		horizonAccount := &horizon.Account{
			AccountID: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Balances: []horizon.Balance{
				{
					Asset: base.Asset{
						Type: "native",
					},
					Balance: "100.0000000",
				},
				{
					Asset: base.Asset{
						Type:   "credit_alphanum4",
						Code:   "EURT",
						Issuer: "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU",
					},
					Balance: "50.0000000",
				},
			},
		}
		horizonClientMock.On("AccountDetail", mock.Anything).Return(*horizonAccount, nil)

		// Mock DistributionAccountService to return success only for EURT (indicating trustline exists)
		mockDistAccService.On("GetBalance", mock.Anything, mock.Anything, mock.MatchedBy(func(asset data.Asset) bool {
			return asset.Code == "EURT" && asset.Issuer == "GA62MH5RDXFWAIWHQEFNMO2SVDDCQLWOO3GO36VQB5LHUXL22DQ6IQAU"
		})).Return(decimal.NewFromFloat(50.0), nil)
		mockDistAccService.On("GetBalance", mock.Anything, mock.Anything, mock.MatchedBy(func(asset data.Asset) bool {
			return asset.Code == "USDC" && asset.Issuer == "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE"
		})).Return(decimal.Zero, errors.New("asset not found"))

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/assets?enabled=true", nil)
		require.NoError(t, err)
		req = req.WithContext(ctxWithTenant)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		var responseAssets []AssetWithEnabledInfo
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseAssets))

		for _, asset := range responseAssets {
			assert.True(t, asset.Enabled)
			if asset.Code == "EURT" {
				assert.NotNil(t, asset.Balance)
				expectedBalance := decimal.RequireFromString("50.0")
				assert.True(t, expectedBalance.Equal(*asset.Balance), "expected balance 50.0, got %s", asset.Balance.String())
			}
		}
	})

	t.Run("filters assets by enabled=false", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		assets := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		require.Equal(t, 2, len(assets))

		tnt := &schema.Tenant{
			ID:                         "test-tenant",
			DistributionAccountType:    schema.DistributionAccountStellarDBVault,
			DistributionAccountAddress: &[]string{"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"}[0],
			DistributionAccountStatus:  schema.AccountStatusActive,
		}
		ctxWithTenant := sdpcontext.SetTenantInContext(ctx, tnt)

		distAccount := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
			Status:  schema.AccountStatusActive,
		}
		distAccResolver.On("DistributionAccountFromContext", mock.Anything).Return(distAccount, nil)

		// Mock DistributionAccountService to return error (indicating no trustline)
		mockDistAccService.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(decimal.Zero, errors.New("asset not found"))

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/assets?enabled=false", nil)
		require.NoError(t, err)
		req = req.WithContext(ctxWithTenant)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		var responseAssets []AssetWithEnabledInfo
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseAssets))

		for _, asset := range responseAssets {
			assert.False(t, asset.Enabled)
		}
	})

	t.Run("returns error for invalid enabled parameter", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/assets?enabled=invalid", nil)
		require.NoError(t, err)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("correctly identifies assets with zero balance trustlines", func(t *testing.T) {
		horizonClientMock.ExpectedCalls = nil
		distAccResolver.ExpectedCalls = nil

		data.DeleteAllFixtures(t, ctx, dbConnectionPool)
		assets := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		require.Equal(t, 2, len(assets))

		tnt := &schema.Tenant{
			ID:                         "test-tenant",
			DistributionAccountType:    schema.DistributionAccountStellarDBVault,
			DistributionAccountAddress: &[]string{"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"}[0],
			DistributionAccountStatus:  schema.AccountStatusActive,
		}
		ctxWithTenant := sdpcontext.SetTenantInContext(ctx, tnt)

		distAccount := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
			Status:  schema.AccountStatusActive,
		}
		distAccResolver.On("DistributionAccountFromContext", mock.Anything).Return(distAccount, nil)

		// Mock DistributionAccountService to return a balance (indicating trustline exists)
		mockDistAccService.On("GetBalance", mock.Anything, mock.Anything, mock.Anything).Return(decimal.Zero, nil)

		// Mock Horizon account with USDC trustline but zero balance
		horizonAccount := &horizon.Account{
			AccountID: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Balances: []horizon.Balance{
				{
					Asset: base.Asset{
						Type: "native",
					},
					Balance: "100.0000000",
				},
				{
					Asset: base.Asset{
						Type:   "credit_alphanum4",
						Code:   "USDC",
						Issuer: "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE",
					},
					Balance: "0.0000000",
				},
			},
		}
		horizonClientMock.On("AccountDetail", mock.Anything).Return(*horizonAccount, nil)

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/assets?enabled=true", nil)
		require.NoError(t, err)
		req = req.WithContext(ctxWithTenant)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		var responseAssets []AssetWithEnabledInfo
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseAssets))

		for _, asset := range responseAssets {
			if asset.Code == "USDC" {
				assert.True(t, asset.Enabled)
				assert.NotNil(t, asset.Balance)
				assert.Equal(t, decimal.Zero, *asset.Balance)
			}
		}
	})
}

func Test_AssetsHandlerCheckTrustlineExists(t *testing.T) {
	ctx := context.Background()

	mockDistAccService := &mocks.MockDistributionAccountService{}

	handler := &AssetsHandler{
		DistributionAccountService: mockDistAccService,
	}

	t.Run("returns true and balance for native assets", func(t *testing.T) {
		asset := data.Asset{Code: "XLM", Issuer: ""}
		account := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.NewFromFloat(100.0), nil)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.True(t, hasTrustline)
		assert.NotNil(t, balance)
		assert.Equal(t, decimal.NewFromFloat(100.0), *balance)
	})

	t.Run("returns true for Circle accounts with supported assets", func(t *testing.T) {
		asset := data.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Type: schema.DistributionAccountCircleDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.NewFromFloat(123.45), nil)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.True(t, hasTrustline)
		assert.NotNil(t, balance)
		assert.Equal(t, decimal.NewFromFloat(123.45), *balance)
	})

	t.Run("returns false for Circle accounts with unsupported assets", func(t *testing.T) {
		asset := data.Asset{Code: "BTC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Type: schema.DistributionAccountCircleDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.Zero, services.ErrNoBalanceForAsset)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.False(t, hasTrustline)
		assert.Nil(t, balance)
	})

	t.Run("returns true for Stellar accounts with trustline but zero balance", func(t *testing.T) {
		asset := data.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.Zero, nil)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.True(t, hasTrustline)
		assert.NotNil(t, balance)
		assert.Equal(t, decimal.Zero, *balance)
	})

	t.Run("returns false for Stellar accounts without trustline", func(t *testing.T) {
		asset := data.Asset{Code: "BTC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.Zero, services.ErrNoBalanceForAsset)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.False(t, hasTrustline)
		assert.Nil(t, balance)
	})
}

func Test_AssetsHandlerGetBalanceInfo(t *testing.T) {
	ctx := context.Background()

	mockDistAccService := &mocks.MockDistributionAccountService{}

	handler := &AssetsHandler{
		DistributionAccountService: mockDistAccService,
	}

	t.Run("returns true and balance for native assets", func(t *testing.T) {
		asset := data.Asset{Code: "XLM", Issuer: ""}
		account := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.Zero, nil)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.True(t, hasTrustline)
		assert.NotNil(t, balance)
		assert.Equal(t, decimal.Zero, *balance)
	})

	t.Run("returns true and balance for Circle accounts with supported assets", func(t *testing.T) {
		asset := data.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Type: schema.DistributionAccountCircleDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.NewFromFloat(321.0), nil)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.True(t, hasTrustline)
		assert.NotNil(t, balance)
		assert.Equal(t, decimal.NewFromFloat(321.0), *balance)
	})

	t.Run("returns false and nil balance for Circle accounts with unsupported assets", func(t *testing.T) {
		asset := data.Asset{Code: "BTC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Type: schema.DistributionAccountCircleDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.Zero, services.ErrNoBalanceForAsset)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.False(t, hasTrustline)
		assert.Nil(t, balance)
	})

	t.Run("returns true and balance for Stellar accounts with trustline", func(t *testing.T) {
		asset := data.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
		}

		expectedBalance := decimal.NewFromFloat(100.5)
		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(expectedBalance, nil)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.True(t, hasTrustline)
		assert.NotNil(t, balance)
		assert.Equal(t, expectedBalance, *balance)
	})

	t.Run("returns false and nil balance for Stellar accounts without trustline", func(t *testing.T) {
		asset := data.Asset{Code: "BTC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"}
		account := schema.TransactionAccount{
			Address: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Type:    schema.DistributionAccountStellarDBVault,
		}

		mockDistAccService.On("GetBalance", ctx, &account, asset).Return(decimal.Zero, services.ErrNoBalanceForAsset)

		hasTrustline, balance, err := handler.getBalanceInfo(ctx, &account, asset)
		require.NoError(t, err)
		assert.False(t, hasTrustline)
		assert.Nil(t, balance)
	})
}

func Test_AssetHandler_CreateAsset(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	model, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	distributionKP := keypair.MustRandom()
	distAccount := schema.NewDefaultStellarTransactionAccount(distributionKP.Address())
	horizonClientMock := &horizonclient.MockClient{}
	signatureService, sigRouter, distAccResolver := signing.NewMockSignatureService(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	handler := &AssetsHandler{
		Models: model,
		SubmitterEngine: engine.SubmitterEngine{
			SignatureService:    signatureService,
			HorizonClient:       horizonClientMock,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          200,
		},
		GetPreconditionsFn: func() txnbuild.Preconditions { return defaultPreconditions },
	}

	code := "USDT"
	issuer := "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ"
	ctx := context.Background()

	t.Run("failed to get distribution account", func(t *testing.T) {
		distAccResolver.On("DistributionAccountFromContext", ctx).
			Return(schema.TransactionAccount{}, errors.New("foobar")).Once()

		rr := httptest.NewRecorder()
		requestBody, err := json.Marshal(AssetRequest{code, issuer})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
		assert.Contains(t, rr.Body.String(), "Cannot resolve distribution account from context")
	})

	t.Run("cannot process request if distribution account is not a native-Stellar account", func(t *testing.T) {
		distAccResolver.On("DistributionAccountFromContext", ctx).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).Once()

		rr := httptest.NewRecorder()
		requestBody, err := json.Marshal(AssetRequest{code, issuer})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
		assert.Contains(t, rr.Body.String(), "Distribution account affiliated with tenant is not a Stellar account")
	})

	distAccResolver.
		On("DistributionAccountFromContext", ctx).
		Return(distAccount, nil)

	t.Run("successfully create an asset", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   code,
								Issuer: issuer,
							},
						},
						Limit:         "", // no limit
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       200,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", mock.Anything, tx, distAccount).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances:  []horizon.Balance{},
			}, nil).
			Once().
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Once()

		rr := httptest.NewRecorder()

		requestBody, err := json.Marshal(AssetRequest{code, issuer})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		entries := getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, "adding trustline for asset USDT:GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ", entries[0].Message)
	})

	t.Run("successfully create the native asset", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "10000",
						Asset: base.Asset{
							Type: "native",
							Code: "XLM",
						},
					},
				},
			}, nil).
			Once()

		rr := httptest.NewRecorder()

		requestBody, err := json.Marshal(AssetRequest{Code: "XLM"})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		entries := getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, "not performing either add or remove trustline", entries[0].Message)
	})

	t.Run("successfully create an asset with a trustline already set", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Code:   code,
							Issuer: issuer,
						},
					},
				},
			}, nil).
			Once()

		rr := httptest.NewRecorder()

		requestBody, err := json.Marshal(AssetRequest{code, issuer})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		entries := getEntries()
		assert.Len(t, entries, 2)
		assert.Equal(t, "not adding trustline for the asset USDT:GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ because it already exists", entries[0].Message)
	})

	t.Run("failed creating asset, issuer invalid", func(t *testing.T) {
		rr := httptest.NewRecorder()

		requestBody, err := json.Marshal(AssetRequest{code, "invalid"})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("failed creating asset, missing field", func(t *testing.T) {
		rr := httptest.NewRecorder()

		requestBody, err := json.Marshal(AssetRequest{})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("failed creating asset, empty fields", func(t *testing.T) {
		rr := httptest.NewRecorder()

		emptyStr := ""
		requestBody, err := json.Marshal(AssetRequest{Code: emptyStr, Issuer: emptyStr})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("asset creation is idempotent", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   code,
								Issuer: issuer,
							},
						},
						Limit:         "", // no limit
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       200,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", mock.Anything, tx, distAccount).
			Return(signedTx, nil).
			Twice()

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances:  []horizon.Balance{},
			}, nil).
			Twice().
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Twice()

		// Creating the asset
		requestBody, err := json.Marshal(AssetRequest{Code: code, Issuer: issuer})
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/assets", bytes.NewReader(requestBody))
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Duplicating the asset
		requestBody, err = json.Marshal(AssetRequest{Code: code, Issuer: issuer})
		require.NoError(t, err)
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, "/assets", bytes.NewReader(requestBody))
		require.NoError(t, err)
		rr = httptest.NewRecorder()
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp = rr.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("failed creating asset, error adding asset trustline", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   code,
								Issuer: issuer,
							},
						},
						Limit:         "", // no limit
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       200,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", mock.Anything, tx, distAccount).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances:  []horizon.Balance{},
			}, nil).
			Once().
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, horizonclient.Error{
				Response: &http.Response{
					StatusCode: http.StatusBadRequest,
				},
				Problem: problem.P{
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_failed",
							"operations":  []string{"op_no_issuer"},
						},
					},
				},
			}).
			Once()

		// Creating the asset
		requestBody, err := json.Marshal(AssetRequest{Code: code, Issuer: issuer})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/assets", bytes.NewReader(requestBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot create new asset"}`, string(respBody))
	})

	t.Run("ensures that issuers public key value has spaces trimmed", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Code:   code,
							Issuer: issuer,
						},
					},
				},
			}, nil).
			Once()

		rr := httptest.NewRecorder()

		requestBody, err := json.Marshal(AssetRequest{code, fmt.Sprintf(" %s ", issuer)})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		entries := getEntries()
		assert.Len(t, entries, 2)
		assert.Equal(t, "not adding trustline for the asset USDT:GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ because it already exists", entries[0].Message)
	})

	horizonClientMock.AssertExpectations(t)
}

func Test_AssetHandler_DeleteAsset(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	distributionKP := keypair.MustRandom()
	distAccount := schema.NewDefaultStellarTransactionAccount(distributionKP.Address())
	horizonClientMock := &horizonclient.MockClient{}
	signatureService, sigRouter, distAccResolver := signing.NewMockSignatureService(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	handler := &AssetsHandler{
		Models: model,
		SubmitterEngine: engine.SubmitterEngine{
			SignatureService:    signatureService,
			HorizonClient:       horizonClientMock,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          150,
		},
		GetPreconditionsFn: func() txnbuild.Preconditions { return defaultPreconditions },
	}

	r := chi.NewRouter()
	r.Delete("/assets/{id}", handler.DeleteAsset)

	t.Run("failed to get distribution account", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "ABC", "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ")

		distAccResolver.On("DistributionAccountFromContext", mock.AnythingOfType("*context.valueCtx")).
			Return(schema.TransactionAccount{}, errors.New("foobar")).Once()

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", asset.ID), nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
		assert.Contains(t, rr.Body.String(), "Cannot resolve distribution account from context")
	})

	t.Run("cannot process request if distribution account is not a native-Stellar account", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "ABC", "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ")

		distAccResolver.On("DistributionAccountFromContext", mock.AnythingOfType("*context.valueCtx")).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).Once()

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", asset.ID), nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
		assert.Contains(t, rr.Body.String(), "Distribution account affiliated with tenant is not a Stellar account")
	})

	distAccResolver.
		On("DistributionAccountFromContext", mock.AnythingOfType("*context.valueCtx")).
		Return(distAccount, nil)

	t.Run("successfully delete an asset and remove the trustline", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "ABC", "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ")

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   asset.Code,
								Issuer: asset.Issuer,
							},
						},
						Limit:         "0",
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       150,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", mock.Anything, tx, distAccount).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				Sequence: 123,
				Balances: []horizon.Balance{
					{
						Balance: "0",
						Asset: base.Asset{
							Code:   asset.Code,
							Issuer: asset.Issuer,
						},
					},
				},
			}, nil).
			Once().
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Once()

		rr := httptest.NewRecorder()

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", asset.ID), nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assetDB, err := model.Assets.Get(ctx, asset.ID)
		require.NoError(t, err)
		assert.NotNil(t, assetDB.DeletedAt)

		entries := getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, "removing trustline for asset ABC:GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ", entries[0].Message)
	})

	// We decided to not have a mismatch between the Network and the Database. So, if the trustline is not removed,
	// the asset won't be deleted as well.
	t.Run("doesn't remove the asset when couldn't remove the trustline", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "ABC", "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ")

		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				Sequence: 123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Code:   asset.Code,
							Issuer: asset.Issuer,
						},
					},
				},
			}, nil).
			Once()

		rr := httptest.NewRecorder()

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", asset.ID), nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Could not remove trustline because distribution account still has balance"}`, string(respBody))

		// Asset should not be soft deleted.
		assetDB, err := model.Assets.Get(ctx, asset.ID)
		require.NoError(t, err)
		assert.Nil(t, assetDB.DeletedAt)

		entries := getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, "not removing trustline for the asset ABC:GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ because the distribution account still has balance: 100.0000000 ABC", entries[0].Message)
	})

	t.Run("returns error when an error occurs removing trustline", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "ABC", "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ")

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{}, horizonclient.Error{
				Response: &http.Response{
					StatusCode: http.StatusBadRequest,
				},
				Problem: problem.P{
					Title:  "Error occurred",
					Status: http.StatusBadRequest,
				},
			}).
			Once()

		rr := httptest.NewRecorder()

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", asset.ID), nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Cannot delete asset"}`, string(respBody))

		// Asset should not be soft deleted.
		assetDB, err := model.Assets.Get(ctx, asset.ID)
		require.NoError(t, err)
		assert.Nil(t, assetDB.DeletedAt)
	})

	t.Run("failed deleting an asset, asset not found", func(t *testing.T) {
		rr := httptest.NewRecorder()

		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", "nonexistant"), nil)
		require.NoError(t, err)
		r.ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	horizonClientMock.AssertExpectations(t)
}

func Test_AssetHandler_handleUpdateAssetTrustlineForDistributionAccount(t *testing.T) {
	distributionKP := keypair.MustRandom()
	distAccount := schema.NewDefaultStellarTransactionAccount(distributionKP.Address())
	horizonClientMock := &horizonclient.MockClient{}
	signatureService, sigRouter, _ := signing.NewMockSignatureService(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	handler := &AssetsHandler{
		SubmitterEngine: engine.SubmitterEngine{
			SignatureService:    signatureService,
			HorizonClient:       horizonClientMock,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          300,
		},
		GetPreconditionsFn: func() txnbuild.Preconditions { return defaultPreconditions },
	}

	assetToAddTrustline := &txnbuild.CreditAsset{
		Code:   "USDC",
		Issuer: "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ",
	}

	assetToRemoveTrustline := &txnbuild.CreditAsset{
		Code:   "USDT",
		Issuer: "GA24LJXFG73JGARIBG2GP6V5TNUUOS6BD23KOFCW3INLDY5KPKS7GACZ",
	}

	ctx := context.Background()

	t.Run("returns error if distribution account is not a native Stellar account", func(t *testing.T) {
		err := handler.handleUpdateAssetTrustlineForDistributionAccount(
			ctx, nil, nil, schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault})
		assert.EqualError(t, err, "distribution account is not a native Stellar account")
	})

	t.Run("returns error if no asset is provided", func(t *testing.T) {
		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, nil, nil, distAccount)
		assert.EqualError(t, err, "should provide at least one asset")
	})

	t.Run("returns error if the assets are the same", func(t *testing.T) {
		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToRemoveTrustline, assetToRemoveTrustline, distAccount)
		assert.EqualError(t, err, "should provide different assets")
	})

	t.Run("returns error if fails getting distribution account details", func(t *testing.T) {
		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{}, horizonclient.Error{
				Response: &http.Response{
					StatusCode: http.StatusBadRequest,
				},
				Problem: problem.P{
					Title:  "Error occurred",
					Status: http.StatusBadRequest,
				},
			}).
			Once()

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline, distAccount)
		assert.EqualError(t, err, "getting distribution account details: horizon error: \"Error occurred\" - check horizon.Error.Problem for more information")
	})

	t.Run("returns error if fails submitting change trust transaction", func(t *testing.T) {
		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   assetToRemoveTrustline.Code,
								Issuer: assetToRemoveTrustline.Issuer,
							},
						},
						Limit:         "0",
						SourceAccount: distributionKP.Address(),
					},
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   assetToAddTrustline.Code,
								Issuer: assetToAddTrustline.Issuer,
							},
						},
						Limit:         "",
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       300,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", ctx, tx, distAccount).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   "XLM",
							Issuer: "",
						},
					},
					{
						Balance: "0",
						Asset: base.Asset{
							Type:   "",
							Code:   assetToRemoveTrustline.Code,
							Issuer: assetToRemoveTrustline.Issuer,
						},
					},
				},
			}, nil).
			Once().
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, horizonclient.Error{
				Response: &http.Response{
					StatusCode: http.StatusBadRequest,
				},
				Problem: problem.P{
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_failed",
							"operations":  []string{"op_no_issuer"},
						},
					},
				},
			}).
			Once()

		err = handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline, distAccount)
		assert.EqualError(t, err, "submitting change trust transaction: submitting change trust transaction to network: horizon response error: StatusCode=0, Extras=transaction: tx_failed - operation codes: [ op_no_issuer ]")
	})

	t.Run("adds and removes the trustlines successfully", func(t *testing.T) {
		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   assetToRemoveTrustline.Code,
								Issuer: assetToRemoveTrustline.Issuer,
							},
						},
						Limit:         "0",
						SourceAccount: distributionKP.Address(),
					},
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   assetToAddTrustline.Code,
								Issuer: assetToAddTrustline.Issuer,
							},
						},
						Limit:         "",
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       300,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", ctx, tx, distAccount).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   "XLM",
							Issuer: "",
						},
					},
					{
						Balance: "0",
						Asset: base.Asset{
							Type:   "",
							Code:   assetToRemoveTrustline.Code,
							Issuer: assetToRemoveTrustline.Issuer,
						},
					},
				},
			}, nil).
			Once().
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Once()

		err = handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline, distAccount)
		assert.NoError(t, err)
	})

	t.Run("doesn't remove the trustline in case still has balance", func(t *testing.T) {
		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   "XLM",
							Issuer: "",
						},
					},
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   assetToRemoveTrustline.Code,
							Issuer: assetToRemoveTrustline.Issuer,
						},
					},
				},
			}, nil).
			Once()

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline, distAccount)
		assert.EqualError(t, err, errCouldNotRemoveTrustline.Error())
	})

	t.Run("doesn't remove the trustline in case it's already removed", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   "XLM",
							Issuer: "",
						},
					},
				},
			}, nil).
			Once()

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, nil, assetToRemoveTrustline, distAccount)
		assert.NoError(t, err)

		entries := getEntries()
		assert.Len(t, entries, 2)
		assert.Equal(t, "not removing trustline for the asset USDT:GA24LJXFG73JGARIBG2GP6V5TNUUOS6BD23KOFCW3INLDY5KPKS7GACZ because it could not be found on the blockchain", entries[0].Message)
	})

	t.Run("doesn't add new trustline if distribution account already have trustline for the asset", func(t *testing.T) {
		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   "XLM",
							Issuer: "",
						},
					},
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   assetToAddTrustline.Code,
							Issuer: assetToAddTrustline.Issuer,
						},
					},
				},
			}, nil).
			Once()

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, nil, distAccount)
		assert.NoError(t, err)
	})

	t.Run("does not perform either add or remove for the native asset", func(t *testing.T) {
		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{
				AccountID: distributionKP.Address(),
			}).
			Return(horizon.Account{
				AccountID: distributionKP.Address(),
				Sequence:  123,
				Balances: []horizon.Balance{
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   "XLM",
							Issuer: "",
						},
					},
					{
						Balance: "100",
						Asset: base.Asset{
							Type:   "",
							Code:   assetToAddTrustline.Code,
							Issuer: assetToAddTrustline.Issuer,
						},
					},
				},
			}, nil).
			Twice()

		nativeAsset := &txnbuild.CreditAsset{
			Code:   "XLM",
			Issuer: "",
		}

		// add trustline
		getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, nativeAsset, nil, distAccount)
		require.NoError(t, err)

		entries := getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, "not performing either add or remove trustline", entries[0].Message)

		// remove trustline
		getEntries = log.DefaultLogger.StartTest(log.WarnLevel)

		err = handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, nil, nativeAsset, distAccount)
		require.NoError(t, err)

		entries = getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, "not performing either add or remove trustline", entries[0].Message)
	})

	horizonClientMock.AssertExpectations(t)
}

func Test_AssetHandler_submitChangeTrustTransaction(t *testing.T) {
	distributionKP := keypair.MustRandom()
	distAccount := schema.NewDefaultStellarTransactionAccount(distributionKP.Address())
	horizonClientMock := &horizonclient.MockClient{}
	signatureService, sigRouter, _ := signing.NewMockSignatureService(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	handler := &AssetsHandler{
		SubmitterEngine: engine.SubmitterEngine{
			SignatureService:    signatureService,
			HorizonClient:       horizonClientMock,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          txnbuild.MinBaseFee,
		},
		GetPreconditionsFn: func() txnbuild.Preconditions { return defaultPreconditions },
	}

	code := "USDC"
	issuer := "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ"

	acc := &horizon.Account{
		AccountID: distributionKP.Address(),
		Sequence:  123,
		Balances: []horizon.Balance{
			{
				Balance: "100",
				Asset: base.Asset{
					Type:   "",
					Code:   "XLM",
					Issuer: "",
				},
			},
			{
				Balance: "100",
				Asset: base.Asset{
					Type:   "",
					Code:   code,
					Issuer: issuer,
				},
			},
		},
	}

	ctx := context.Background()

	t.Run("returns error if no change trust operations is passed", func(t *testing.T) {
		err := handler.submitChangeTrustTransaction(ctx, acc, []*txnbuild.ChangeTrust{}, distAccount)
		assert.EqualError(t, err, "should have at least one change trust operation")
	})

	t.Run("returns error when fails signing transaction", func(t *testing.T) {
		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   code,
								Issuer: issuer,
							},
						},
						Limit:         "",
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       txnbuild.MinBaseFee,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", ctx, tx, distAccount).
			Return(nil, errors.New("unexpected error")).
			Once()

		err = handler.submitChangeTrustTransaction(ctx, acc, []*txnbuild.ChangeTrust{
			{
				Line: txnbuild.ChangeTrustAssetWrapper{
					Asset: txnbuild.CreditAsset{
						Code:   code,
						Issuer: issuer,
					},
				},
				Limit:         "",
				SourceAccount: distributionKP.Address(),
			},
		}, distAccount)
		assert.EqualError(t, err, "signing change trust transaction: unexpected error")
	})

	t.Run("returns error if fails submitting change trust transaction", func(t *testing.T) {
		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   code,
								Issuer: issuer,
							},
						},
						Limit:         "",
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       txnbuild.MinBaseFee,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", ctx, tx, distAccount).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, horizonclient.Error{
				Response: &http.Response{
					StatusCode: http.StatusBadRequest,
				},
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_failed",
							"operations":  []string{"op_no_issuer"},
						},
					},
				},
			}).
			Once()

		err = handler.submitChangeTrustTransaction(ctx, acc, []*txnbuild.ChangeTrust{
			{
				Line: txnbuild.ChangeTrustAssetWrapper{
					Asset: txnbuild.CreditAsset{
						Code:   code,
						Issuer: issuer,
					},
				},
				Limit:         "",
				SourceAccount: distributionKP.Address(),
			},
		}, distAccount)
		assert.EqualError(t, err, "submitting change trust transaction to network: horizon response error: StatusCode=400, Extras=transaction: tx_failed - operation codes: [ op_no_issuer ]")
	})

	t.Run("submits transaction correctly", func(t *testing.T) {
		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount: &txnbuild.SimpleAccount{
					AccountID: distributionKP.Address(),
					Sequence:  124,
				},
				IncrementSequenceNum: false,
				Operations: []txnbuild.Operation{
					&txnbuild.ChangeTrust{
						Line: txnbuild.ChangeTrustAssetWrapper{
							Asset: txnbuild.CreditAsset{
								Code:   code,
								Issuer: issuer,
							},
						},
						Limit:         "",
						SourceAccount: distributionKP.Address(),
					},
				},
				BaseFee:       txnbuild.MinBaseFee,
				Preconditions: defaultPreconditions,
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		sigRouter.
			On("SignStellarTransaction", ctx, tx, distAccount).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Once()

		err = handler.submitChangeTrustTransaction(ctx, acc, []*txnbuild.ChangeTrust{
			{
				Line: txnbuild.ChangeTrustAssetWrapper{
					Asset: txnbuild.CreditAsset{
						Code:   code,
						Issuer: issuer,
					},
				},
				Limit:         "",
				SourceAccount: distributionKP.Address(),
			},
		}, distAccount)
		assert.NoError(t, err)
	})

	horizonClientMock.AssertExpectations(t)
}

type assetTestMock struct {
	SignatureService  signing.SignatureService
	SignatureRouter   *sigMocks.MockSignerRouter
	HorizonClientMock *horizonclient.MockClient
	Handler           AssetsHandler
}

func newAssetTestMock(t *testing.T) *assetTestMock {
	t.Helper()

	horizonClientMock := &horizonclient.MockClient{}
	signatureService, sigRouter, _ := signing.NewMockSignatureService(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	return &assetTestMock{
		SignatureService:  signatureService,
		SignatureRouter:   sigRouter,
		HorizonClientMock: horizonClientMock,
		Handler: AssetsHandler{
			SubmitterEngine: engine.SubmitterEngine{
				SignatureService:    signatureService,
				HorizonClient:       horizonClientMock,
				LedgerNumberTracker: mLedgerNumberTracker,
				MaxBaseFee:          txnbuild.MinBaseFee,
			},
		},
	}
}

func Test_AssetHandler_submitChangeTrustTransaction_makeSurePreconditionsAreSetAsExpected(t *testing.T) {
	ctx := context.Background()
	distributionKP := keypair.MustRandom()
	distAccount := schema.NewDefaultStellarTransactionAccount(distributionKP.Address())

	const code = "USDC"
	const issuer = "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ"
	acc := &horizon.Account{}
	changeTrustOp := &txnbuild.ChangeTrust{
		Line: txnbuild.ChangeTrustAssetWrapper{
			Asset: txnbuild.CreditAsset{
				Code:   code,
				Issuer: issuer,
			},
		},
		Limit:         "",
		SourceAccount: distributionKP.Address(),
	}
	txParamsWithoutPreconditions := txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: distributionKP.Address(),
			Sequence:  124,
		},
		IncrementSequenceNum: false,
		Operations: []txnbuild.Operation{
			&txnbuild.ChangeTrust{
				Line: txnbuild.ChangeTrustAssetWrapper{
					Asset: txnbuild.CreditAsset{
						Code:   code,
						Issuer: issuer,
					},
				},
				Limit:         "",
				SourceAccount: distributionKP.Address(),
			},
		},
		BaseFee: txnbuild.MinBaseFee,
	}

	t.Run("makes sure a non-empty precondition is used if none is explicitly set", func(t *testing.T) {
		mocks := newAssetTestMock(t)
		mocks.Handler.GetPreconditionsFn = nil

		txParams := txParamsWithoutPreconditions
		txParams.Preconditions = txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)}
		tx, err := txnbuild.NewTransaction(txParams)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		mocks.SignatureRouter.
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), distAccount).
			Run(func(t *testing.T, expectedTx *txnbuild.Transaction, actualTxIndex int) func(args mock.Arguments) {
				return func(args mock.Arguments) {
					actualTx, ok := args.Get(actualTxIndex).(*txnbuild.Transaction)
					require.True(t, ok, actualTxIndex)

					expXDR := expectedTx.ToXDR().Preconditions().TimeBounds
					actXDR := actualTx.ToXDR().Preconditions().TimeBounds

					require.Equal(t, expXDR.MinTime, actXDR.MinTime)

					expectedMax := time.Unix(int64(expXDR.MaxTime), 0).UTC()
					actualMax := time.Unix(int64(actXDR.MaxTime), 0).UTC()

					require.WithinDuration(t, expectedMax, actualMax, 30*time.Second,
						"MaxTime bounds drift too far: expected %s, got %s", expectedMax, actualMax)
				}
			}(t, signedTx, 1)).
			Return(signedTx, nil).
			Once()

		mocks.HorizonClientMock.
			On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Run(func(t *testing.T, expectedTx *txnbuild.Transaction, actualTxIndex int) func(args mock.Arguments) {
				return func(args mock.Arguments) {
					actualTx, ok := args.Get(actualTxIndex).(*txnbuild.Transaction)
					require.True(t, ok, actualTxIndex)

					expXDR := expectedTx.ToXDR().Preconditions().TimeBounds
					actXDR := actualTx.ToXDR().Preconditions().TimeBounds

					require.Equal(t, expXDR.MinTime, actXDR.MinTime)

					expectedMax := time.Unix(int64(expXDR.MaxTime), 0).UTC()
					actualMax := time.Unix(int64(actXDR.MaxTime), 0).UTC()

					require.WithinDuration(t, expectedMax, actualMax, 60*time.Second,
						"MaxTime bounds drift too far: expected %s, got %s", expectedMax, actualMax)
				}
			}(t, signedTx, 0)).
			Return(horizon.Transaction{}, nil).
			Once()
		defer mocks.HorizonClientMock.AssertExpectations(t)

		err = mocks.Handler.submitChangeTrustTransaction(ctx, acc, []*txnbuild.ChangeTrust{changeTrustOp}, distAccount)
		assert.NoError(t, err)
	})

	t.Run("makes sure a the precondition that was set is used", func(t *testing.T) {
		mocks := newAssetTestMock(t)
		newPreconditions := txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(int64(rand.Intn(999999999)))}
		mocks.Handler.GetPreconditionsFn = func() txnbuild.Preconditions { return newPreconditions }

		txParams := txParamsWithoutPreconditions
		txParams.Preconditions = newPreconditions
		tx, err := txnbuild.NewTransaction(txParams)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		mocks.SignatureRouter.
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), distAccount).
			Run(func(t *testing.T, expectedTx *txnbuild.Transaction, actualTxIndex int) func(args mock.Arguments) {
				return func(args mock.Arguments) {
					actualTx, ok := args.Get(actualTxIndex).(*txnbuild.Transaction)
					require.True(t, ok, actualTxIndex)

					expXDR := expectedTx.ToXDR().Preconditions().TimeBounds
					actXDR := actualTx.ToXDR().Preconditions().TimeBounds

					require.Equal(t, expXDR.MinTime, actXDR.MinTime)

					expectedMax := time.Unix(int64(expXDR.MaxTime), 0).UTC()
					actualMax := time.Unix(int64(actXDR.MaxTime), 0).UTC()

					require.WithinDuration(t, expectedMax, actualMax, 30*time.Second,
						"MaxTime bounds drift too far: expected %s, got %s", expectedMax, actualMax)
				}
			}(t, signedTx, 1)).
			Return(signedTx, nil).
			Once()

		mocks.HorizonClientMock.
			On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Run(func(t *testing.T, expectedTx *txnbuild.Transaction, actualTxIndex int) func(args mock.Arguments) {
				return func(args mock.Arguments) {
					actualTx, ok := args.Get(actualTxIndex).(*txnbuild.Transaction)
					require.True(t, ok, actualTxIndex)

					expXDR := expectedTx.ToXDR().Preconditions().TimeBounds
					actXDR := actualTx.ToXDR().Preconditions().TimeBounds

					require.Equal(t, expXDR.MinTime, actXDR.MinTime)

					expectedMax := time.Unix(int64(expXDR.MaxTime), 0).UTC()
					actualMax := time.Unix(int64(actXDR.MaxTime), 0).UTC()

					require.WithinDuration(t, expectedMax, actualMax, 30*time.Second,
						"MaxTime bounds drift too far: expected %s, got %s", expectedMax, actualMax)
				}
			}(t, signedTx, 0)).
			Once()
		defer mocks.HorizonClientMock.AssertExpectations(t)

		err = mocks.Handler.submitChangeTrustTransaction(ctx, acc, []*txnbuild.ChangeTrust{changeTrustOp}, distAccount)
		assert.NoError(t, err)
	})
}

// Test_AssetsHandler_XWalletIDRouting pins the trustline-remediation fix: every assets endpoint
// that touches an account must act on the wallet named by X-Wallet-Id, not on the tenant default.
//
// Before this, CreateAsset/DeleteAsset/GetAssets(enabled) all resolved through
// DistributionAccountFromContext and issued their ChangeTrust against the tenant default account
// only — so a secondary wallet (which does not inherit the default's trustlines, and re-derives
// its own from the enabled wallet providers) could never acquire a trustline after creation by
// any route. The tenant-default behaviour when the header is absent is pinned here too: it is the
// pre-multi-wallet path and must not shift.
func Test_AssetsHandler_XWalletIDRouting(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Tenant default wallet (the only account the old code could ever reach) + a second wallet.
	defaultKP := keypair.MustRandom()
	defaultWallet := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	_, err = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, defaultWallet.ID, defaultKP.Address())
	require.NoError(t, err)

	walletBKP := keypair.MustRandom()
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type, distribution_account_address)
		VALUES ('assets-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', $1) RETURNING id`, walletBKP.Address()))

	// A wallet still mid-provisioning: reserved row, no on-chain account yet.
	var pendingWalletID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &pendingWalletID, `
		INSERT INTO distribution_wallets (name, distribution_account_type, status)
		VALUES ('assets-wallet-pending', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'PENDING') RETURNING id`))

	walletBAccount := schema.TransactionAccount{
		Address: walletBKP.Address(),
		Type:    schema.DistributionAccountStellarDBVault,
		Status:  schema.AccountStatusActive,
	}
	tenantDefaultAccount := schema.NewDefaultStellarTransactionAccount(defaultKP.Address())

	// fcOnB holds financial_controller globally, plus on wallet B and the pending wallet;
	// fcOnDefault holds it globally and on the default wallet only, so wallet B is outside both
	// its read scope and its write entitlement.
	fcOnB := &auth.User{ID: "assets-fc-b", Email: "fc-b@assets.test", Roles: []string{string(data.FinancialControllerUserRole)}}
	fcOnDefault := &auth.User{ID: "assets-fc-default", Email: "fc-d@assets.test", Roles: []string{string(data.FinancialControllerUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, fcOnB.ID, walletBID, data.FinancialControllerUserRole, nil)
	require.NoError(t, err)
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, fcOnB.ID, pendingWalletID, data.FinancialControllerUserRole, nil)
	require.NoError(t, err)
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, fcOnDefault.ID, defaultWallet.ID, data.FinancialControllerUserRole, nil)
	require.NoError(t, err)

	authManagerMock := &auth.AuthManagerMock{}
	authManagerMock.On("GetUserByID", mock.Anything, fcOnB.ID).Return(fcOnB, nil)
	authManagerMock.On("GetUserByID", mock.Anything, fcOnDefault.ID).Return(fcOnDefault, nil)

	// Each subtest gets its own mock set: an unexpected call on any of them (for example the
	// tenant-default resolver being consulted on a header-routed request) fails that subtest.
	newHandler := func(t *testing.T) (*AssetsHandler, *horizonclient.MockClient, *sigMocks.MockSignerRouter, *sigMocks.MockDistributionAccountResolver, *mocks.MockDistributionAccountService) {
		t.Helper()

		horizonClientMock := &horizonclient.MockClient{}
		signatureService, sigRouter, distAccResolver := signing.NewMockSignatureService(t)
		mockDistAccService := &mocks.MockDistributionAccountService{}

		return &AssetsHandler{
			Models: models,
			SubmitterEngine: engine.SubmitterEngine{
				SignatureService:    signatureService,
				HorizonClient:       horizonClientMock,
				LedgerNumberTracker: preconditionsMocks.NewMockLedgerNumberTracker(t),
				MaxBaseFee:          200,
			},
			DistributionAccountService: mockDistAccService,
			AuthManager:                authManagerMock,
			GetPreconditionsFn:         func() txnbuild.Preconditions { return defaultPreconditions },
		}, horizonClientMock, sigRouter, distAccResolver, mockDistAccService
	}

	// expectChangeTrust wires the horizon + signing mocks for exactly one ChangeTrust submitted
	// from `kp`'s account, and returns nothing: a mismatch on the account is a mock failure, which
	// is the whole point — it is how "the wrong wallet was charged" is detected.
	expectChangeTrust := func(t *testing.T, horizonClientMock *horizonclient.MockClient, sigRouter *sigMocks.MockSignerRouter,
		kp *keypair.Full, account schema.TransactionAccount, balances []horizon.Balance, asset txnbuild.CreditAsset, limit string,
	) {
		t.Helper()

		horizonClientMock.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: kp.Address()}).
			Return(horizon.Account{AccountID: kp.Address(), Sequence: 123, Balances: balances}, nil).
			Once()

		expectedTx, txErr := txnbuild.NewTransaction(txnbuild.TransactionParams{
			SourceAccount:        &txnbuild.SimpleAccount{AccountID: kp.Address(), Sequence: 124},
			IncrementSequenceNum: false,
			Operations: []txnbuild.Operation{&txnbuild.ChangeTrust{
				Line:          txnbuild.ChangeTrustAssetWrapper{Asset: asset},
				Limit:         limit,
				SourceAccount: kp.Address(),
			}},
			BaseFee:       200,
			Preconditions: defaultPreconditions,
		})
		require.NoError(t, txErr)

		signedTx, signErr := expectedTx.Sign(network.TestNetworkPassphrase, kp)
		require.NoError(t, signErr)

		sigRouter.On("SignStellarTransaction", mock.Anything, expectedTx, account).Return(signedTx, nil).Once()
		horizonClientMock.
			On("SubmitTransactionWithOptions", signedTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Once()
	}

	postAsset := func(t *testing.T, handler *AssetsHandler, userID, headerWalletID, code, issuer string) *httptest.ResponseRecorder {
		t.Helper()

		body, mErr := json.Marshal(AssetRequest{Code: code, Issuer: issuer})
		require.NoError(t, mErr)

		req := httptest.NewRequest(http.MethodPost, "/assets", bytes.NewReader(body))
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		if headerWalletID != "" {
			req.Header.Set(XWalletIDHeader, headerWalletID)
		}

		rr := httptest.NewRecorder()
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)
		return rr
	}

	const (
		assetCode   = "USDT"
		assetIssuer = "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ"
	)
	creditAsset := txnbuild.CreditAsset{Code: assetCode, Issuer: assetIssuer}

	t.Run("POST /assets routes the ChangeTrust to the wallet named by X-Wallet-Id", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		handler, horizonClientMock, sigRouter, _, _ := newHandler(t)

		expectChangeTrust(t, horizonClientMock, sigRouter, walletBKP, walletBAccount, []horizon.Balance{}, creditAsset, "")

		rr := postAsset(t, handler, fcOnB.ID, walletBID, assetCode, assetIssuer)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

		horizonClientMock.AssertExpectations(t)

		createdAssets, aErr := models.Assets.GetAll(ctx)
		require.NoError(t, aErr)
		require.Len(t, createdAssets, 1)
		assert.Equal(t, assetCode, createdAssets[0].Code)
	})

	t.Run("POST /assets is idempotent remediation: an existing asset still gets the wallet's trustline", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		existing := data.CreateAssetFixture(t, ctx, dbConnectionPool, assetCode, assetIssuer)
		handler, horizonClientMock, sigRouter, _, _ := newHandler(t)

		expectChangeTrust(t, horizonClientMock, sigRouter, walletBKP, walletBAccount, []horizon.Balance{}, creditAsset, "")

		rr := postAsset(t, handler, fcOnB.ID, walletBID, assetCode, assetIssuer)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		assert.Contains(t, rr.Body.String(), existing.ID, "re-adding an existing asset must reuse the row, not fail")

		horizonClientMock.AssertExpectations(t)
	})

	t.Run("POST /assets without the header keeps using the tenant default account", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		handler, horizonClientMock, sigRouter, distAccResolver, _ := newHandler(t)

		distAccResolver.On("DistributionAccountFromContext", mock.Anything).Return(tenantDefaultAccount, nil).Once()
		expectChangeTrust(t, horizonClientMock, sigRouter, defaultKP, tenantDefaultAccount, []horizon.Balance{}, creditAsset, "")

		// fcOnDefault holds no membership on any other wallet, and the tenant has three wallets:
		// the header-less path must still work, with no membership gate and no "which wallet?" 400.
		rr := postAsset(t, handler, fcOnDefault.ID, "", assetCode, assetIssuer)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

		horizonClientMock.AssertExpectations(t)
	})

	t.Run("POST /assets rejects a wallet the caller holds no membership on, and creates nothing", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		handler, _, _, _, _ := newHandler(t)

		rr := postAsset(t, handler, fcOnDefault.ID, walletBID, assetCode, assetIssuer)
		require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		assert.NotContains(t, rr.Body.String(), "assets-wallet-b", "403 must not disclose wallet details")

		remaining, aErr := models.Assets.GetAll(ctx)
		require.NoError(t, aErr)
		assert.Empty(t, remaining, "a rejected request must not insert the asset")
	})

	t.Run("POST /assets rejects a wallet that is not active yet", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		handler, _, _, _, _ := newHandler(t)

		rr := postAsset(t, handler, fcOnB.ID, pendingWalletID, assetCode, assetIssuer)
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		assert.Contains(t, rr.Body.String(), "not active")
	})

	t.Run("DELETE /assets/{id} removes the trustline from the wallet named by X-Wallet-Id", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, assetCode, assetIssuer)
		handler, horizonClientMock, sigRouter, _, _ := newHandler(t)

		expectChangeTrust(t, horizonClientMock, sigRouter, walletBKP, walletBAccount,
			[]horizon.Balance{{Asset: base.Asset{Type: "credit_alphanum4", Code: assetCode, Issuer: assetIssuer}, Balance: "0.0000000"}},
			creditAsset, "0")

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", asset.ID), nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, fcOnB.ID))
		req.Header.Set(XWalletIDHeader, walletBID)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", asset.ID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

		rr := httptest.NewRecorder()
		http.HandlerFunc(handler.DeleteAsset).ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		horizonClientMock.AssertExpectations(t)
	})

	getAssets := func(t *testing.T, handler *AssetsHandler, userID, headerWalletID, query string) *httptest.ResponseRecorder {
		t.Helper()

		req := httptest.NewRequest(http.MethodGet, "/assets?"+query, nil)
		req = req.WithContext(sdpcontext.SetUserIDInContext(ctx, userID))
		if headerWalletID != "" {
			req.Header.Set(XWalletIDHeader, headerWalletID)
		}

		rr := httptest.NewRecorder()
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)
		return rr
	}

	t.Run("GET /assets?enabled reports the balances of the wallet named by X-Wallet-Id", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, assetCode, assetIssuer)
		handler, _, _, _, mockDistAccService := newHandler(t)

		mockDistAccService.
			On("GetBalance", mock.Anything, mock.MatchedBy(func(account *schema.TransactionAccount) bool {
				return account != nil && account.Address == walletBKP.Address()
			}), mock.Anything).
			Return(decimal.NewFromInt(42), nil).
			Once()

		rr := getAssets(t, handler, fcOnB.ID, walletBID, "enabled=true")
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		var responseAssets []AssetWithEnabledInfo
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseAssets))
		require.Len(t, responseAssets, 1)
		assert.Equal(t, asset.ID, responseAssets[0].ID)
		assert.True(t, responseAssets[0].Enabled)

		mockDistAccService.AssertExpectations(t)
	})

	t.Run("GET /assets?enabled 404s a wallet outside the caller's read scope", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		data.CreateAssetFixture(t, ctx, dbConnectionPool, assetCode, assetIssuer)
		handler, _, _, _, _ := newHandler(t)

		rr := getAssets(t, handler, fcOnDefault.ID, walletBID, "enabled=true")
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})

	t.Run("GET /assets?enabled=false reports every asset for a wallet with no on-chain account yet", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		data.CreateAssetFixture(t, ctx, dbConnectionPool, assetCode, assetIssuer)
		handler, _, _, _, _ := newHandler(t)

		// No GetBalance expectation: an unprovisioned wallet must not be queried at all.
		rr := getAssets(t, handler, fcOnB.ID, pendingWalletID, "enabled=false")
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		var responseAssets []AssetWithEnabledInfo
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseAssets))
		require.Len(t, responseAssets, 1)
		assert.False(t, responseAssets[0].Enabled)
		assert.Nil(t, responseAssets[0].Balance)
	})

	t.Run("GET /assets?enabled without the header keeps using the tenant default account", func(t *testing.T) {
		data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
		data.CreateAssetFixture(t, ctx, dbConnectionPool, assetCode, assetIssuer)
		handler, _, _, distAccResolver, mockDistAccService := newHandler(t)

		distAccResolver.On("DistributionAccountFromContext", mock.Anything).Return(tenantDefaultAccount, nil).Once()
		mockDistAccService.
			On("GetBalance", mock.Anything, mock.MatchedBy(func(account *schema.TransactionAccount) bool {
				return account != nil && account.Address == defaultKP.Address()
			}), mock.Anything).
			Return(decimal.NewFromInt(7), nil).
			Once()

		rr := getAssets(t, handler, fcOnDefault.ID, "", "enabled=true")
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		var responseAssets []AssetWithEnabledInfo
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseAssets))
		require.Len(t, responseAssets, 1)

		mockDistAccService.AssertExpectations(t)
	})
}
