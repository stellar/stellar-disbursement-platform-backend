package circle

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// Environment holds the possible environments for the Circle API.
type Environment string

const (
	Production Environment = "https://api.circle.com"
	Sandbox    Environment = "https://api-sandbox.circle.com"
)

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
