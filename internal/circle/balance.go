package circle

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

var (
	ErrUnsupportedCurrency           = errors.New("unsupported Circle currency code")
	ErrUnsupportedCurrencyForNetwork = errors.New("unsupported Circle currency code for this network type")
)

// ListBusinessBalancesResponse represents the response containing business balances.
type ListBusinessBalancesResponse struct {
	Data Balances `json:"data,omitempty"`
}

// Balances represents the available and unsettled balances for different currencies.
type Balances struct {
	Available []Balance `json:"available"`
	Unsettled []Balance `json:"unsettled"`
}

// Balance represents the amount and currency of a balance or transfer.
type Balance struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// AllowedAssetsMap is a map of Circle currency codes to Stellar assets, for each network type.
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

// ParseStellarAsset returns the Stellar asset for the given Circle currency code, or an error if the currency is not
// supported in the SDP.
func ParseStellarAsset(circleCurrency string, networkType utils.NetworkType) (data.Asset, error) {
	return parseStellarAssetFromAllowlist(circleCurrency, networkType, AllowedAssetsMap)
}

// parseStellarAssetFromAllowlist returns the Stellar asset for the given Circle currency code, or an error if the
// currency is not supported in the SDP. This function allows for the use of a custom asset allowlist.
func parseStellarAssetFromAllowlist(circleCurrency string, networkType utils.NetworkType, allowedAssetsMap map[string]map[utils.NetworkType]data.Asset) (data.Asset, error) {
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

// parseBusinessBalancesResponse parses the response from the Circle API into a Balances struct.
func parseBusinessBalancesResponse(resp *http.Response) (*Balances, error) {
	var balancesResponse ListBusinessBalancesResponse
	if err := json.NewDecoder(resp.Body).Decode(&balancesResponse); err != nil {
		return nil, fmt.Errorf("unmarshalling Circle HTTP response: %w", err)
	}

	return &balancesResponse.Data, nil
}
