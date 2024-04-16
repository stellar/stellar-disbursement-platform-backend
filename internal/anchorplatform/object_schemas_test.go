package anchorplatform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewStellarAssetInAIF(t *testing.T) {
	testCases := []struct {
		name        string
		assetCode   string
		assetIssuer string
		expected    string
	}{
		{
			name:        "issued 'USDC'",
			assetCode:   "USDC",
			assetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			expected:    "stellar:USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		},
		{
			name:        "issued 'XLM'",
			assetCode:   "XLM",
			assetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			expected:    "stellar:XLM:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		},
		{
			name:        "issued 'native'",
			assetCode:   "native",
			assetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			expected:    "stellar:native:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		},
		{
			name:        "native asset represented as 'native'",
			assetCode:   "native",
			assetIssuer: "",
			expected:    "stellar:native",
		},
		{
			name:        "native asset represented as 'XLM'",
			assetCode:   "XLM",
			assetIssuer: "",
			expected:    "stellar:native",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			aifAsset := NewStellarAssetInAIF(tc.assetCode, tc.assetIssuer)
			assert.Equal(t, tc.expected, aifAsset)
		})
	}
}
