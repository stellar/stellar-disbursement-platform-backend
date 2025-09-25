package wallets

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
)

var TestnetWallets = []data.Wallet{
	{
		Name:              "Demo Wallet",
		Homepage:          "https://demo-wallet.stellar.org",
		DeepLinkSchema:    "https://demo-wallet.stellar.org",
		SEP10ClientDomain: "demo-wallet-server.stellar.org",
		Assets: []data.Asset{
			assets.USDCAssetTestnet,
			assets.XLMAsset,
		},
	},
	{
		Name:              "Vibrant Assist",
		Homepage:          "https://vibrantapp.com/vibrant-assist",
		DeepLinkSchema:    "https://vibrantapp.com/sdp-dev",
		SEP10ClientDomain: "api-dev.vibrantapp.com",
		Assets: []data.Asset{
			assets.USDCAssetTestnet,
		},
	},
	{
		Name:        "User Managed Wallet",
		Assets:      assets.AllAssetsTestnet,
		UserManaged: true,
	},
	{
		Name:           "SDP Embedded Wallet",
		DeepLinkSchema: "http://localhost:3000/invite",
		Homepage:       "http://localhost:3000",
		Assets:         assets.AllAssetsTestnet,
		Embedded:       true,
	},
}
