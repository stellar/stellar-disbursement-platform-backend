package httphandler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_BalancesHandler_Get(t *testing.T) {
	circleAPIError := &circle.APIError{
		Code:    400,
		Message: "some circle error",
		Errors: []circle.APIErrorDetail{
			{
				Error:    "some error",
				Message:  "some message",
				Location: "some location",
			},
		},
	}

	testCases := []struct {
		name             string
		networkType      utils.NetworkType
		prepareMocks     func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver)
		expectedStatus   int
		expectedResponse string
	}{
		{
			name:        "returns a 500 error in DistributionAccountResolver",
			networkType: utils.TestnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{}, errors.New("distribution account error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error":"Cannot retrieve distribution account"}`,
		},
		{
			name:        "returns a 400 error if the distribution account is not Circle",
			networkType: utils.TestnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: fmt.Sprintf(`{"error":"This endpoint is only available for tenants using %v"}`, schema.CirclePlatform),
		},
		{
			name:        "propagate Circle API error if GetWalletByID fails",
			networkType: utils.TestnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:           schema.DistributionAccountCircleDBVault,
						CircleWalletID: "circle-wallet-id",
						Status:         schema.AccountStatusActive,
					}, nil).
					Once()

				mCircleService.
					On("GetBusinessBalances", mock.Anything).
					Return(nil, fmt.Errorf("wrapped error: %w", circleAPIError)).
					Once()
			},
			expectedStatus: circleAPIError.Code,
			expectedResponse: `{
				"error": "Cannot complete Circle request: some circle error",
				"extras": {
					"circle_errors": [
						{
							"error": "some error",
							"message": "some message",
							"location": "some location"
						}
					]
				}
			}`,
		},
		{
			name:        "returns a 400 if account status is PENDING_USER_ACTIVATION",
			networkType: utils.TestnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:   schema.DistributionAccountCircleDBVault,
						Status: schema.AccountStatusPendingUserActivation,
					}, nil).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: `{"error": "This organization's distribution account is in PENDING_USER_ACTIVATION state, please complete the CIRCLE activation process to access this endpoint."}`,
		},
		{
			name:        "returns a 500 if circle.GetWalletByID fails with an unexpected error",
			networkType: utils.TestnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:           schema.DistributionAccountCircleDBVault,
						CircleWalletID: "circle-wallet-id",
						Status:         schema.AccountStatusActive,
					}, nil).
					Once()
				mCircleService.
					On("GetBusinessBalances", mock.Anything).
					Return(nil, errors.New("unexpected error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Cannot complete Circle request"}`,
		},
		{
			name:        "[Testnet] 🎉 successfully returns balances",
			networkType: utils.TestnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:           schema.DistributionAccountCircleDBVault,
						CircleWalletID: "circle-wallet-id",
						Status:         schema.AccountStatusActive,
					}, nil).
					Once()
				mCircleService.
					On("GetBusinessBalances", mock.Anything).
					Return(&circle.Balances{
						Available: []circle.Balance{
							{Amount: "123.00", Currency: "USD"},
						},
						Unsettled: []circle.Balance{},
					}, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"account": {
					"circle_wallet_id": "circle-wallet-id",
					"type": "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT",
					"status": "ACTIVE"
				},
				"balances": [{
					"amount": "123.00",
					"asset_code": "USDC",
					"asset_issuer": "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
				}]
			}`,
		},
		{
			name:        "[Pubnet] 🎉 successfully returns balances",
			networkType: utils.PubnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleService *circle.MockService, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:           schema.DistributionAccountCircleDBVault,
						CircleWalletID: "circle-wallet-id",
						Status:         schema.AccountStatusActive,
					}, nil).
					Once()

				mCircleService.
					On("GetBusinessBalances", mock.Anything).
					Return(&circle.Balances{
						Available: []circle.Balance{
							{Amount: "123.00", Currency: "USD"},
						},
						Unsettled: []circle.Balance{},
					}, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"account": {
					"circle_wallet_id": "circle-wallet-id",
					"type": "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT",
					"status": "ACTIVE"
				},
				"balances": [{
					"amount": "123.00",
					"asset_code": "USDC",
					"asset_issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mDistributionAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
			mCircleService := circle.NewMockService(t)
			tc.prepareMocks(t, mCircleService, mDistributionAccountResolver)

			h := BalancesHandler{
				DistributionAccountResolver: mDistributionAccountResolver,
				NetworkType:                 tc.networkType,
				CircleService:               mCircleService,
			}

			rr := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/balances", nil)
			require.NoError(t, err)
			http.HandlerFunc(h.Get).ServeHTTP(rr, req)
			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			assert.JSONEq(t, tc.expectedResponse, string(respBody))
		})
	}
}

func Test_BalancesHandler_filterBalances(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name             string
		networkType      utils.NetworkType
		circleBalances   *circle.Balances
		expectedBalances []Balance
	}{
		{
			name:        "[Pubnet] only supported assets are included",
			networkType: utils.PubnetNetworkType,
			circleBalances: &circle.Balances{
				Available: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
					{Currency: "USD", Amount: "200"},
					{Currency: "BAR", Amount: "300"},
					{Currency: "EUR", Amount: "400"},
				},
			},
			expectedBalances: []Balance{
				{
					Amount:      "200",
					AssetCode:   assets.USDCAssetPubnet.Code,
					AssetIssuer: assets.USDCAssetPubnet.Issuer,
				},
				{
					Amount:      "400",
					AssetCode:   assets.EURCAssetPubnet.Code,
					AssetIssuer: assets.EURCAssetPubnet.Issuer,
				},
			},
		},
		{
			name:        "[Testnet] only supported assets are included",
			networkType: utils.TestnetNetworkType,
			circleBalances: &circle.Balances{
				Available: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
					{Currency: "USD", Amount: "200"},
					{Currency: "BAR", Amount: "300"},
					{Currency: "EUR", Amount: "400"},
				},
			},
			expectedBalances: []Balance{
				{
					Amount:      "200",
					AssetCode:   assets.USDCAssetTestnet.Code,
					AssetIssuer: assets.USDCAssetTestnet.Issuer,
				},
				{
					Amount:      "400",
					AssetCode:   assets.EURCAssetTestnet.Code,
					AssetIssuer: assets.EURCAssetTestnet.Issuer,
				},
			},
		},
		{
			name:        "[Pubnet] none of the provided assets is supported",
			networkType: utils.PubnetNetworkType,
			circleBalances: &circle.Balances{
				Available: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
				},
			},
			expectedBalances: []Balance{},
		},
		{
			name:        "[Testnet] none of the provided assets is supported",
			networkType: utils.TestnetNetworkType,
			circleBalances: &circle.Balances{
				Available: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
				},
			},
			expectedBalances: []Balance{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := BalancesHandler{NetworkType: tc.networkType}

			actualBalances := h.filterBalances(ctx, tc.circleBalances.Available)

			assert.Equal(t, tc.expectedBalances, actualBalances)
		})
	}
}

func Test_wrapCircleError(t *testing.T) {
	circleAPIError := &circle.APIError{
		Code:    400,
		Message: "some circle error",
		Errors: []circle.APIErrorDetail{
			{
				Error:    "some error",
				Message:  "some message",
				Location: "some location",
			},
		},
	}

	ctx := context.Background()
	testCases := []struct {
		name          string
		err           error
		wantHTTPError *httperror.HTTPError
	}{
		{
			name:          "nil error",
			err:           nil,
			wantHTTPError: nil,
		},
		{
			name:          "unexpected error",
			err:           errors.New("unexpected error"),
			wantHTTPError: httperror.InternalError(ctx, "Cannot complete Circle request", errors.New("unexpected error"), nil),
		},
		{
			name:          "circle.APIError",
			err:           circleAPIError,
			wantHTTPError: httperror.BadRequest("Cannot complete Circle request: some circle error", circleAPIError, map[string]interface{}{"circle_errors": circleAPIError.Errors}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualHTTPError := wrapCircleError(ctx, tc.err)
			assert.Equal(t, tc.wantHTTPError, actualHTTPError)
		})
	}
}
