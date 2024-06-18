package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// USDC

const USDCAssetCode = "USDC"

const USDCAssetIssuerPubnet = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

var USDCAssetPubnet = data.Asset{
	Code:   USDCAssetCode,
	Issuer: USDCAssetIssuerPubnet,
}

// EURC

const EURCAssetCode = "EURC"

const EURCAssetIssuerPubnet = "GDHU6WRG4IEQXM5NZ4BMPKOXHW76MZM4Y2IEMFDVXBSDP6SJY4ITNPP2"

var EURCAssetPubnet = data.Asset{
	Code:   EURCAssetCode,
	Issuer: EURCAssetIssuerPubnet,
}

// XLM

const XLMAssetCode = "XLM"

var XLMAsset = data.Asset{
	Code:   XLMAssetCode,
	Issuer: "",
}
