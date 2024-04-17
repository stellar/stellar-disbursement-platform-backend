package utils

import (
	"fmt"

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
