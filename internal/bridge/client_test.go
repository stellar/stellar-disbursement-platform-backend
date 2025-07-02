package bridge

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
	"github.com/stretchr/testify/require"

	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
)

func Test_NewClient(t *testing.T) {
	t.Run("valid options", func(t *testing.T) {
		client, err := NewClient(ClientOptions{
			BaseURL: "https://api.bridge.example.com",
			APIKey:  "test-api-key",
		})
		require.NoError(t, err)
		assert.NotNil(t, client)

		c, ok := client.(*Client)
		assert.True(t, ok)
		assert.Equal(t, "https://api.bridge.example.com", c.baseURL)
		assert.Equal(t, "test-api-key", c.apiKey)
	})

	t.Run("missing baseURL", func(t *testing.T) {
		client, err := NewClient(ClientOptions{
			APIKey: "test-api-key",
		})
		assert.EqualError(t, err, "validating client options: baseURL is required")
		assert.Nil(t, client)
	})

	t.Run("missing apiKey", func(t *testing.T) {
		client, err := NewClient(ClientOptions{
			BaseURL: "https://api.bridge.example.com",
		})
		assert.EqualError(t, err, "validating client options: apiKey is required")
		assert.Nil(t, client)
	})
}

func Test_Client_PostKYCLink(t *testing.T) {
	ctx := context.Background()
	validKYCRequest := KYCLinkRequest{
		FullName: "John Doe",
		Email:    "john@example.com",
		Type:     KYCTypeIndividual,
	}

	t.Run("post kyc link error", func(t *testing.T) {
		client, mocks := newClientWithMocks(t)
		testError := errors.New("test error")
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		kycLink, err := client.PostKYCLink(ctx, validKYCRequest)
		assert.EqualError(t, err, fmt.Errorf("making HTTP request: making HTTP request: %w", testError).Error())
		assert.Nil(t, kycLink)
	})

	t.Run("post kyc link fails to validate request", func(t *testing.T) {
		client, _ := newClientWithMocks(t)
		kycLink, err := client.PostKYCLink(ctx, KYCLinkRequest{})
		assert.EqualError(t, err, "validating KYC link request: full_name is required")
		assert.Nil(t, kycLink)
	})

	t.Run("post kyc link api error", func(t *testing.T) {
		errorResponse := `{
			"code": "BAD_REQUEST",
			"message": "Invalid customer ID",
			"type": "validation_error"
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewBufferString(errorResponse)),
			}, nil).
			Once()

		kycLink, err := client.PostKYCLink(ctx, validKYCRequest)
		assert.EqualError(t, err, "making HTTP request: Bridge API error [BAD_REQUEST] = Invalid customer ID")
		assert.Nil(t, kycLink)
	})

	t.Run("post kyc link successful", func(t *testing.T) {
		successResponse := `{
			"id": "kyc-link-123",
			"full_name": "John Doe",
			"email": "john@example.com",
			"type": "individual",
			"kyc_link": "https://bridge.example.com/kyc/kyc-link-123",
			"kyc_status": "not_started",
			"tos_status": "pending",
			"customer_id": "customer-123"
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(bytes.NewBufferString(successResponse)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "https://api.bridge.example.com/v0/kyc_links", req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "test-api-key", req.Header.Get("Api-Key"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.NotEmpty(t, req.Header.Get("Idempotency-Key"))
			}).
			Once()

		kycLink, err := client.PostKYCLink(ctx, validKYCRequest)
		assert.NoError(t, err)
		assert.Equal(t, "kyc-link-123", kycLink.ID)
		assert.Equal(t, "customer-123", kycLink.CustomerID)
		assert.Equal(t, KYCStatusNotStarted, kycLink.KYCStatus)
	})
}

func Test_Client_GetKYCLink(t *testing.T) {
	ctx := context.Background()

	t.Run("get kyc link error", func(t *testing.T) {
		client, mocks := newClientWithMocks(t)
		testError := errors.New("test error")
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		kycLink, err := client.GetKYCLink(ctx, "kyc-link-123")
		assert.EqualError(t, err, fmt.Errorf("making HTTP request: making HTTP request: %w", testError).Error())
		assert.Nil(t, kycLink)
	})

	t.Run("get kyc link missing id", func(t *testing.T) {
		client, _ := newClientWithMocks(t)
		kycLink, err := client.GetKYCLink(ctx, "")
		assert.EqualError(t, err, "kycLinkID is required")
		assert.Nil(t, kycLink)
	})

	t.Run("get kyc link api error", func(t *testing.T) {
		errorResponse := `{
			"code": "NOT_FOUND",
			"message": "KYC link not found",
			"type": "resource_error"
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString(errorResponse)),
			}, nil).
			Once()

		kycLink, err := client.GetKYCLink(ctx, "kyc-link-123")
		assert.EqualError(t, err, "making HTTP request: Bridge API error [NOT_FOUND] = KYC link not found")
		assert.Nil(t, kycLink)
	})

	t.Run("get kyc link successful", func(t *testing.T) {
		successResponse := `{
			"id": "kyc-link-123",
			"full_name": "John Doe",
			"email": "john@example.com",
			"type": "individual",
			"kyc_link": "https://bridge.example.com/kyc/kyc-link-123",
			"kyc_status": "approved",
			"tos_status": "approved",
			"customer_id": "customer-123"
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(successResponse)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "https://api.bridge.example.com/v0/kyc_links/kyc-link-123", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "test-api-key", req.Header.Get("Api-Key"))
				assert.Empty(t, req.Header.Get("Content-Type"))
				assert.Empty(t, req.Header.Get("Idempotency-Key"))
			}).
			Once()

		kycLink, err := client.GetKYCLink(ctx, "kyc-link-123")
		assert.NoError(t, err)
		assert.Equal(t, "kyc-link-123", kycLink.ID)
		assert.Equal(t, "customer-123", kycLink.CustomerID)
		assert.Equal(t, KYCStatusApproved, kycLink.KYCStatus)
	})
}

func Test_Client_PostVirtualAccount(t *testing.T) {
	ctx := context.Background()
	validVARequest := VirtualAccountRequest{
		Source: VirtualAccountSource{
			Currency: "USD",
		},
		Destination: VirtualAccountDestination{
			PaymentRail: "stellar",
			Currency:    "USDC",
			Address:     "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN",
		},
	}

	t.Run("post virtual account error", func(t *testing.T) {
		client, mocks := newClientWithMocks(t)
		testError := errors.New("test error")
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		va, err := client.PostVirtualAccount(ctx, "customer-123", validVARequest)
		assert.EqualError(t, err, fmt.Errorf("making HTTP request: making HTTP request: %w", testError).Error())
		assert.Nil(t, va)
	})

	t.Run("post virtual account missing customer id", func(t *testing.T) {
		client, _ := newClientWithMocks(t)
		va, err := client.PostVirtualAccount(ctx, "", validVARequest)
		assert.EqualError(t, err, "customerID is required")
		assert.Nil(t, va)
	})

	t.Run("post virtual account fails to validate request", func(t *testing.T) {
		client, _ := newClientWithMocks(t)
		va, err := client.PostVirtualAccount(ctx, "customer-123", VirtualAccountRequest{})
		assert.EqualError(t, err, "validating virtual account request: source currency is required")
		assert.Nil(t, va)
	})

	t.Run("post virtual account api error", func(t *testing.T) {
		errorResponse := `{
			"code": "INVALID_CUSTOMER",
			"message": "Customer not found",
			"type": "validation_error"
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewBufferString(errorResponse)),
			}, nil).
			Once()

		va, err := client.PostVirtualAccount(ctx, "customer-123", validVARequest)
		assert.EqualError(t, err, "making HTTP request: Bridge API error [INVALID_CUSTOMER] = Customer not found")
		assert.Nil(t, va)
	})

	t.Run("post virtual account successful", func(t *testing.T) {
		successResponse := `{
			"id": "va-123",
			"status": "activated",
			"customer_id": "customer-123",
			"source_deposit_instructions": {
				"bank_beneficiary_name": "Test Beneficiary",
				"currency": "USD",
				"bank_name": "Test Bank",
				"bank_address": "123 Bank St",
				"bank_account_number": "1234567890",
				"bank_routing_number": "987654321",
				"payment_rails": ["ach"]
			},
			"destination": {
				"payment_rail": "stellar",
				"currency": "USDC",
				"address": "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN"
			}
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(bytes.NewBufferString(successResponse)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "https://api.bridge.example.com/v0/customers/customer-123/virtual_accounts", req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "test-api-key", req.Header.Get("Api-Key"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
				assert.NotEmpty(t, req.Header.Get("Idempotency-Key"))
			}).
			Once()

		va, err := client.PostVirtualAccount(ctx, "customer-123", validVARequest)
		assert.NoError(t, err)
		assert.Equal(t, "va-123", va.ID)
		assert.Equal(t, "customer-123", va.CustomerID)
		assert.Equal(t, VirtualAccountActivated, va.Status)
		assert.Equal(t, "stellar", va.Destination.PaymentRail)
	})
}

func Test_Client_GetVirtualAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("get virtual account error", func(t *testing.T) {
		client, mocks := newClientWithMocks(t)
		testError := errors.New("test error")
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		va, err := client.GetVirtualAccount(ctx, "customer-123", "va-123")
		assert.EqualError(t, err, fmt.Errorf("making HTTP request: making HTTP request: %w", testError).Error())
		assert.Nil(t, va)
	})

	t.Run("get virtual account missing customer id", func(t *testing.T) {
		client, _ := newClientWithMocks(t)
		va, err := client.GetVirtualAccount(ctx, "", "va-123")
		assert.EqualError(t, err, "customerID is required")
		assert.Nil(t, va)
	})

	t.Run("get virtual account missing virtual account id", func(t *testing.T) {
		client, _ := newClientWithMocks(t)
		va, err := client.GetVirtualAccount(ctx, "customer-123", "")
		assert.EqualError(t, err, "virtualAccountID is required")
		assert.Nil(t, va)
	})

	t.Run("get virtual account api error", func(t *testing.T) {
		errorResponse := `{
			"code": "NOT_FOUND",
			"message": "Virtual account not found",
			"type": "resource_error"
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString(errorResponse)),
			}, nil).
			Once()

		va, err := client.GetVirtualAccount(ctx, "customer-123", "va-123")
		assert.EqualError(t, err, "making HTTP request: Bridge API error [NOT_FOUND] = Virtual account not found")
		assert.Nil(t, va)
	})

	t.Run("get virtual account successful", func(t *testing.T) {
		successResponse := `{
			"id": "va-123",
			"status": "activated",
			"customer_id": "customer-123",
			"source_deposit_instructions": {
				"bank_beneficiary_name": "Test Beneficiary",
				"currency": "USD",
				"bank_name": "Test Bank",
				"bank_address": "123 Bank St",
				"bank_account_number": "1234567890",
				"bank_routing_number": "987654321",
				"payment_rails": ["ach"]
			},
			"destination": {
				"payment_rail": "stellar",
				"currency": "USDC",
				"address": "GCKFBEIYTKP5RDBPFKWYFVQNMZ5KMGMW3RFKAWJ3CCDQPWXEMFXH7YDN"
			}
		}`
		client, mocks := newClientWithMocks(t)
		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(successResponse)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "https://api.bridge.example.com/v0/customers/customer-123/virtual_accounts/va-123", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "test-api-key", req.Header.Get("Api-Key"))
				assert.Empty(t, req.Header.Get("Content-Type"))
				assert.Empty(t, req.Header.Get("Idempotency-Key"))
			}).
			Once()

		va, err := client.GetVirtualAccount(ctx, "customer-123", "va-123")
		assert.NoError(t, err)
		assert.Equal(t, "va-123", va.ID)
		assert.Equal(t, "customer-123", va.CustomerID)
		assert.Equal(t, VirtualAccountActivated, va.Status)
		assert.Equal(t, "stellar", va.Destination.PaymentRail)
	})
}

func Test_Client_handleErrorResponse(t *testing.T) {
	client := &Client{}

	t.Run("success status codes", func(t *testing.T) {
		tests := []int{http.StatusOK, http.StatusCreated}
		for _, statusCode := range tests {
			resp := &http.Response{StatusCode: statusCode}
			err := client.handleErrorResponse(resp)
			assert.NoError(t, err)
		}
	})

	t.Run("error with valid json", func(t *testing.T) {
		errorResponse := `{
			"code": "VALIDATION_ERROR",
			"message": "Invalid request",
			"type": "validation_error",
			"details": "Field 'customer_id' is required"
		}`
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString(errorResponse)),
		}

		err := client.handleErrorResponse(resp)
		assert.EqualError(t, err, "Bridge API error [VALIDATION_ERROR] = Invalid request - Field 'customer_id' is required")
	})

	t.Run("error with invalid json", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
		}

		err := client.handleErrorResponse(resp)
		assert.Contains(t, err.Error(), "bridge API returned status 500")
	})

	t.Run("error without details", func(t *testing.T) {
		errorResponse := `{
			"code": "SERVER_ERROR",
			"message": "Internal error",
			"type": "server_error"
		}`
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(errorResponse)),
		}

		err := client.handleErrorResponse(resp)
		assert.EqualError(t, err, "Bridge API error [SERVER_ERROR] = Internal error")
	})
}

func Test_Client_makeRequest(t *testing.T) {
	t.Run("request construction", func(t *testing.T) {
		client, mocks := newClientWithMocks(t)
		ctx := context.Background()

		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("{}")),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "https://example.com/test", req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "test-api-key", req.Header.Get("Api-Key"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

				// Verify idempotency key is a valid UUID
				idempotencyKey := req.Header.Get("Idempotency-Key")
				assert.NotEmpty(t, idempotencyKey)
				_, err := uuid.Parse(idempotencyKey)
				assert.NoError(t, err)
			}).
			Once()

		body := bytes.NewBufferString(`{"test": "data"}`)
		resp, err := client.makeRequest(ctx, http.MethodPost, "https://example.com/test", body)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("get request headers", func(t *testing.T) {
		client, mocks := newClientWithMocks(t)
		ctx := context.Background()

		mocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("{}")),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "test-api-key", req.Header.Get("Api-Key"))
				assert.Empty(t, req.Header.Get("Content-Type"))
				assert.Empty(t, req.Header.Get("Idempotency-Key"))
			}).
			Once()

		resp, err := client.makeRequest(ctx, http.MethodGet, "https://example.com/test", nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func newClientWithMocks(t *testing.T) (*Client, *clientMocks) {
	httpClientMock := httpclientMocks.NewHttpClientMock(t)

	return &Client{
			baseURL:    "https://api.bridge.example.com",
			apiKey:     "test-api-key",
			httpClient: httpClientMock,
		}, &clientMocks{
			httpClientMock: httpClientMock,
		}
}

type clientMocks struct {
	httpClientMock *httpclientMocks.HttpClientMock
}
