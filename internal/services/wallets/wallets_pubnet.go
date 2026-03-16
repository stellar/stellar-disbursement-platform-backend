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
			assets.USDCAssetPubnet,
		},
	},
	{
		Name:              "Vibrant Assist",
		Homepage:          "https://vibrantapp.com/vibrant-assist",
		DeepLinkSchema:    "https://vibrantapp.com/sdp",
		SEP10ClientDomain: "vibrantapp.com",
		Assets: []data.Asset{
			assets.USDCAssetPubnet,
		},
	},
	{
		Name:              "Vibrant Assist RC",
		Homepage:          "vibrantapp.com/vibrant-assist",
		DeepLinkSchema:    "https://vibrantapp.com/sdp-rc",
		SEP10ClientDomain: "vibrantapp.com",
		Assets: []data.Asset{
			assets.USDCAssetPubnet,
		},
	},
	{
		Name:        "User Managed Wallet",
		UserManaged: true,
		Assets: []data.Asset{
			assets.EURCAssetPubnet,
			assets.USDCAssetPubnet,
			assets.XLMAsset,
			assets.SWAGAssetPubnet,
		},
	},
	{
		Name:              "Meridian Pay",
		Homepage:          "https://meridianpay.stellar.org",
		DeepLinkSchema:    "https://meridianpay.stellar.org/invite",
		SEP10ClientDomain: "",
		UserManaged:       false,
		Embedded:          true,
		Enabled:           true,
		Assets: []data.Asset{
			assets.SWAGAssetPubnet,
			assets.XLMAsset,
		},
	},
	// {
	// 	Name:              "Beans App",
	// 	Homepage:          "https://www.beansapp.com/disbursements",
	// 	DeepLinkSchema:    "https://www.beansapp.com/disbursements/registration?redirect=true",
	// 	SEP10ClientDomain: "api.beansapp.com",
	// },
}
