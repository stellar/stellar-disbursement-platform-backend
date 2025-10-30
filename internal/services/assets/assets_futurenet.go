package assets

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// AllAssetsFuturenet represents the assets available on the Futurenet network.
// At the moment Futurenet only supports the native asset, so we intentionally
// keep this list scoped to XLM.
var AllAssetsFuturenet = []data.Asset{
	XLMAsset,
}
