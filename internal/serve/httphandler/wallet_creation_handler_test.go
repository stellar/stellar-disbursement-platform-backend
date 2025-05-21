package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_WalletCreationHandler_CreateWallet(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:     "123",
		PublicKey: "04f5",
	})

	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	walletService.On("CreateWallet", ctx, "test-tenant-id", "123", "04f5").Return(nil)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var respBody WalletResponse
	err := json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)

	assert.Equal(t, data.PendingWalletStatus, respBody.Status)
	assert.Empty(t, respBody.ContractAddress)
}

func Test_WalletCreationHandler_CreateWallet_ValidationErrors(t *testing.T) {
	errorCases := []struct {
		name          string
		requestBody   CreateWalletRequest
		expectedField string
	}{
		{
			name: "empty token",
			requestBody: CreateWalletRequest{
				Token:     "",
				PublicKey: "04f5",
			},
			expectedField: "token",
		},
		{
			name: "empty public key",
			requestBody: CreateWalletRequest{
				Token:     "123",
				PublicKey: "",
			},
			expectedField: "public_key",
		},
		{
			name: "invalid public key",
			requestBody: CreateWalletRequest{
				Token:     "123",
				PublicKey: "invalid_key",
			},
			expectedField: "public_key",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			walletService := mocks.NewMockEmbeddedWalletService(t)
			handler := WalletCreationHandler{
				embeddedWalletService: walletService,
			}

			rr := httptest.NewRecorder()
			requestBody, _ := json.Marshal(tc.requestBody)

			req, _ := http.NewRequest(http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
			http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
			var errResp map[string]any
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&errResp))
			fields := errResp["extras"].(map[string]any)
			assert.Contains(t, fields, tc.expectedField)
		})
	}
}

func Test_WalletCreationHandler_CreateWallet_InternalError(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:     "123",
		PublicKey: "04f5",
	})

	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	walletService.On("CreateWallet", ctx, "test-tenant-id", "123", "04f5").Return(errors.New("foobar"))

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_CreateWallet_InvalidToken(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:     "123",
		PublicKey: "04f5",
	})

	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	walletService.On("CreateWallet", ctx, "test-tenant-id", "123", "04f5").Return(services.ErrInvalidToken)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_CreateWallet_InvalidStatus(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:     "123",
		PublicKey: "04f5",
	})

	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	walletService.On("CreateWallet", ctx, "test-tenant-id", "123", "04f5").Return(services.ErrCreateWalletInvalidStatus)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_CreateWallet_MissingTenant(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:     "123",
		PublicKey: "04f5",
	})

	req, _ := http.NewRequest(http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_GetWallet(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}
	rr := httptest.NewRecorder()
	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	walletService.On("GetWallet", ctx, "test-tenant-id", "123").Return(&data.EmbeddedWallet{
		ContractAddress: "contract-address",
		WalletStatus:    data.PendingWalletStatus,
	}, nil)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/embedded-wallet/status?token=123", nil)
	http.HandlerFunc(handler.GetWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
	var respBody WalletResponse
	err := json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)

	assert.Equal(t, "contract-address", respBody.ContractAddress)
	assert.Equal(t, data.PendingWalletStatus, respBody.Status)
}

func Test_WalletCreationHandler_GetWallet_MissingToken(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/embedded-wallet/status", nil)
	http.HandlerFunc(handler.GetWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_GetWallet_InvalidToken(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	walletService.On("GetWallet", ctx, "test-tenant-id", "123").Return(nil, services.ErrInvalidToken)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/embedded-wallet/status?token=123", nil)
	http.HandlerFunc(handler.GetWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_GetWallet_InternalError(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tenant.Tenant{
		ID: "test-tenant-id",
	})

	walletService.On("GetWallet", ctx, "test-tenant-id", "123").Return(nil, errors.New("foobar"))

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/embedded-wallet/status?token=123", nil)
	http.HandlerFunc(handler.GetWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_GetWallet_MissingTenant(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		embeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()

	req, _ := http.NewRequest(http.MethodGet, "/embedded-wallet/status?token=123", nil)
	http.HandlerFunc(handler.GetWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
}
