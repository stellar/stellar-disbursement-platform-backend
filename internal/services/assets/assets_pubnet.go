package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// EURCAssetCode is the code for the EURC asset for pubnet and testnet
const EURCAssetCode = "EURC"

// USDCAssetCode is the code for the USDC asset for pubnet and testnet
const USDCAssetCode = "USDC"

// XLMAssetCode is the code for the XLM asset for pubnet and testnet
const XLMAssetCode = "XLM"

// EURCAssetIssuerPubnet is the issuer for the EURC asset for pubnet
const EURCAssetIssuerPubnet = "GDHU6WRG4IEQXM5NZ4BMPKOXHW76MZM4Y2IEMFDVXBSDP6SJY4ITNPP2"

// USDCAssetIssuerPubnet is the issuer for the USDC asset for pubnet
const USDCAssetIssuerPubnet = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

// EURCAssetPubnet is the EURC asset for pubnet
var EURCAssetPubnet = data.Asset{
	Code:   EURCAssetCode,
	Issuer: EURCAssetIssuerPubnet,
}

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
