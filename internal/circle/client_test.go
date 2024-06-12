package circle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func Test_NewClient(t *testing.T) {
	t.Run("production environment", func(t *testing.T) {
		cc := NewClient(Production, "test-key")
		assert.Equal(t, "https://api.circle.com", cc.BasePath)
		assert.Equal(t, "test-key", cc.APIKey)
	})

	t.Run("sandbox environment", func(t *testing.T) {
		cc := NewClient(Sandbox, "test-key")
		assert.Equal(t, "https://api-sandbox.circle.com", cc.BasePath)
		assert.Equal(t, "test-key", cc.APIKey)
	})
}

func Test_Client_Ping(t *testing.T) {
	ctx := context.Background()

	t.Run("ping error", func(t *testing.T) {
		cc, httpClientMock := newClientWithMock(t)
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
		cc, httpClientMock := newClientWithMock(t)
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
		Source:      TransferAccount{Type: TransferAccountTypeWallet, ID: "source-id"},
		Destination: TransferAccount{Type: TransferAccountTypeBlockchain, Chain: "XLM", Address: "GBG2DFASN2E5ZZSOYH7SJ7HWBKR4M5LYQ5Q5ZVBWS3RI46GDSYTEA6YF", AddressTag: "txmemo2"},
		Amount:      Money{Amount: "100.00", Currency: "USD"},
	}

	t.Run("post transfer error", func(t *testing.T) {
		cc, httpClientMock := newClientWithMock(t)
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
		cc, _ := newClientWithMock(t)
		transfer, err := cc.PostTransfer(ctx, TransferRequest{})
		assert.EqualError(t, err, fmt.Errorf("validating transfer request: %w", errors.New("source type must be provided")).Error())
		assert.Nil(t, transfer)
	})

	t.Run("post transfer fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, httpClientMock := newClientWithMock(t)

		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()

		transfer, err := cc.PostTransfer(ctx, validTransferReq)
		assert.EqualError(t, err, "API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[]")
		assert.Nil(t, transfer)
	})

	t.Run("post transfer successful", func(t *testing.T) {
		cc, httpClientMock := newClientWithMock(t)
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
		cc, httpClientMock := newClientWithMock(t)
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
		cc, httpClientMock := newClientWithMock(t)
		httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()

		transfer, err := cc.GetTransferByID(ctx, "test-id")
		assert.EqualError(t, err, "API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[]")
		assert.Nil(t, transfer)
	})

	t.Run("get transfer by id successful", func(t *testing.T) {
		cc, httpClientMock := newClientWithMock(t)
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

func newClientWithMock(t *testing.T) (Client, *httpclientMocks.HttpClientMock) {
	httpClientMock := httpclientMocks.NewHttpClientMock(t)

	return Client{
		BasePath:   "http://localhost:8080",
		APIKey:     "test-key",
		httpClient: httpClientMock,
	}, httpClientMock
}
