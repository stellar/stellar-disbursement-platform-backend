package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
)

func Test_SponsoredTransactionHandler_CreateSponsoredTransaction(t *testing.T) {
	const contractAddress = "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"

	t.Run("returns unauthorized when wallet contract address is not in context", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := context.Background()

		const validInvokeHostFunctionOpXDR = "AAAAAAAAAAEBAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4fIAAAAAh0cmFuc2ZlcgAAAAMAAAASAAAAAAAAAAAXzoXCN9GMUZaRt9PhPtTS78G1YOFnR1iG5pXpG5+5SwAAABIAAAAAAAAAABfOhcI30YxRlpG30+E+1NLvwbVg4WdHWIbmlekbn7lLAAAACgAAAAAAAAAAAAAAAAAPQkAAAAAA"
		requestBody, err := json.Marshal(CreateSponsoredTransactionRequest{
			OperationXDR: validInvokeHostFunctionOpXDR,
		})
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/sponsored-transactions", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.CreateSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Result().StatusCode)
	})

	t.Run("returns bad request when JSON is malformed", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/sponsored-transactions", strings.NewReader("invalid-json"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.CreateSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
	})

	t.Run("returns bad request when operation_xdr is empty", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/sponsored-transactions", strings.NewReader(`{"operation_xdr": ""}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.CreateSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
	})

	t.Run("returns bad request when operation_xdr is invalid base64", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/sponsored-transactions", strings.NewReader(`{"operation_xdr": "invalid-base64-!!!"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.CreateSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
	})

	t.Run("returns bad request when operation_xdr is not valid XDR", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/sponsored-transactions", strings.NewReader(`{"operation_xdr": "aGVsbG8gd29ybGQ="}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		http.HandlerFunc(handler.CreateSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
	})

	t.Run("returns bad request when contract address is not supported for tenant", func(t *testing.T) {
		dbConnectionPool := testutils.GetDBConnectionPool(t)
		models, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		embeddedWallet := data.CreateWalletFixture(t, context.Background(), dbConnectionPool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
		data.MakeWalletEmbedded(t, context.Background(), dbConnectionPool, embeddedWallet.ID)

		walletAsset := data.CreateAssetFixture(t, context.Background(), dbConnectionPool, "USDC", keypair.MustRandom().Address())
		data.CreateWalletAssets(t, context.Background(), dbConnectionPool, embeddedWallet.ID, []string{walletAsset.ID})

		walletService := mocks.NewMockEmbeddedWalletService(t)
		handlerWithModels := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
			Models:                models,
			NetworkPassphrase:     network.TestNetworkPassphrase,
		}

		unsupportedAsset := data.Asset{
			Code:   "TEST",
			Issuer: keypair.MustRandom().Address(),
		}
		operationXDR, contractAddress := buildInvokeHostFunctionXDR(t, unsupportedAsset, network.TestNetworkPassphrase)

		requestBody, err := json.Marshal(CreateSponsoredTransactionRequest{
			OperationXDR: operationXDR,
		})
		require.NoError(t, err)

		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/sponsored-transactions", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		http.HandlerFunc(handlerWithModels.CreateSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
	})

	t.Run("successfully creates sponsored transaction for supported contract address", func(t *testing.T) {
		dbConnectionPool := testutils.GetDBConnectionPool(t)
		models, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		embeddedWallet := data.CreateWalletFixture(t, context.Background(), dbConnectionPool, "embedded-wallet", "https://example.com", "embedded.example.com", "embedded://")
		data.MakeWalletEmbedded(t, context.Background(), dbConnectionPool, embeddedWallet.ID)

		issuer := keypair.MustRandom().Address()
		asset := data.CreateAssetFixture(t, context.Background(), dbConnectionPool, "TEST", issuer)
		data.CreateWalletAssets(t, context.Background(), dbConnectionPool, embeddedWallet.ID, []string{asset.ID})

		walletService := mocks.NewMockEmbeddedWalletService(t)
		handlerWithModels := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
			Models:                models,
			NetworkPassphrase:     network.TestNetworkPassphrase,
		}

		operationXDR, contractAddress := buildInvokeHostFunctionXDR(t, *asset, network.TestNetworkPassphrase)

		requestBody, err := json.Marshal(CreateSponsoredTransactionRequest{
			OperationXDR: operationXDR,
		})
		require.NoError(t, err)

		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/sponsored-transactions", strings.NewReader(string(requestBody)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		walletService.On("SponsorTransaction", mock.Anything, contractAddress, operationXDR).Return("tx-id", nil).Once()

		rr := httptest.NewRecorder()
		http.HandlerFunc(handlerWithModels.CreateSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusAccepted, rr.Result().StatusCode)
		var respBody CreateSponsoredTransactionResponse
		err = json.Unmarshal(rr.Body.Bytes(), &respBody)
		require.NoError(t, err)

		assert.Equal(t, "tx-id", respBody.ID)
		assert.Equal(t, string(data.PendingSponsoredTransactionStatus), respBody.Status)
	})
}

func Test_SponsoredTransactionHandler_GetSponsoredTransaction(t *testing.T) {
	const contractAddress = "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"

	t.Run("returns unauthorized when wallet contract address is not in context", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := context.Background()
		transactionID := "missing-contract-address"

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/embedded-wallets/sponsored-transactions/%s", transactionID), nil)
		require.NoError(t, err)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", transactionID)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		http.HandlerFunc(handler.GetSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Result().StatusCode)
	})

	t.Run("returns bad request when transaction ID is empty", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/embedded-wallets/sponsored-transactions/", nil)
		require.NoError(t, err)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		http.HandlerFunc(handler.GetSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
	})

	t.Run("returns not found when transaction does not exist", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)
		transactionID := "non-existent-id"

		walletService.On("GetTransactionStatus", mock.Anything, contractAddress, transactionID).Return(nil, data.ErrRecordNotFound).Once()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/embedded-wallets/sponsored-transactions/%s", transactionID), nil)
		require.NoError(t, err)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", transactionID)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		http.HandlerFunc(handler.GetSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Result().StatusCode)
	})

	t.Run("returns internal server error when service fails", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)
		transactionID := "test-transaction-id"

		walletService.On("GetTransactionStatus", mock.Anything, contractAddress, transactionID).Return(nil, errors.New("service error")).Once()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/embedded-wallets/sponsored-transactions/%s", transactionID), nil)
		require.NoError(t, err)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", transactionID)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		http.HandlerFunc(handler.GetSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
	})

	t.Run("successfully returns sponsored transaction", func(t *testing.T) {
		walletService := mocks.NewMockEmbeddedWalletService(t)
		handler := SponsoredTransactionHandler{
			EmbeddedWalletService: walletService,
		}

		rr := httptest.NewRecorder()
		ctx := sdpcontext.SetWalletContractAddressInContext(context.Background(), contractAddress)
		transactionID := "test-transaction-id"
		createdAt := time.Now()
		updatedAt := time.Now()

		mockTransaction := &data.SponsoredTransaction{
			ID:              transactionID,
			Account:         "GDSPHTXJIMA762ZXHPDEKMEPGVUK6MQGJRM4YVBF2DDPZDV7VXFITYCN",
			Status:          string(data.SuccessSponsoredTransactionStatus),
			TransactionHash: "test-hash",
			CreatedAt:       &createdAt,
			UpdatedAt:       &updatedAt,
		}

		walletService.On("GetTransactionStatus", mock.Anything, contractAddress, transactionID).Return(mockTransaction, nil).Once()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/embedded-wallets/sponsored-transactions/%s", transactionID), nil)
		require.NoError(t, err)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", transactionID)
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		http.HandlerFunc(handler.GetSponsoredTransaction).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		var respBody GetSponsoredTransactionResponse
		err = json.Unmarshal(rr.Body.Bytes(), &respBody)
		require.NoError(t, err)

		assert.Equal(t, string(data.SuccessSponsoredTransactionStatus), respBody.Status)
		assert.Equal(t, "test-hash", respBody.TransactionHash)
	})
}

func Test_CreateSponsoredTransactionRequest_Validate(t *testing.T) {
	t.Run("returns error when operation_xdr is empty", func(t *testing.T) {
		req := CreateSponsoredTransactionRequest{
			OperationXDR: "",
		}
		err := req.Validate()
		assert.NotNil(t, err)
	})

	t.Run("returns error when operation_xdr is whitespace only", func(t *testing.T) {
		req := CreateSponsoredTransactionRequest{
			OperationXDR: "   ",
		}
		err := req.Validate()
		assert.NotNil(t, err)
	})

	t.Run("returns error when operation_xdr is invalid base64", func(t *testing.T) {
		req := CreateSponsoredTransactionRequest{
			OperationXDR: "invalid-base64-!!!",
		}
		err := req.Validate()
		assert.NotNil(t, err)
	})

	t.Run("returns error when operation_xdr is not valid XDR structure", func(t *testing.T) {
		req := CreateSponsoredTransactionRequest{
			OperationXDR: "aGVsbG8gd29ybGQ=",
		}
		err := req.Validate()
		assert.NotNil(t, err)
	})

	t.Run("returns nil when operation_xdr is a valid invoke host function op", func(t *testing.T) {
		req := CreateSponsoredTransactionRequest{
			OperationXDR: "AAAAAAAAAAEBAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4fIAAAAAh0cmFuc2ZlcgAAAAMAAAASAAAAAAAAAAAXzoXCN9GMUZaRt9PhPtTS78G1YOFnR1iG5pXpG5+5SwAAABIAAAAAAAAAABfOhcI30YxRlpG30+E+1NLvwbVg4WdHWIbmlekbn7lLAAAACgAAAAAAAAAAAAAAAAAPQkAAAAAA",
		}
		err := req.Validate()
		assert.Nil(t, err)
	})
}

func buildInvokeHostFunctionXDR(t *testing.T, asset data.Asset, networkPassphrase string) (string, string) {
	t.Helper()

	var xdrAsset xdr.Asset
	if asset.IsNative() {
		xdrAsset = xdr.MustNewNativeAsset()
	} else {
		var err error
		xdrAsset, err = xdr.NewCreditAsset(asset.Code, asset.Issuer)
		require.NoError(t, err)
	}

	contractID, err := xdrAsset.ContractID(networkPassphrase)
	require.NoError(t, err)
	var contractIDHash xdr.ContractId
	copy(contractIDHash[:], contractID[:])

	invoke := xdr.InvokeHostFunctionOp{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: xdr.ScAddress{
					Type:       xdr.ScAddressTypeScAddressTypeContract,
					ContractId: &contractIDHash,
				},
				FunctionName: xdr.ScSymbol("transfer"),
				Args:         []xdr.ScVal{},
			},
		},
	}

	operationXDR, err := xdr.MarshalBase64(invoke)
	require.NoError(t, err)

	contractAddress, err := strkey.Encode(strkey.VersionByteContract, contractID[:])
	require.NoError(t, err)

	return operationXDR, contractAddress
}
