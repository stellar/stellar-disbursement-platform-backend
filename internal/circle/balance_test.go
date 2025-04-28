package circle

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_ParseStellarAsset(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		circleCurrency   string
		networkType      utils.NetworkType
		allowedAssetsMap map[string]map[utils.NetworkType]data.Asset
		expectedAsset    data.Asset
		expectedError    error
	}{
		{
			name:             "[Pubnet] USDC",
			circleCurrency:   "USD",
			networkType:      utils.PubnetNetworkType,
			allowedAssetsMap: AllowedAssetsMap,
			expectedAsset:    assets.USDCAssetPubnet,
			expectedError:    nil,
		},
		{
			name:             "[Testnet] USDC",
			circleCurrency:   "USD",
			networkType:      utils.TestnetNetworkType,
			allowedAssetsMap: AllowedAssetsMap,
			expectedAsset:    assets.USDCAssetTestnet,
			expectedError:    nil,
		},
		{
			name:             "[Pubnet] EUR",
			circleCurrency:   "EUR",
			networkType:      utils.PubnetNetworkType,
			allowedAssetsMap: AllowedAssetsMap,
			expectedAsset:    assets.EURCAssetPubnet,
			expectedError:    nil,
		},
		{
			name:             "[Testnet] EUR",
			circleCurrency:   "EUR",
			networkType:      utils.TestnetNetworkType,
			allowedAssetsMap: AllowedAssetsMap,
			expectedAsset:    assets.EURCAssetTestnet,
			expectedError:    nil,
		},
		{
			name:             "Unsupported currency",
			circleCurrency:   "JPY",
			networkType:      utils.PubnetNetworkType,
			allowedAssetsMap: AllowedAssetsMap,
			expectedAsset:    data.Asset{},
			expectedError:    ErrUnsupportedCurrency,
		},
		{
			name:           "Unsupported currency for network type",
			circleCurrency: "JPY",
			networkType:    utils.PubnetNetworkType,
			allowedAssetsMap: map[string]map[utils.NetworkType]data.Asset{
				"JPY": {},
			},
			expectedAsset: data.Asset{},
			expectedError: ErrUnsupportedCurrencyForNetwork,
		},
	}

	for _, tc := range testCases {
		testCases := tc
		t.Run(testCases.name, func(t *testing.T) {
			t.Parallel()

			if !assert.ObjectsAreEqual(testCases.allowedAssetsMap, AllowedAssetsMap) {
				return
			}
			asset, err := ParseStellarAsset(testCases.circleCurrency, testCases.networkType)

			if testCases.expectedError == nil {
				assert.NoError(t, err)
				assert.Equal(t, testCases.expectedAsset, asset)
			} else {
				assert.Equal(t, testCases.expectedError, err)
			}
		})

		t.Run("FromAllowlist/"+testCases.name, func(t *testing.T) {
			asset, err := parseStellarAssetFromAllowlist(testCases.circleCurrency, testCases.networkType, testCases.allowedAssetsMap)

			if testCases.expectedError == nil {
				assert.NoError(t, err)
				assert.Equal(t, testCases.expectedAsset, asset)
			} else {
				assert.Equal(t, testCases.expectedError, err)
			}
		})
	}
}
