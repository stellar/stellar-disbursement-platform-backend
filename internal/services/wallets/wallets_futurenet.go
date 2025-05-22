package wallets

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
)

var FuturenetWallets = []data.Wallet{
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
		Name:        "User Managed Wallet",
		Assets:      assets.AllAssetsTestnet,
		UserManaged: true,
	},
	{
		Name:     "SDP Embedded Wallet",
		Assets:   assets.AllAssetsTestnet,
		Embedded: true,
	},
	{
		Name:           "SDP Embedded Wallet",
		DeepLinkSchema: "https://stellar.org",
		Homepage:       "https://stellar.org",
		Assets:         assets.AllAssetsTestnet,
		Embedded:       true,
	},
}
