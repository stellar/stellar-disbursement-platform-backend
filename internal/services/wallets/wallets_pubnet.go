package wallets

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
)

var PubnetWallets = []data.Wallet{
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
		Name:              "Freedom Wallet",
		Homepage:          "https://freedom-public-uat.bpventures.us",
		DeepLinkSchema:    "https://freedom-public-uat.bpventures.us/disbursement/create",
		SEP10ClientDomain: "freedom-public-uat.bpventures.us",
		Assets: []data.Asset{
			assets.USDCAssetPubnet,
		},
	},
	// {
	// 	Name:              "Beans App",
	// 	Homepage:          "https://www.beansapp.com/disbursements",
	// 	DeepLinkSchema:    "https://www.beansapp.com/disbursements/registration?redirect=true",
	// 	SEP10ClientDomain: "api.beansapp.com",
	// },
}
