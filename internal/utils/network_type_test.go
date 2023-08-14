package utils

import (
	"testing"

	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
)

func Test_GetNetworkTypeFromNetworkPassphrase(t *testing.T) {
	testCases := []struct {
		networkPassphrase   string
		expectedNetworkType NetworkType
		expectedError       string
	}{
		{
			networkPassphrase:   network.PublicNetworkPassphrase,
			expectedNetworkType: PubnetNetworkType,
			expectedError:       "",
		},
		{
			networkPassphrase:   network.TestNetworkPassphrase,
			expectedNetworkType: TestnetNetworkType,
			expectedError:       "",
		},
		{
			networkPassphrase:   "invalid",
			expectedNetworkType: "",
			expectedError:       "invalid network passphrase provided",
		},
	}

	for _, tc := range testCases {
		networkType, err := GetNetworkTypeFromNetworkPassphrase(tc.networkPassphrase)
		assert.Equal(t, tc.expectedNetworkType, networkType)
		if tc.expectedError != "" {
			assert.EqualError(t, err, tc.expectedError)
		} else {
			assert.Nil(t, err)
		}
	}
}
