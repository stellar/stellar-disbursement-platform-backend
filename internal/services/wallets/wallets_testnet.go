package wallets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

var TestnetWallets = []data.Wallet{
	{
		Name:              "Demo Wallet",
		Homepage:          "https://demo-wallet.stellar.org",
		DeepLinkSchema:    "https://demo-wallet.stellar.org",
		SEP10ClientDomain: "demo-wallet-server.stellar.org",
		Assets: []data.Asset{
			{
				Code:   "USDC",
				Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			},
			{
				Code:   "XLM",
				Issuer: "",
			},
		},
	},
}
