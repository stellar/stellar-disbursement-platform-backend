package utils

import (
	"fmt"
	"testing"

	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
)

func Test_AllNetworkTypes(t *testing.T) {
	expectedNetworkTypes := []NetworkType{
		TestnetNetworkType,
		PubnetNetworkType,
	}

	assert.Equal(t, expectedNetworkTypes, AllNetworkTypes())
}

func Test_NetworkType_Validate(t *testing.T) {
	testCases := []struct {
		networkType NetworkType
		expectedErr error
	}{
		{
			networkType: TestnetNetworkType,
			expectedErr: nil,
		},
		{
			networkType: PubnetNetworkType,
			expectedErr: nil,
		},
		{
			networkType: "UNSUPPORTED",
			expectedErr: fmt.Errorf(`invalid network type "UNSUPPORTED"`),
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.networkType), func(t *testing.T) {
			err := tc.networkType.Validate()
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func Test_NetworkType_IsTestnet(t *testing.T) {
	testCases := []struct {
		networkType    NetworkType
		expectedResult bool
	}{
		{
			networkType:    TestnetNetworkType,
			expectedResult: true,
		},
		{
			networkType:    PubnetNetworkType,
			expectedResult: false,
		},
		{
			networkType:    "UNSUPPORTED",
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.networkType), func(t *testing.T) {
			assert.Equal(t, tc.expectedResult, tc.networkType.IsTestnet())
		})
	}
}

func Test_NetworkType_IsPubnet(t *testing.T) {
	testCases := []struct {
		networkType    NetworkType
		expectedResult bool
	}{
		{
			networkType:    TestnetNetworkType,
			expectedResult: false,
		},
		{
			networkType:    PubnetNetworkType,
			expectedResult: true,
		},
		{
			networkType:    "UNSUPPORTED",
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.networkType), func(t *testing.T) {
			assert.Equal(t, tc.expectedResult, tc.networkType.IsPubnet())
		})
	}
}

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
