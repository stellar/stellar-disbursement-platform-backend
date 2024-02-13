package signing

import (
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DistributionAccountEnvOptions_String_doesntContainPrivateKey(t *testing.T) {
	opts := DistributionAccountEnvOptions{
		DistributionPrivateKey: "SOME_PRIVATE_KEY",
		NetworkPassphrase:      "SOME_PASSPHRASE",
	}
	assert.NotContains(t, opts.String(), "SOME_PRIVATE_KEY")
	assert.Contains(t, opts.String(), "SOME_PASSPHRASE")
	assert.Contains(t, opts.String(), "*signing.DistributionAccountEnvOptions")
}

func Test_DistributionAccountEnvOptions_Validate(t *testing.T) {
	testCases := []struct {
		name              string
		opts              DistributionAccountEnvOptions
		wantErrorContains string
	}{
		{
			name:              "returns an error if the network passphrase is empty",
			opts:              DistributionAccountEnvOptions{},
			wantErrorContains: "network passphrase cannot be empty",
		},
		{
			name: "returns an error if the distribution private key is empty",
			opts: DistributionAccountEnvOptions{
				NetworkPassphrase: network.TestNetworkPassphrase,
			},
			wantErrorContains: "distribution private key is not a valid Ed25519 secret",
		},
		{
			name: "returns an error if the distribution private key is invalid",
			opts: DistributionAccountEnvOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: "invalid",
			},
			wantErrorContains: "distribution private key is not a valid Ed25519 secret",
		},
		{
			name: "🎉 successfully validate options",
			opts: DistributionAccountEnvOptions{
				NetworkPassphrase:      network.TestNetworkPassphrase,
				DistributionPrivateKey: keypair.MustRandom().Seed(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()

			if tc.wantErrorContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErrorContains)
			}
		})
	}
}
