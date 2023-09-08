package anchorplatform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewStellarAssetInAIF(t *testing.T) {
	// USDC
	assetCode := "USDC"
	assetIssuer := "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"
	asset := NewStellarAssetInAIF(assetCode, assetIssuer)
	require.Equal(t, "stellar:USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", asset)

	// XLM
	assetCode = "XLM"
	assetIssuer = ""
	asset = NewStellarAssetInAIF(assetCode, assetIssuer)
	require.Equal(t, "stellar:XLM", asset)
}
