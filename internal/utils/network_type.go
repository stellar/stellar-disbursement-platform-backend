package utils

import (
	"fmt"
	"strings"

	"github.com/stellar/go/network"
)

type NetworkType string

const (
	PubnetNetworkType  NetworkType = "pubnet"
	TestnetNetworkType NetworkType = "testnet"
)

func GetNetworkTypeFromNetworkPassphrase(networkPassphrase string) (NetworkType, error) {
	switch networkPassphrase {
	case network.PublicNetworkPassphrase:
		return PubnetNetworkType, nil
	case network.TestNetworkPassphrase:
		return TestnetNetworkType, nil
	default:
		return "", fmt.Errorf("invalid network passphrase provided")
	}
}

func GetNetworkTypeFromString(networkType string) (NetworkType, error) {
	switch NetworkType(strings.ToLower(networkType)) {
	case PubnetNetworkType:
		return PubnetNetworkType, nil
	case TestnetNetworkType:
		return TestnetNetworkType, nil
	default:
		return "", fmt.Errorf("invalid network type provided")
	}
}
