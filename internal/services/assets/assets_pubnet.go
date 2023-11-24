package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// USDCAssetCode is the code for the USDC asset for pubnet and testnet
const USDCAssetCode = "USDC"

// XLMAssetCode is the code for the XLM asset for pubnet and testnet
const XLMAssetCode = "XLM"

// USDCAssetIssuerPubnet is the issuer for the USDC asset for pubnet
const USDCAssetIssuerPubnet = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

// USDCAssetPubnet is the USDC asset for pubnet
var USDCAssetPubnet = data.Asset{
	Code:   USDCAssetCode,
	Issuer: USDCAssetIssuerPubnet,
}

// XLMAsset is the XLM asset for pubnet
var XLMAsset = data.Asset{
	Code:   XLMAssetCode,
	Issuer: "",
}
