package circle

import (
	"errors"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// Balance represents the amount and currency of a balance or transfer.
type Balance struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

var AllowedAssetsMap = map[string]map[utils.NetworkType]data.Asset{
	"USD": {
		utils.PubnetNetworkType:  assets.USDCAssetPubnet,
		utils.TestnetNetworkType: assets.USDCAssetTestnet,
	},
	"EUR": {
		utils.PubnetNetworkType:  assets.EURCAssetPubnet,
		utils.TestnetNetworkType: assets.EURCAssetTestnet,
	},
}

var (
	ErrUnsupportedCurrency           = errors.New("unsupported Circle currency code")
	ErrUnsupportedCurrencyForNetwork = errors.New("unsupported Circle currency code for this network type")
)

// ParseStellarAsset returns the Stellar asset for the given Circle currency code, or an error if the currency is not supported in the SDP.
func ParseStellarAsset(circleCurrency string, networkType utils.NetworkType) (data.Asset, error) {
	return ParseStellarAssetFromAllowlist(circleCurrency, networkType, AllowedAssetsMap)
}

func ParseStellarAssetFromAllowlist(circleCurrency string, networkType utils.NetworkType, allowedAssetsMap map[string]map[utils.NetworkType]data.Asset) (data.Asset, error) {
	assetByNetworkType, ok := allowedAssetsMap[circleCurrency]
	if !ok {
		return data.Asset{}, ErrUnsupportedCurrency
	}

	asset, ok := assetByNetworkType[networkType]
	if !ok {
		return data.Asset{}, ErrUnsupportedCurrencyForNetwork
	}

	return asset, nil
}
