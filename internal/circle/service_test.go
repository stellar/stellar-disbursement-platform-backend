package circle

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_ServiceOptions_Validate(t *testing.T) {
	var clientFactory ClientFactory = func(networkType utils.NetworkType, apiKey string) ClientInterface {
		return nil
	}
	circleClientConfigModel := &ClientConfigModel{}

	testCases := []struct {
		name                string
		opts                ServiceOptions
		expectedErrContains string
	}{
		{
			name:                "ClientFactory validation fails",
			opts:                ServiceOptions{},
			expectedErrContains: "ClientFactory is required",
		},
		{
			name:                "ClientConfigModel validation fails",
			opts:                ServiceOptions{ClientFactory: clientFactory},
			expectedErrContains: "ClientConfigModel is required",
		},
		{
			name: "NetworkType validation fails",
			opts: ServiceOptions{
				ClientFactory:     clientFactory,
				ClientConfigModel: circleClientConfigModel,
				NetworkType:       utils.NetworkType("FOOBAR"),
			},
			expectedErrContains: `validating NetworkType: invalid network type "FOOBAR"`,
		},
		{
			name: "EncryptionPassphrase validation fails",
			opts: ServiceOptions{
				ClientFactory:        clientFactory,
				ClientConfigModel:    circleClientConfigModel,
				NetworkType:          utils.TestnetNetworkType,
				EncryptionPassphrase: "FOO BAR",
			},
			expectedErrContains: "EncryptionPassphrase is invalid",
		},
		{
			name: "🎉 successfully validates options",
			opts: ServiceOptions{
				ClientFactory:        clientFactory,
				ClientConfigModel:    circleClientConfigModel,
				NetworkType:          utils.TestnetNetworkType,
				EncryptionPassphrase: "SCW5I426WV3IDTLSTLQEHC6BMXWI2Z6C4DXAOC4ZA2EIHTAZQ6VD3JI6",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.expectedErrContains != "" {
				assert.Contains(t, err.Error(), tc.expectedErrContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
