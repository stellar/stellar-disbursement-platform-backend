package services

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_StellarNativeDistributionAccount_GetBalances(t *testing.T) {
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
			svc := StellarNativeDistributionAccountService{
				horizonClient: &mHorizonClient,
			}

			tc.mockHorizonClientFn(&mHorizonClient)
			balances, err := svc.GetBalances(&distAcc)
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

func Test_StellarNativeDistributionAccount_GetBalance(t *testing.T) {
	accAddress := keypair.MustRandom().Address()
	distAcc := schema.NewStellarEnvTransactionAccount(accAddress)

	nativeAsset := data.Asset{Code: assets.XLMAssetCode}
	usdcAsset := data.Asset{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet}

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

	eurcIssuer := keypair.MustRandom().Address()
	eurcAsset := data.Asset{Code: "EURC", Issuer: eurcIssuer}

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
			svc := StellarNativeDistributionAccountService{
				horizonClient: &mHorizonClient,
			}

			mockSetup(&mHorizonClient)
			balance, err := svc.GetBalance(&distAcc, tc.asset)
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

func Test_NewDistributionAccountService(t *testing.T) {
	mHorizonClient := horizonclient.MockClient{}
	svcOpts := DistributionAccountServiceOptions{
		HorizonClient: &mHorizonClient,
	}
	svc := NewDistributionAccountService(svcOpts)

	t.Run("maps the correct distribution account type to the correct service implementation", func(t *testing.T) {
		targetSvc, ok := svc.strategies[schema.DistributionAccountStellarDBVault]
		assert.True(t, ok)
		assert.Equal(t, targetSvc, svc.strategies[schema.DistributionAccountStellarDBVault])

		targetSvc, ok = svc.strategies[schema.DistributionAccountStellarEnv]
		assert.True(t, ok)
		assert.Equal(t, targetSvc, svc.strategies[schema.DistributionAccountStellarEnv])

		// TODO [SDP-1232]: Change this when Circle distribution account service is added
		_, ok = svc.strategies[schema.DistributionAccountCircleDBVault]
		assert.False(t, ok)
	})
}
