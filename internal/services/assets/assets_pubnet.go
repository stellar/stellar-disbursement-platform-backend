package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

var AllAssetsPubnet = []data.Asset{
	EURCAssetPubnet,
	USDCAssetPubnet,
	XLMAsset,
}

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

// Meridian Pay: SWAG
const (
	SWAGAssetCode         = "SWAG"
	SWAGAssetIssuerPubnet = "GA7ZMISKTW3TMY3QRJJ57ABPPPZ765BQ5PMF66QQOQT2WBBWCG5G7MNK"
)

var SWAGAssetPubnet = data.Asset{
	Code:   SWAGAssetCode,
	Issuer: SWAGAssetIssuerPubnet,
}

// / Meridian Pay:
const (
	STICKERAssetCode = "STICKER"
	POSTERAssetCode = "POSTER"
	MerchAssetIssuerPubnet = "GAVAILXQC6PM7MVP2DUDHWSQBZRG7JRPVF754YHCHFS3SLY3FKYZU7DT"
)

var STICKERAssetPubnet = data.Asset{
	Code:   STICKERAssetCode,
	Issuer: MerchAssetIssuerPubnet,
}

var POSTERAssetPubnet = data.Asset{
	Code:   POSTERAssetCode,
	Issuer: MerchAssetIssuerPubnet,
}


