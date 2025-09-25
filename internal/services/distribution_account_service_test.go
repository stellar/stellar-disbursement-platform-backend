package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_DistributionAccountServiceOptions_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		opts          DistributionAccountServiceOptions
		expectedError string
	}{
		{
			name:          "🔴returns error if Horizon client is nil",
			opts:          DistributionAccountServiceOptions{},
			expectedError: "horizon client cannot be nil",
		},
		{
			name: "🔴returns error if Circle service is nil",
			opts: DistributionAccountServiceOptions{
				HorizonClient: &horizonclient.Client{},
			},
			expectedError: "circle service cannot be nil",
		},
		{
			name: "🔴returns error if network type is invalid",
			opts: DistributionAccountServiceOptions{
				HorizonClient: &horizonclient.Client{},
				CircleService: &circle.Service{},
				NetworkType:   "foobar",
			},
			expectedError: `validating network type: invalid network type "foobar"`,
		},
		{
			name: "🟢returns nil if all fields are valid",
			opts: DistributionAccountServiceOptions{
				HorizonClient: &horizonclient.Client{},
				CircleService: &circle.Service{},
				NetworkType:   utils.TestnetNetworkType,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_StellarDistributionAccountService_GetBalances(t *testing.T) {
	ctx := context.Background()
	accAddress := keypair.MustRandom().Address()
	distAcc := schema.NewStellarEnvTransactionAccount(accAddress)

	nativeAsset := data.Asset{Code: assets.XLMAssetCode, Issuer: ""}
	usdcAsset := data.Asset{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet}

	testCases := []struct {
		name                string
		expectedBalances    map[data.Asset]float64
		expectedError       error
		mockHorizonClientFn func(mHorizonClient *horizonclient.MockClient)
	}{
		{
			name: "🟢successfully gets balances",
			mockHorizonClientFn: func(mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{
					AccountID: distAcc.Address,
				}).Return(horizon.Account{
					Balances: []horizon.Balance{
						{
							Asset:   base.Asset{Code: usdcAsset.Code, Issuer: usdcAsset.Issuer},
							Balance: "100.0000000",
						},
						{
							Asset:   base.Asset{Code: nativeAsset.Code, Type: "native"},
							Balance: "100000.0000000",
						},
					},
				}, nil).Once()
			},
			expectedBalances: map[data.Asset]float64{
				usdcAsset:   100.0,
				nativeAsset: 100000.0,
			},
		},
		{
			name: "🔴returns error when horizon client request results in error",
			mockHorizonClientFn: func(mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{
					AccountID: distAcc.Address,
				}).Return(horizon.Account{}, fmt.Errorf("foobar")).Once()
			},
			expectedError: errors.New("getting details for account from Horizon: foobar"),
		},
		{
			name: "🔴returns error when attempting to parse invalid balance into float",
			mockHorizonClientFn: func(mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{
					AccountID: distAcc.Address,
				}).Return(horizon.Account{
					Balances: []horizon.Balance{
						{
							Asset:   base.Asset{Code: nativeAsset.Code, Type: "native"},
							Balance: "invalid_balance",
						},
					},
				}, nil).Once()
			},
			expectedError: errors.New("parsing balance to float"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mHorizonClient := horizonclient.MockClient{}
			svc := StellarDistributionAccountService{
				horizonClient: &mHorizonClient,
			}

			tc.mockHorizonClientFn(&mHorizonClient)
			balances, err := svc.GetBalances(ctx, &distAcc)
			if tc.expectedError != nil {
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedBalances, balances)
			}

			mHorizonClient.AssertExpectations(t)
		})
	}
}

func Test_StellarDistributionAccountService_GetBalance(t *testing.T) {
	ctx := context.Background()
	accAddress := keypair.MustRandom().Address()
	distAcc := schema.NewStellarEnvTransactionAccount(accAddress)

	nativeAsset := data.Asset{Code: assets.XLMAssetCode}
	usdcAsset := assets.USDCAssetTestnet
	eurcAsset := assets.EURCAssetTestnet

	mockSetup := func(mHorizonClient *horizonclient.MockClient) {
		mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: distAcc.Address,
		}).Return(horizon.Account{
			Balances: []horizon.Balance{
				{
					Asset:   base.Asset{Code: usdcAsset.Code, Issuer: usdcAsset.Issuer},
					Balance: "100.0000000",
				},
				{
					Asset:   base.Asset{Code: nativeAsset.Code, Type: "native"},
					Balance: "120.0000000",
				},
			},
		}, nil).Once()
	}

	testCases := []struct {
		name            string
		asset           data.Asset
		expectedBalance float64
		expectedError   error
	}{
		{
			name:            "🟢successfully gets balance for asset with issuer",
			asset:           usdcAsset,
			expectedBalance: 100.0,
		},
		{
			name:            "🟢successfully gets balance for native asset",
			asset:           nativeAsset,
			expectedBalance: 120.0,
		},
		{
			name:          "🔴returns error if asset is not found on account",
			asset:         eurcAsset,
			expectedError: fmt.Errorf("balance for asset %s not found for distribution account", eurcAsset),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mHorizonClient := horizonclient.MockClient{}
			svc := StellarDistributionAccountService{
				horizonClient: &mHorizonClient,
			}

			mockSetup(&mHorizonClient)
			balance, err := svc.GetBalance(ctx, &distAcc, tc.asset)
			if tc.expectedError != nil {
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedBalance, balance)
			}

			mHorizonClient.AssertExpectations(t)
		})
	}
}

func Test_CircleDistributionAccountService_GetBalances(t *testing.T) {
	ctx := context.Background()
	circleDistAcc := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	testCases := []struct {
		name             string
		networkType      utils.NetworkType
		account          schema.TransactionAccount
		prepareMocksFn   func(mCircleService *circle.MockService)
		expectedBalances map[data.Asset]float64
		expectedError    error
	}{
		{
			name:          "🔴returns an error if the account is not a Circle type",
			networkType:   utils.TestnetNetworkType,
			account:       schema.NewDefaultHostAccount("gost-account-address"),
			expectedError: errors.New("distribution account is not a Circle account"),
		},
		{
			name:        "🔴returns an error if the circle account is not ACTIVE",
			networkType: utils.TestnetNetworkType,
			account: schema.TransactionAccount{
				CircleWalletID: "circle-wallet-id",
				Type:           schema.DistributionAccountCircleDBVault,
				Status:         schema.AccountStatusPendingUserActivation,
			},
			expectedError: fmt.Errorf("this organization's distribution account is in %s state, please complete the %s activation process to access this endpoint", schema.AccountStatusPendingUserActivation, schema.CirclePlatform),
		},
		{
			name:        "🔴wrap error comming from GetWalletByID",
			networkType: utils.TestnetNetworkType,
			account:     circleDistAcc,
			prepareMocksFn: func(mCircleService *circle.MockService) {
				mCircleService.
					On("GetBusinessBalances", ctx).
					Return(nil, errors.New("foobar")).
					Once()
			},
			expectedError: errors.New("getting wallet by ID: foobar"),
		},
		{
			name:        "🟢[Testnet]successfully gets balances, ignoring the unsupported ones",
			networkType: utils.TestnetNetworkType,
			account:     circleDistAcc,
			prepareMocksFn: func(mCircleService *circle.MockService) {
				mCircleService.
					On("GetBusinessBalances", ctx).
					Return(&circle.Balances{
						Available: []circle.Balance{
							{Currency: "USD", Amount: "100.0"},
							{Currency: "EUR", Amount: "200.0"},
							{Currency: "UNSUPPORTED_ASSET", Amount: "300.0"},
						},
						Unsettled: []circle.Balance{},
					}, nil).
					Once()
			},
			expectedBalances: map[data.Asset]float64{
				assets.USDCAssetTestnet: 100.0,
				assets.EURCAssetTestnet: 200.0,
			},
		},
		{
			name:        "🟢[Pubnet]successfully gets balances, ignoring the unsupported ones",
			networkType: utils.PubnetNetworkType,
			account:     circleDistAcc,
			prepareMocksFn: func(mCircleService *circle.MockService) {
				mCircleService.
					On("GetBusinessBalances", ctx).
					Return(&circle.Balances{
						Available: []circle.Balance{
							{Currency: "USD", Amount: "100.0"},
							{Currency: "EUR", Amount: "200.0"},
							{Currency: "UNSUPPORTED_ASSET", Amount: "300.0"},
						},
					}, nil).
					Once()
			},
			expectedBalances: map[data.Asset]float64{
				assets.USDCAssetPubnet: 100.0,
				assets.EURCAssetPubnet: 200.0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := CircleDistributionAccountService{
				NetworkType: tc.networkType,
			}

			if tc.prepareMocksFn != nil {
				mCircleService := circle.NewMockService(t)
				svc.CircleService = mCircleService
				tc.prepareMocksFn(mCircleService)
			}

			balances, err := svc.GetBalances(ctx, &tc.account)
			if tc.expectedError != nil {
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedBalances, balances)
			}
		})
	}
}

func Test_CircleDistributionAccountService_GetBalance(t *testing.T) {
	ctx := context.Background()
	circleDistAcc := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}
	unsupportedAsset := data.Asset{Code: "FOO", Issuer: "GCANIBF4EHC5ZKKMSPX2WFGJ4ZO7BI4JFHZHBUQC5FH3JOOLKG7F5DL3"}
	mockGetGetBusinessBalancesFn := func(mCircleService *circle.MockService) {
		mCircleService.
			On("GetBusinessBalances", ctx).
			Return(&circle.Balances{
				Available: []circle.Balance{
					{Currency: "USD", Amount: "100.0"},
					{Currency: "EUR", Amount: "200.0"},
				},
			}, nil).
			Once()
	}

	testCases := []struct {
		name            string
		networkType     utils.NetworkType
		account         schema.TransactionAccount
		asset           data.Asset
		prepareMocksFn  func(mCircleService *circle.MockService)
		expectedBalance float64
		expectedError   error
	}{
		{
			name:          "🔴wrap error from GetBalances",
			networkType:   utils.TestnetNetworkType,
			account:       schema.NewDefaultHostAccount("gost-account-address"),
			asset:         assets.USDCAssetTestnet,
			expectedError: errors.New("distribution account is not a Circle account"),
		},
		{
			name:           "🔴returns an error if the desired asset could not be found",
			networkType:    utils.TestnetNetworkType,
			account:        circleDistAcc,
			asset:          unsupportedAsset,
			prepareMocksFn: mockGetGetBusinessBalancesFn,
			expectedError:  fmt.Errorf("balance for asset %v not found for distribution account", unsupportedAsset),
		},
		{
			name:            "🟢[Testnet]successfully gets balance for supported asset USDC",
			networkType:     utils.TestnetNetworkType,
			account:         circleDistAcc,
			asset:           assets.USDCAssetTestnet,
			prepareMocksFn:  mockGetGetBusinessBalancesFn,
			expectedBalance: 100.0,
		},
		{
			name:            "🟢[Pubnet]successfully gets balance for supported asset EURC",
			networkType:     utils.PubnetNetworkType,
			account:         circleDistAcc,
			asset:           assets.EURCAssetPubnet,
			prepareMocksFn:  mockGetGetBusinessBalancesFn,
			expectedBalance: 200.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := CircleDistributionAccountService{
				NetworkType: tc.networkType,
			}

			if tc.prepareMocksFn != nil {
				mCircleService := circle.NewMockService(t)
				svc.CircleService = mCircleService
				tc.prepareMocksFn(mCircleService)
			}

			// Create some noise by injecting extra fields in the asset boject, to check if the service is (correctly) ignoring them.
			now := time.Now()
			assetWithExtraFields := data.Asset{
				Code:      tc.asset.Code,
				Issuer:    tc.asset.Issuer,
				ID:        "asset-id",
				CreatedAt: &now,
				UpdatedAt: &now,
				DeletedAt: &now,
			}

			balance, err := svc.GetBalance(ctx, &tc.account, assetWithExtraFields)
			if tc.expectedError != nil {
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedBalance, balance)
			}
		})
	}
}

func Test_NewDistributionAccountService(t *testing.T) {
	mHorizonClient := &horizonclient.MockClient{}
	mCircleService := &circle.MockService{}
	svcOpts := DistributionAccountServiceOptions{
		HorizonClient: mHorizonClient,
		NetworkType:   utils.TestnetNetworkType,
		CircleService: mCircleService,
	}
	svc, err := NewDistributionAccountService(svcOpts)
	require.NoError(t, err)

	stellarDistributionAccSvc := &StellarDistributionAccountService{
		horizonClient: mHorizonClient,
	}
	circleDistributionAccSvc := &CircleDistributionAccountService{
		CircleService: mCircleService,
		NetworkType:   utils.TestnetNetworkType,
	}

	testCases := []struct {
		accountType schema.AccountType
		expectedSvc DistributionAccountServiceInterface
	}{
		{
			accountType: schema.DistributionAccountStellarEnv,
			expectedSvc: stellarDistributionAccSvc,
		},
		{
			accountType: schema.DistributionAccountStellarDBVault,
			expectedSvc: stellarDistributionAccSvc,
		},
		{
			accountType: schema.DistributionAccountCircleDBVault,
			expectedSvc: circleDistributionAccSvc,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.accountType), func(t *testing.T) {
			actualSvc, ok := svc.strategies[tc.accountType]
			assert.True(t, ok)
			assert.Equal(t, tc.expectedSvc, actualSvc)
		})
	}
}
