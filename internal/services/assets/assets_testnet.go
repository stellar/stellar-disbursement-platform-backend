package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// USDC

const USDCAssetIssuerTestnet = "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"

var USDCAssetTestnet = data.Asset{
	Code:   USDCAssetCode,
	Issuer: USDCAssetIssuerTestnet,
}

// EURC

const EURCAssetIssuerTestnet = "GB3Q6QDZYTHWT7E5PVS3W7FUT5GVAFC5KSZFFLPU25GO7VTC3NM2ZTVO"

var EURCAssetTestnet = data.Asset{
	Code:   EURCAssetCode,
	Issuer: EURCAssetIssuerTestnet,
}
