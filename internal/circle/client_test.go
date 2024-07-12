package circle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_NewClient(t *testing.T) {
	mockTntManager := &tenant.TenantManagerMock{}
	t.Run("production environment", func(t *testing.T) {
		clientInterface := NewClient(utils.PubnetNetworkType, "test-key", mockTntManager)
		cc, ok := clientInterface.(*Client)
		assert.True(t, ok)
		assert.Equal(t, string(Production), cc.BasePath)
		assert.Equal(t, "test-key", cc.APIKey)
	})

	t.Run("sandbox environment", func(t *testing.T) {
		clientInterface := NewClient(utils.TestnetNetworkType, "test-key", mockTntManager)
		cc, ok := clientInterface.(*Client)
		assert.True(t, ok)
		assert.Equal(t, string(Sandbox), cc.BasePath)
		assert.Equal(t, "test-key", cc.APIKey)
	})
}

func Test_Client_Ping(t *testing.T) {
	ctx := context.Background()

	t.Run("ping error", func(t *testing.T) {
		cc, httpClientMock, _ := newClientWithMock(t)
		testError := errors.New("test error")
		httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		ok, err := cc.Ping(ctx)
		assert.EqualError(t, err, fmt.Errorf("making request: %w", testError).Error())
		assert.False(t, ok)
	})

	t.Run("ping successful", func(t *testing.T) {
		cc, httpClientMock, _ := newClientWithMock(t)
		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"message": "pong"}`)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/ping", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Empty(t, req.Header.Get("Authorization"))
			}).
			Once()

		ok, err := cc.Ping(ctx)
		assert.NoError(t, err)
		assert.True(t, ok)
	})
}

func Test_Client_PostTransfer(t *testing.T) {
	ctx := context.Background()
	validTransferReq := TransferRequest{
		Source:         TransferAccount{Type: TransferAccountTypeWallet, ID: "source-id"},
		Destination:    TransferAccount{Type: TransferAccountTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF", AddressTag: "txmemo2"},
		Amount:         Balance{Amount: "100.00", Currency: "USD"},
		IdempotencyKey: uuid.NewString(),
	}

	t.Run("post transfer error", func(t *testing.T) {
		cc, httpClientMock, _ := newClientWithMock(t)
		testError := errors.New("test error")
		httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		transfer, err := cc.PostTransfer(ctx, validTransferReq)
		assert.EqualError(t, err, fmt.Errorf("making request: %w", testError).Error())
		assert.Nil(t, transfer)
	})

	t.Run("post transfer fails to validate request", func(t *testing.T) {
		cc, _, _ := newClientWithMock(t)
		transfer, err := cc.PostTransfer(ctx, TransferRequest{})
		assert.EqualError(t, err, fmt.Errorf("validating transfer request: %w", errors.New("source type must be provided")).Error())
		assert.Nil(t, transfer)
	})

	t.Run("post transfer fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, httpClientMock, tntManagerMock := newClientWithMock(t)
		tnt := &tenant.Tenant{ID: "test-id"}
		ctx = tenant.SaveTenantInContext(ctx, tnt)

		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		tntManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()

		transfer, err := cc.PostTransfer(ctx, validTransferReq)
		assert.EqualError(t, err, "API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, transfer)
	})

	t.Run("post transfer successful", func(t *testing.T) {
		cc, httpClientMock, _ := newClientWithMock(t)
		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "test-id"}}`)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/transfers", req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			}).
			Once()

		transfer, err := cc.PostTransfer(ctx, validTransferReq)
		assert.NoError(t, err)
		assert.Equal(t, "test-id", transfer.ID)
	})
}

func Test_Client_GetTransferByID(t *testing.T) {
	ctx := context.Background()
	t.Run("get transfer by id error", func(t *testing.T) {
		cc, httpClientMock, _ := newClientWithMock(t)
		testError := errors.New("test error")
		httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		transfer, err := cc.GetTransferByID(ctx, "test-id")
		assert.EqualError(t, err, fmt.Errorf("making request: %w", testError).Error())
		assert.Nil(t, transfer)
	})

	t.Run("get transfer by id fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, httpClientMock, tntManagerMock := newClientWithMock(t)
		tnt := &tenant.Tenant{ID: "test-id"}
		ctx = tenant.SaveTenantInContext(ctx, tnt)

		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		tntManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()

		transfer, err := cc.GetTransferByID(ctx, "test-id")
		assert.EqualError(t, err, "API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, transfer)
	})

	t.Run("get transfer by id successful", func(t *testing.T) {
		cc, httpClientMock, _ := newClientWithMock(t)
		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "test-id"}}`)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/transfers/test-id", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Once()

		transfer, err := cc.GetTransferByID(ctx, "test-id")
		assert.NoError(t, err)
		assert.Equal(t, "test-id", transfer.ID)
	})
}

func Test_Client_GetWalletByID(t *testing.T) {
	ctx := context.Background()
	t.Run("get wallet by id error", func(t *testing.T) {
		cc, httpClientMock, _ := newClientWithMock(t)
		testError := errors.New("test error")
		httpClientMock.
			On("Do", mock.Anything).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/wallets/test-id", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Return(nil, testError).
			Once()

		wallet, err := cc.GetWalletByID(ctx, "test-id")
		assert.EqualError(t, err, fmt.Errorf("making request: %w", testError).Error())
		assert.Nil(t, wallet)
	})

	t.Run("get wallet by id fails auth", func(t *testing.T) {
		const unauthorizedResponse = `{
			"code": 401,
			"message": "Malformed key. Does it contain three parts?"
		}`
		cc, httpClientMock, tntManagerMock := newClientWithMock(t)
		tnt := &tenant.Tenant{ID: "test-id"}
		ctx = tenant.SaveTenantInContext(ctx, tnt)

		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		tntManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()

		transfer, err := cc.GetWalletByID(ctx, "test-id")
		assert.EqualError(t, err, "API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, transfer)
	})

	t.Run("get wallet by id successful", func(t *testing.T) {
		const getWalletResponseJSON = `{
			"data": {
				"walletId": "test-id",
				"entityId": "2f47c999-9022-4939-acea-dc3afa9ccbaf",
				"type": "end_user_wallet",
				"description": "Treasury Wallet",
				"balances": [
					{
						"amount": "4790.00",
						"currency": "USD"
					}
				]
			}
		}`
		cc, httpClientMock, _ := newClientWithMock(t)
		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(getWalletResponseJSON)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/wallets/test-id", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Once()

		wallet, err := cc.GetWalletByID(ctx, "test-id")
		assert.NoError(t, err)
		wantWallet := &Wallet{
			WalletID:    "test-id",
			EntityID:    "2f47c999-9022-4939-acea-dc3afa9ccbaf",
			Type:        "end_user_wallet",
			Description: "Treasury Wallet",
			Balances: []Balance{
				{Amount: "4790.00", Currency: "USD"},
			},
		}
		assert.Equal(t, wantWallet, wallet)
	})
}

func newClientWithMock(t *testing.T) (Client, *httpclientMocks.HttpClientMock, *tenant.TenantManagerMock) {
	httpClientMock := httpclientMocks.NewHttpClientMock(t)
	tntManagerMock := &tenant.TenantManagerMock{}

	return Client{
		BasePath:      "http://localhost:8080",
		APIKey:        "test-key",
		httpClient:    httpClientMock,
		tenantManager: tntManagerMock,
	}, httpClientMock, tntManagerMock
}
