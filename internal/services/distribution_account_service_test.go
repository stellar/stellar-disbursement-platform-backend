package services

import (
	"context"
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
	mHorizonClient := horizonclient.MockClient{}
	svc := NewStellarNativeDistributionAccountService(&mHorizonClient)

	ctx := context.Background()
	accAddress := keypair.MustRandom().Address()
	distAcc := schema.NewDefaultStellarDistributionAccount(accAddress)

	nativeAsset := data.Asset{Code: "XLM", Issuer: ""}
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
							Asset:   base.Asset{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet},
							Balance: "100.0000000",
						},
						{
							Asset:   base.Asset{Code: "XLM", Type: "native"},
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
			name: "🔴returns error when horizon client response contains duplicate balance entries for the same asset",
			mockHorizonClientFn: func(mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{
					AccountID: distAcc.Address,
				}).Return(horizon.Account{
					Balances: []horizon.Balance{
						{
							Asset:   base.Asset{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet},
							Balance: "100.0000000",
						},
						{
							Asset:   base.Asset{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet},
							Balance: "101.0000000",
						},
					},
				}, nil).Once()
			},
			expectedError: fmt.Errorf(
				"duplicate balance for asset %s:%s found for distribution account",
				assets.USDCAssetCode,
				assets.USDCAssetIssuerTestnet),
		},
		{
			name: "🔴returns error when horizon client request results in error",
			mockHorizonClientFn: func(mHorizonClient *horizonclient.MockClient) {
				mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{
					AccountID: distAcc.Address,
				}).Return(horizon.Account{}, fmt.Errorf("foobar")).Once()
			},
			expectedError: errors.New("cannot get details for account from Horizon: foobar"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockHorizonClientFn(&mHorizonClient)
			balances, err := svc.GetBalances(ctx, distAcc)
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
	mHorizonClient := horizonclient.MockClient{}
	svc := NewStellarNativeDistributionAccountService(&mHorizonClient)

	ctx := context.Background()
	accAddress := keypair.MustRandom().Address()
	distAcc := schema.NewDefaultStellarDistributionAccount(accAddress)

	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{
		AccountID: distAcc.Address,
	}).Return(horizon.Account{
		Balances: []horizon.Balance{
			{
				Asset:   base.Asset{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet},
				Balance: "100.0000000",
			},
			{
				Asset:   base.Asset{Code: assets.XLMAssetCode, Type: "native"},
				Balance: "120.0000000",
			},
		},
	}, nil)

	nativeAsset := data.Asset{Code: "XLM", Issuer: ""}
	usdcAsset := data.Asset{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet}
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
			expectedError: fmt.Errorf("balance for asset %s:%s not found for distribution account", eurcAsset.Code, eurcAsset.Issuer),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			balance, err := svc.GetBalance(ctx, distAcc, tc.asset)
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
	mHorizonClient := horizonclient.MockClient{}
	svc := NewDistributionAccountService(&mHorizonClient)

	t.Run("maps the correct distribution account type to the correct service implementation", func(t *testing.T) {
		stellarNativeSvc := svc.strategies[schema.DistributionAccountTypeDBVaultStellar]
		assert.Equal(t, stellarNativeSvc, svc.strategies[schema.DistributionAccountTypeEnvStellar])
	})
}
