package circle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_NewClient(t *testing.T) {
	mockTntManager := &tenant.TenantManagerMock{}
	mMonitorService := monitorMocks.NewMockMonitorService(t)
	t.Run("production environment", func(t *testing.T) {
		clientInterface := NewClient(ClientOptions{
			NetworkType:    utils.PubnetNetworkType,
			APIKey:         "test-key",
			TenantManager:  mockTntManager,
			MonitorService: mMonitorService,
		})
		cc, ok := clientInterface.(*Client)
		assert.True(t, ok)
		assert.Equal(t, string(Production), cc.BasePath)
		assert.Equal(t, "test-key", cc.APIKey)
	})

	t.Run("sandbox environment", func(t *testing.T) {
		clientInterface := NewClient(ClientOptions{
			NetworkType:    utils.TestnetNetworkType,
			APIKey:         "test-key",
			TenantManager:  mockTntManager,
			MonitorService: mMonitorService,
		})
		cc, ok := clientInterface.(*Client)
		assert.True(t, ok)
		assert.Equal(t, string(Sandbox), cc.BasePath)
		assert.Equal(t, "test-key", cc.APIKey)
	})
}

func Test_Client_Ping(t *testing.T) {
	ctx := context.Background()

	t.Run("ping error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		ok, err := cc.Ping(ctx)
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/ping: %w", testError).Error())
		assert.False(t, ok)
	})

	t.Run("ping successful", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		cMocks.httpClientMock.
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
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		transfer, err := cc.PostTransfer(ctx, validTransferReq)
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/transfers: %w", testError).Error())
		assert.Nil(t, transfer)
	})

	t.Run("post transfer fails to validate request", func(t *testing.T) {
		cc, _ := newClientWithMocks(t)
		transfer, err := cc.PostTransfer(ctx, TransferRequest{})
		assert.EqualError(t, err, fmt.Errorf("validating transfer request: %w", errors.New("source type must be provided")).Error())
		assert.Nil(t, transfer)
	})

	t.Run("post transfer fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    transferPath,
			"method":      http.MethodPost,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		transfer, err := cc.PostTransfer(ctx, validTransferReq)
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, transfer)
	})

	t.Run("post transfer successful", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
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

		expectedLabels := map[string]string{
			"endpoint":    transferPath,
			"method":      http.MethodPost,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusCreated),
			"tenant_name": "test-tenant",
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		transfer, err := cc.PostTransfer(ctx, validTransferReq)
		assert.NoError(t, err)
		assert.Equal(t, "test-id", transfer.ID)
	})
}

func Test_Client_GetTransferByID(t *testing.T) {
	ctx := context.Background()
	t.Run("get transfer by id error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		transfer, err := cc.GetTransferByID(ctx, "test-id")
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/transfers/test-id: %w", testError).Error())
		assert.Nil(t, transfer)
	})

	t.Run("get transfer by id fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    transferPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		transfer, err := cc.GetTransferByID(ctx, "test-id")
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, transfer)
	})

	t.Run("get transfer by id successful", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
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

		expectedLabels := map[string]string{
			"endpoint":    transferPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusOK),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		transfer, err := cc.GetTransferByID(ctx, "test-id")
		assert.NoError(t, err)
		assert.Equal(t, "test-id", transfer.ID)
	})
}

func Test_Client_PostRecipient(t *testing.T) {
	ctx := context.Background()
	validRecipientReq := RecipientRequest{
		IdempotencyKey: uuid.NewString(),
		Address:        "GCESKSSHPZKB6IE67LFZRZBGSX2FTHP4LUOIOZ54BUQFHYCQGH3WGUNX",
		Chain:          StellarChainCode,
		Metadata:       RecipientMetadata{Nickname: "test-nickname"},
	}

	t.Run("post recipient error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		recipient, err := cc.PostRecipient(ctx, validRecipientReq)
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/addressBook/recipients: %w", testError).Error())
		assert.Nil(t, recipient)
	})

	t.Run("post recipient fails to validate request", func(t *testing.T) {
		cc, _ := newClientWithMocks(t)
		recipient, err := cc.PostRecipient(ctx, RecipientRequest{})
		assert.EqualError(t, err, "validating recipient request: idempotency key must be provided")
		assert.Nil(t, recipient)
	})

	t.Run("post recipient fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    addressRecipientPath,
			"method":      http.MethodPost,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		recipient, err := cc.PostRecipient(ctx, validRecipientReq)
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, recipient)
	})

	t.Run("post transfer successful", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "test-id"}}`)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/addressBook/recipients", req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			}).
			Once()

		expectedLabels := map[string]string{
			"endpoint":    addressRecipientPath,
			"method":      http.MethodPost,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusCreated),
			"tenant_name": "test-tenant",
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		recipient, err := cc.PostRecipient(ctx, validRecipientReq)
		assert.NoError(t, err)
		assert.Equal(t, "test-id", recipient.ID)
	})
}

func Test_Client_GetRecipientByID(t *testing.T) {
	ctx := context.Background()
	t.Run("get recipient by id error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		recipient, err := cc.GetRecipientByID(ctx, "test-id")
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/addressBook/recipients/test-id: %w", testError).Error())
		assert.Nil(t, recipient)
	})

	t.Run("get recipient by id fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    addressRecipientPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		recipient, err := cc.GetRecipientByID(ctx, "test-id")
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, recipient)
	})

	t.Run("get recipient by id successful", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "test-id"}}`)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/addressBook/recipients/test-id", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Once()

		expectedLabels := map[string]string{
			"endpoint":    addressRecipientPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusOK),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		recipient, err := cc.GetRecipientByID(ctx, "test-id")
		assert.NoError(t, err)
		assert.Equal(t, "test-id", recipient.ID)
	})
}

func Test_Client_PostPayout(t *testing.T) {
	ctx := context.Background()
	validPayoutReq := PayoutRequest{
		IdempotencyKey: uuid.NewString(),
		Source:         TransferAccount{Type: TransferAccountTypeWallet, ID: "source-id"},
		Destination:    TransferAccount{Type: TransferAccountTypeAddressBook, Chain: StellarChainCode, ID: uuid.NewString(), AddressTag: "txmemo2"},
		Amount:         Balance{Amount: "100.00", Currency: "USD"},
		ToAmount:       ToAmount{Currency: "USD"},
	}

	t.Run("post payout error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		payout, err := cc.PostPayout(ctx, validPayoutReq)
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/payouts: %w", testError).Error())
		assert.Nil(t, payout)
	})

	t.Run("post payout fails to validate request", func(t *testing.T) {
		cc, _ := newClientWithMocks(t)
		payout, err := cc.PostPayout(ctx, PayoutRequest{})
		assert.EqualError(t, err, "validating payout request: idempotency key must be provided")
		assert.Nil(t, payout)
	})

	t.Run("post payout fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    payoutPath,
			"method":      http.MethodPost,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		payout, err := cc.PostPayout(ctx, validPayoutReq)
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, payout)
	})

	t.Run("post transfer successful", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "test-id"}}`)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/payouts", req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
				assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			}).
			Once()

		expectedLabels := map[string]string{
			"endpoint":    payoutPath,
			"method":      http.MethodPost,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusCreated),
			"tenant_name": "test-tenant",
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		payout, err := cc.PostPayout(ctx, validPayoutReq)
		assert.NoError(t, err)
		assert.Equal(t, "test-id", payout.ID)
	})
}

func Test_Client_GetPayoutByID(t *testing.T) {
	ctx := context.Background()
	t.Run("get payout by id error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(nil, testError).
			Once()

		payout, err := cc.GetPayoutByID(ctx, "test-id")
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/payouts/test-id: %w", testError).Error())
		assert.Nil(t, payout)
	})

	t.Run("get payout by id fails auth", func(t *testing.T) {
		unauthorizedResponse := `{"code": 401, "message": "Malformed key. Does it contain three parts?"}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    payoutPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		payout, err := cc.GetPayoutByID(ctx, "test-id")
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, payout)
	})

	t.Run("get payout by id successful", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "test-id"}}`)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/payouts/test-id", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Once()

		expectedLabels := map[string]string{
			"endpoint":    payoutPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusOK),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		payout, err := cc.GetPayoutByID(ctx, "test-id")
		assert.NoError(t, err)
		assert.Equal(t, "test-id", payout.ID)
	})
}

func Test_Client_GetBusinessBalances(t *testing.T) {
	ctx := context.Background()
	t.Run("get business balances error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/businessAccount/balances", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Return(nil, testError).
			Once()

		wallet, err := cc.GetBusinessBalances(ctx)
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/businessAccount/balances: %w", testError).Error())
		assert.Nil(t, wallet)
	})

	t.Run("get business balances fails auth", func(t *testing.T) {
		const unauthorizedResponse = `{
			"code": 401,
			"message": "Malformed key. Does it contain three parts?"
		}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    businessBalancesPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		transfer, err := cc.GetBusinessBalances(ctx)
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, transfer)
	})

	t.Run("get business balances successful", func(t *testing.T) {
		const getBalancesResponseJSON = `{
			"data": {
					"available": [
						{
							"amount": "22306.90",
							"currency": "USD"
						}
					],
					"unsettled": []
				}
			}
		}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(getBalancesResponseJSON)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/businessAccount/balances", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Once()
		expectedLabels := map[string]string{
			"endpoint":    businessBalancesPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusOK),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		businessBalances, err := cc.GetBusinessBalances(ctx)
		assert.NoError(t, err)

		wantBusinessBalances := &Balances{
			Available: []Balance{
				{Amount: "22306.90", Currency: "USD"},
			},
			Unsettled: []Balance{},
		}
		assert.Equal(t, wantBusinessBalances, businessBalances)
	})
}

func Test_Client_GetAccountConfiguration(t *testing.T) {
	ctx := context.Background()
	t.Run("get configuration error", func(t *testing.T) {
		cc, cMocks := newClientWithMocks(t)
		testError := errors.New("test error")
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/configuration", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Return(nil, testError).
			Once()

		wallet, err := cc.GetAccountConfiguration(ctx)
		assert.EqualError(t, err, fmt.Errorf("making request: submitting request to http://localhost:8080/v1/configuration: %w", testError).Error())
		assert.Nil(t, wallet)
	})

	t.Run("get configuration fails auth", func(t *testing.T) {
		const unauthorizedResponse = `{
			"code": 401,
			"message": "Malformed key. Does it contain three parts?"
		}`
		cc, cMocks := newClientWithMocks(t)
		tnt := &schema.Tenant{ID: "test-id"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(unauthorizedResponse)),
			}, nil).
			Once()
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()
		expectedLabels := map[string]string{
			"endpoint":    configurationPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusUnauthorized),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		transfer, err := cc.GetAccountConfiguration(ctx)
		assert.EqualError(t, err, "handling API response error: circle API error: APIError: Code=401, Message=Malformed key. Does it contain three parts?, Errors=[], StatusCode=401")
		assert.Nil(t, transfer)
	})

	t.Run("get configuration successful", func(t *testing.T) {
		const getConfigurationResponseJSON = `{
			"data": {
				"payments": {
					"masterWalletId": "1016352538"
				}
			}
		}`
		tnt := &schema.Tenant{ID: "test-id", Name: "test-tenant"}
		ctx = sdpcontext.SetTenantInContext(ctx, tnt)

		cc, cMocks := newClientWithMocks(t)
		cMocks.httpClientMock.
			On("Do", mock.Anything).
			Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(getConfigurationResponseJSON)),
			}, nil).
			Run(func(args mock.Arguments) {
				req, ok := args.Get(0).(*http.Request)
				assert.True(t, ok)

				assert.Equal(t, "http://localhost:8080/v1/configuration", req.URL.String())
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
			}).
			Once()
		expectedLabels := map[string]string{
			"endpoint":    configurationPath,
			"method":      http.MethodGet,
			"status":      "success",
			"status_code": strconv.Itoa(http.StatusOK),
			"tenant_name": tnt.Name,
		}
		cMocks.monitorServiceMock.
			On("MonitorHistogram", mock.Anything, monitor.CircleAPIRequestDurationTag, expectedLabels).
			Return(nil).Once()
		cMocks.monitorServiceMock.
			On("MonitorCounters", monitor.CircleAPIRequestsTotalTag, expectedLabels).
			Return(nil).Once()

		config, err := cc.GetAccountConfiguration(ctx)
		assert.NoError(t, err)
		wantConfig := &AccountConfiguration{
			Payments: WalletConfig{
				MasterWalletID: "1016352538",
			},
		}
		assert.Equal(t, wantConfig, config)
	})
}

func Test_Client_handleError(t *testing.T) {
	ctx := context.Background()
	tnt := &schema.Tenant{ID: "test-id"}
	ctx = sdpcontext.SetTenantInContext(ctx, tnt)

	cc, cMocks := newClientWithMocks(t)

	t.Run("deactivate tenant distribution account error", func(t *testing.T) {
		testError := errors.New("foo")
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(testError).Once()

		err := cc.handleError(ctx, &http.Response{StatusCode: http.StatusUnauthorized})
		assert.EqualError(t, err, fmt.Errorf("deactivating tenant distribution account: %w", testError).Error())
	})

	t.Run("deactivates tenant distribution account if Circle error response is unauthorized", func(t *testing.T) {
		unauthorizedResponse := &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(bytes.NewBufferString(`{"code": 401, "message": "Unauthorized"}`)),
		}
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()

		err := cc.handleError(ctx, unauthorizedResponse)
		assert.EqualError(t, fmt.Errorf("circle API error: %w", errors.New("APIError: Code=401, Message=Unauthorized, Errors=[], StatusCode=401")), err.Error())
	})

	t.Run("deactivates tenant distribution account if Circle error response is forbidden", func(t *testing.T) {
		unauthorizedResponse := &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(bytes.NewBufferString(`{"code": 403, "message": "Forbidden"}`)),
		}
		cMocks.tenantManagerMock.
			On("DeactivateTenantDistributionAccount", mock.Anything, tnt.ID).
			Return(nil).Once()

		err := cc.handleError(ctx, unauthorizedResponse)
		assert.EqualError(t, fmt.Errorf("circle API error: %w", errors.New("APIError: Code=403, Message=Forbidden, Errors=[], StatusCode=403")), err.Error())
	})

	t.Run("does not deactivate tenant distribution account if Circle error response is not unauthorized or forbidden", func(t *testing.T) {
		unauthorizedResponse := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewBufferString(`{"code": 400, "message": "Bad Request"}`)),
		}

		err := cc.handleError(ctx, unauthorizedResponse)
		assert.EqualError(t, fmt.Errorf("circle API error: %w", errors.New("APIError: Code=400, Message=Bad Request, Errors=[], StatusCode=400")), err.Error())
	})

	t.Run("records error correctly when not proper json", func(t *testing.T) {
		unauthorizedResponse := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(bytes.NewBufferString(`error code: 1015`)),
		}

		err := cc.handleError(ctx, unauthorizedResponse)
		assert.EqualError(t, fmt.Errorf("circle API error: %w", errors.New("APIError: Code=0, Message=error code: 1015, Errors=[], StatusCode=429")), err.Error())
	})

	cMocks.tenantManagerMock.AssertExpectations(t)
}

func Test_Client_request(t *testing.T) {
	tests := []struct {
		name               string
		responses          []http.Response
		expectedAttempts   int
		expectedStatusCode int
		expectedError      string
	}{
		{
			name: "Success on first attempt",
			responses: []http.Response{
				{StatusCode: http.StatusOK},
			},
			expectedAttempts:   1,
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Success after rate limit",
			responses: []http.Response{
				{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"1"}},
				},
				{StatusCode: http.StatusOK},
			},
			expectedAttempts:   2,
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Fail after max retries",
			responses: []http.Response{
				{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"1"}},
				},
				{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"1"}},
				},
				{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"1"}},
				},
				{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"1"}},
				},
			},
			expectedAttempts: 4,
			expectedError:    "rate limited, retry after: 1s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cc, cMocks := newClientWithMocks(t)
			httpClientMock := cMocks.httpClientMock

			ctx := context.Background()
			u := "https://api-sandbox.circle.com/test"
			method := http.MethodGet
			isAuthed := true
			body := []byte("test-body")

			for _, resp := range tt.responses {
				cMocks.httpClientMock.
					On("Do", mock.Anything).
					Return(&resp, nil).Once()
			}

			resp, err := cc.request(ctx, "/test", u, method, isAuthed, body)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatusCode, resp.StatusCode)
			}

			httpClientMock.AssertNumberOfCalls(t, "Do", tt.expectedAttempts)

			// Check if the request was properly formed
			lastCall := httpClientMock.Calls[len(httpClientMock.Calls)-1]
			lastReq := lastCall.Arguments[0].(*http.Request)
			assert.Equal(t, method, lastReq.Method)
			assert.Equal(t, u, lastReq.URL.String())
			assert.Equal(t, "Bearer test-key", lastReq.Header.Get("Authorization"))
			assert.Equal(t, "application/json", lastReq.Header.Get("Content-Type"))
		})
	}
}

func newClientWithMocks(t *testing.T) (Client, *clientMocks) {
	httpClientMock := httpclientMocks.NewHttpClientMock(t)
	tntManagerMock := tenant.NewTenantManagerMock(t)
	monitorSvcMock := monitorMocks.NewMockMonitorService(t)

	return Client{
			BasePath:       "http://localhost:8080",
			APIKey:         "test-key",
			httpClient:     httpClientMock,
			tenantManager:  tntManagerMock,
			monitorService: monitorSvcMock,
		}, &clientMocks{
			httpClientMock:     httpClientMock,
			tenantManagerMock:  tntManagerMock,
			monitorServiceMock: monitorSvcMock,
		}
}

type clientMocks struct {
	httpClientMock     *httpclientMocks.HttpClientMock
	tenantManagerMock  *tenant.TenantManagerMock
	monitorServiceMock *monitorMocks.MockMonitorService
}
