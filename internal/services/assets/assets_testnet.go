package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// EURCAssetIssuerTestnet is the issuer for the EURC asset for testnet
const EURCAssetIssuerTestnet = "GB3Q6QDZYTHWT7E5PVS3W7FUT5GVAFC5KSZFFLPU25GO7VTC3NM2ZTVO"

// USDCAssetIssuerTestnet is the issuer for the USDC asset for testnet
const USDCAssetIssuerTestnet = "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"

// EURCAssetTestnet is the EURC asset for testnet
var EURCAssetTestnet = data.Asset{
	Code:   EURCAssetCode,
	Issuer: EURCAssetIssuerTestnet,
}

// USDCAssetTestnet is the USDC asset for testnet
var USDCAssetTestnet = data.Asset{
	Code:   USDCAssetCode,
	Issuer: USDCAssetIssuerTestnet,
}
