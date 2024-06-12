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
	circleMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/circle/mocks"
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
		prepareMocks     func(t *testing.T, mCircleClient *circleMocks.MockClient, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver)
		expectedStatus   int
		expectedResponse string
	}{
		{
			name: "returns a 500 error in DistributionAccountResolver",
			prepareMocks: func(t *testing.T, mCircleClient *circleMocks.MockClient, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{}, errors.New("distribution account error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error":"Cannot retrieve distribution account"}`,
		},
		{
			name: "returns a 400 error if the distribution account is not Circle",
			prepareMocks: func(t *testing.T, mCircleClient *circleMocks.MockClient, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
					Once()
			},
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: fmt.Sprintf(`{"error":"This endpoint is only available for tenants using %v"}`, schema.CirclePlatform),
		},
		{
			name: "propagate Circle API error if GetWalletByID fails",
			prepareMocks: func(t *testing.T, mCircleClient *circleMocks.MockClient, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:    schema.DistributionAccountCircleDBVault,
						Address: "circle-wallet-id",
						Status:  schema.AccountStatusActive,
					}, nil).
					Once()
				mCircleClient.
					On("GetWalletByID", mock.Anything, "circle-wallet-id").
					Return(nil, fmt.Errorf("wrapped error: %w", circleAPIError)).
					Once()
			},
			expectedStatus: circleAPIError.Code,
			expectedResponse: `{
				"error": "Cannot retrieve Circle wallet: some circle error",
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
			name: "returns a 500 if circle.GetWalletByID fails with an unexpected error",
			prepareMocks: func(t *testing.T, mCircleClient *circleMocks.MockClient, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:    schema.DistributionAccountCircleDBVault,
						Address: "circle-wallet-id",
						Status:  schema.AccountStatusActive,
					}, nil).
					Once()
				mCircleClient.
					On("GetWalletByID", mock.Anything, "circle-wallet-id").
					Return(nil, errors.New("unexpected error")).
					Once()
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedResponse: `{"error": "Cannot retrieve Circle wallet"}`,
		},
		{
			name:        "[Testnet] 🎉 successfully returns balances",
			networkType: utils.TestnetNetworkType,
			prepareMocks: func(t *testing.T, mCircleClient *circleMocks.MockClient, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:    schema.DistributionAccountCircleDBVault,
						Address: "circle-wallet-id",
						Status:  schema.AccountStatusActive,
					}, nil).
					Once()
				mCircleClient.
					On("GetWalletByID", mock.Anything, "circle-wallet-id").
					Return(&circle.Wallet{
						WalletID:    "test-id",
						EntityID:    "2f47c999-9022-4939-acea-dc3afa9ccbaf",
						Type:        "end_user_wallet",
						Description: "Treasury Wallet",
						Balances: []circle.Balance{
							{Amount: "123.00", Currency: "USD"},
						},
					}, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"account": {
					"address": "circle-wallet-id",
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
			prepareMocks: func(t *testing.T, mCircleClient *circleMocks.MockClient, mDistributionAccountResolver *sigMocks.MockDistributionAccountResolver) {
				mDistributionAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{
						Type:    schema.DistributionAccountCircleDBVault,
						Address: "circle-wallet-id",
						Status:  schema.AccountStatusActive,
					}, nil).
					Once()
				mCircleClient.
					On("GetWalletByID", mock.Anything, "circle-wallet-id").
					Return(&circle.Wallet{
						WalletID:    "test-id",
						EntityID:    "2f47c999-9022-4939-acea-dc3afa9ccbaf",
						Type:        "end_user_wallet",
						Description: "Treasury Wallet",
						Balances: []circle.Balance{
							{Amount: "123.00", Currency: "USD"},
						},
					}, nil).
					Once()
			},
			expectedStatus: http.StatusOK,
			expectedResponse: `{
				"account": {
					"address": "circle-wallet-id",
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
			mCircleClient := circleMocks.NewMockClient(t)
			tc.prepareMocks(t, mCircleClient, mDistributionAccountResolver)
			h := BalancesHandler{
				DistributionAccountResolver: mDistributionAccountResolver,
				NetworkType:                 tc.networkType,
				CircleClientFactory: func(env circle.Environment, apiKey string) circle.ClientInterface {
					return mCircleClient
				},
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
		circleWallet     *circle.Wallet
		expectedBalances []Balance
	}{
		{
			name:        "[Pubnet] only supported assets are included",
			networkType: utils.PubnetNetworkType,
			circleWallet: &circle.Wallet{
				Balances: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
					{Currency: "USD", Amount: "200"},
					{Currency: "BAR", Amount: "300"},
				},
			},
			expectedBalances: []Balance{
				{
					Amount:      "200",
					AssetCode:   assets.USDCAssetPubnet.Code,
					AssetIssuer: assets.USDCAssetPubnet.Issuer,
				},
			},
		},
		{
			name:        "[Testnet] only supported assets are included",
			networkType: utils.TestnetNetworkType,
			circleWallet: &circle.Wallet{
				Balances: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
					{Currency: "USD", Amount: "200"},
					{Currency: "BAR", Amount: "300"},
				},
			},
			expectedBalances: []Balance{
				{
					Amount:      "200",
					AssetCode:   assets.USDCAssetTestnet.Code,
					AssetIssuer: assets.USDCAssetTestnet.Issuer,
				},
			},
		},
		{
			name:        "[Pubnet] none of the provided assets is supported",
			networkType: utils.PubnetNetworkType,
			circleWallet: &circle.Wallet{
				Balances: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
				},
			},
			expectedBalances: []Balance{},
		},
		{
			name:        "[Testnet] none of the provided assets is supported",
			networkType: utils.TestnetNetworkType,
			circleWallet: &circle.Wallet{
				Balances: []circle.Balance{
					{Currency: "FOO", Amount: "100"},
				},
			},
			expectedBalances: []Balance{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := BalancesHandler{NetworkType: tc.networkType}

			actualBalances := h.filterBalances(ctx, tc.circleWallet)

			assert.Equal(t, tc.expectedBalances, actualBalances)
		})
	}
}
