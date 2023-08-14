package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/base"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_AssetsHandlerGetAssets(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	handler := &AssetsHandler{
		Models: models,
	}

	t.Run("successfully returns a list of assets", func(t *testing.T) {
		expected := data.ClearAndCreateAssetFixtures(t, ctx, dbConnectionPool)
		expectedJSON, err := json.Marshal(expected)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/assets", nil)
		http.HandlerFunc(handler.GetAssets).ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.JSONEq(t, string(expectedJSON), string(respBody))
	})
}

func Test_AssetHandlerAddAsset(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	model, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	distributionKP := keypair.MustRandom()
	horizonClientMock := &horizonclient.MockClient{}
	signatureService := mocks.NewMockSignatureService(t)

	handler := &AssetsHandler{
		Models:           model,
		SignatureService: signatureService,
		HorizonClient:    horizonClientMock,
	}

	code := "USDT"
	issuer := "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ"

	signatureService.
		On("DistributionAccount").
		Return(distributionKP.Address()).
		Maybe()

	ctx := context.Background()

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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", mock.Anything, tx, distributionKP.Address()).
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

		requestBody, _ := json.Marshal(AssetRequest{code, issuer})

		req, _ := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		entries := getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, "adding trustline for asset USDT:GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ", entries[0].Message)
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

		requestBody, _ := json.Marshal(AssetRequest{code, issuer})

		req, _ := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		entries := getEntries()
		assert.Len(t, entries, 2)
		assert.Equal(t, "not adding trustline for the asset USDT:GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ because it already exists", entries[0].Message)
	})

	t.Run("failed creating asset, issuer invalid", func(t *testing.T) {
		rr := httptest.NewRecorder()

		requestBody, _ := json.Marshal(AssetRequest{code, "invalid"})

		req, _ := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("failed creating asset, missing field", func(t *testing.T) {
		rr := httptest.NewRecorder()

		requestBody, _ := json.Marshal(AssetRequest{})

		req, _ := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("failed creating asset, empty fields", func(t *testing.T) {
		rr := httptest.NewRecorder()

		emptyStr := ""
		requestBody, _ := json.Marshal(AssetRequest{Code: emptyStr, Issuer: emptyStr})

		req, _ := http.NewRequest(http.MethodPost, "/assets", strings.NewReader(string(requestBody)))
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("failed creating asset, duplicated asset", func(t *testing.T) {
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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", mock.Anything, tx, distributionKP.Address()).
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

		// Creating the asset
		requestBody, err := json.Marshal(AssetRequest{Code: code, Issuer: issuer})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/assets", bytes.NewReader(requestBody))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		http.HandlerFunc(handler.CreateAsset).ServeHTTP(rr, req)

		resp := rr.Result()

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

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.JSONEq(t, `{"error": "asset already exists"}`, string(respBody))
	})

	t.Run("failed creating asset, error adding asset trustline", func(t *testing.T) {
		ctx := context.Background()

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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", mock.Anything, tx, distributionKP.Address()).
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

	horizonClientMock.AssertExpectations(t)
	signatureService.AssertExpectations(t)
}

func Test_AssetHandlerDeleteAsset(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	distributionKP := keypair.MustRandom()
	horizonClientMock := &horizonclient.MockClient{}
	signatureService := mocks.NewMockSignatureService(t)

	handler := &AssetsHandler{
		Models:           model,
		SignatureService: signatureService,
		HorizonClient:    horizonClientMock,
	}

	r := chi.NewRouter()
	r.Delete("/assets/{id}", handler.DeleteAsset)

	signatureService.
		On("DistributionAccount").
		Return(distributionKP.Address()).
		Maybe()

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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", mock.Anything, tx, distributionKP.Address()).
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
		assert.Len(t, entries, 2)
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

		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/assets/%s", "nonexistant"), nil)
		r.ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	horizonClientMock.AssertExpectations(t)
	signatureService.AssertExpectations(t)
}

func Test_AssetHandler_handleUpdateAssetTrustlineForDistributionAccount(t *testing.T) {
	distributionKP := keypair.MustRandom()
	horizonClientMock := &horizonclient.MockClient{}
	signatureService := mocks.NewMockSignatureService(t)

	handler := &AssetsHandler{
		SignatureService: signatureService,
		HorizonClient:    horizonClientMock,
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

	signatureService.
		On("DistributionAccount").
		Return(distributionKP.Address()).
		Maybe()

	t.Run("returns error if no asset is provided", func(t *testing.T) {
		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, nil, nil)
		assert.EqualError(t, err, "should provide at least one asset")
	})

	t.Run("returns error if the assets are the same", func(t *testing.T) {
		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToRemoveTrustline, assetToRemoveTrustline)
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

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline)
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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", ctx, tx, distributionKP.Address()).
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

		err = handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline)
		assert.EqualError(t, err, "submitting change trust transaction: submitting change trust transaction to network: horizon error: \"\" (tx_failed, op_no_issuer) - check horizon.Error.Problem for more information")
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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", ctx, tx, distributionKP.Address()).
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

		err = handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline)
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

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, assetToRemoveTrustline)
		assert.EqualError(t, err, errCouldNotRemoveTrustline.Error())
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

		err := handler.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAddTrustline, nil)
		assert.NoError(t, err)
	})

	horizonClientMock.AssertExpectations(t)
	signatureService.AssertExpectations(t)
}

func Test_AssetHandler_submitChangeTrustTransaction(t *testing.T) {
	distributionKP := keypair.MustRandom()
	horizonClientMock := &horizonclient.MockClient{}
	signatureService := mocks.NewMockSignatureService(t)

	handler := &AssetsHandler{
		SignatureService: signatureService,
		HorizonClient:    horizonClientMock,
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

	signatureService.
		On("DistributionAccount").
		Return(distributionKP.Address())

	t.Run("returns error if no change trust operations is passed", func(t *testing.T) {
		err := handler.submitChangeTrustTransaction(ctx, acc, []*txnbuild.ChangeTrust{})
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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", ctx, tx, distributionKP.Address()).
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
		})
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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", ctx, tx, distributionKP.Address()).
			Return(signedTx, nil).
			Once()

		horizonClientMock.
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
		})
		assert.EqualError(t, err, "submitting change trust transaction to network: horizon error: \"\" (tx_failed, op_no_issuer) - check horizon.Error.Problem for more information")
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
				BaseFee:       txnbuild.MinBaseFee * feeMultiplierInStroops,
				Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)},
			},
		)
		require.NoError(t, err)

		signedTx, err := tx.Sign(network.TestNetworkPassphrase, distributionKP)
		require.NoError(t, err)

		signatureService.
			On("SignStellarTransaction", ctx, tx, distributionKP.Address()).
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
		})
		assert.NoError(t, err)
	})

	horizonClientMock.AssertExpectations(t)
	signatureService.AssertExpectations(t)
}
