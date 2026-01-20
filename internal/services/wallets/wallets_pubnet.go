package wallets

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
)

var PubnetWallets = []data.Wallet{
	{
		Name:              "Decaf",
		Homepage:          "https://decaf.so",
		DeepLinkSchema:    "https://decafwallet.app.link",
		SEP10ClientDomain: "decaf.so",
		Assets: []data.Asset{
			assets.XLMAsset,
			assets.USDCAssetPubnet,
		},
	},
	{
		Name:              "Vesseo",
		Homepage:          "https://vesseoapp.com",
		DeepLinkSchema:    "https://vesseoapp.com/disbursement",
		SEP10ClientDomain: "vesseoapp.com",
		Assets: []data.Asset{
			assets.XLMAsset,
			assets.USDCAssetPubnet,
		},
	},
	{
		Name:              "Beans App",
		Homepage:          "https://beansapp.com",
		DeepLinkSchema:    "https://www.beansapp.com/disbursements/registration?env=prod",
		SEP10ClientDomain: "api.beansapp.com",
		Assets: []data.Asset{
			assets.XLMAsset,
			assets.USDCAssetPubnet,
			assets.EURCAssetPubnet,
		},
	},
	{
		Name:           "Embedded Wallet",
		DeepLinkSchema: "SELF",
		Homepage:       "https://stellar.org",
		Assets: []data.Asset{
			assets.XLMAsset,
			assets.USDCAssetPubnet,
			assets.EURCAssetPubnet,
		},
		Embedded: true,
	},
	{
		Name:        "User Managed Wallet",
		Assets:      assets.AllAssetsPubnet,
		UserManaged: true,
	},
}
