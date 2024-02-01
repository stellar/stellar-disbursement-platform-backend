package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// USDCAssetIssuerTestnet is the issuer for the USDC asset for testnet
const USDCAssetIssuerTestnet = "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"

// USDCAssetTestnet is the USDC asset for testnet
var USDCAssetTestnet = data.Asset{
	Code:   USDCAssetCode,
	Issuer: USDCAssetIssuerTestnet,
}
