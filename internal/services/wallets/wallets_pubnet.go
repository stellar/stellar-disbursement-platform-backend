package wallets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

var PubnetWallets = []data.Wallet{
	{
		Name:              "Vibrant Assist",
		Homepage:          "https://vibrantapp.com/vibrant-assist",
		DeepLinkSchema:    "https://vibrantapp.com/sdp",
		SEP10ClientDomain: "api.vibrantapp.com",
		Assets: []data.Asset{
			{
				Code:   "USDC",
				Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
			},
		},
	},
	{
		Name:              "Vibrant Assist RC",
		Homepage:          "vibrantapp.com/vibrant-assist",
		DeepLinkSchema:    "https://vibrantapp.com/sdp-rc",
		SEP10ClientDomain: "vibrantapp.com",
		Assets: []data.Asset{
			{
				Code:   "USDC",
				Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
			},
		},
	},
	// {
	// 	Name:              "Beans App",
	// 	Homepage:          "https://www.beansapp.com/disbursements",
	// 	DeepLinkSchema:    "https://www.beansapp.com/disbursements/registration?redirect=true",
	// 	SEP10ClientDomain: "api.beansapp.com",
	// },
}
