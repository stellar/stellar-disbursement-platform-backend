package utils

import (
	"fmt"
	"slices"

	"github.com/stellar/go/network"
)

type NetworkType string

const (
	PubnetNetworkType  NetworkType = "pubnet"
	TestnetNetworkType NetworkType = "testnet"
)

func AllNetworkTypes() []NetworkType {
	return []NetworkType{
		TestnetNetworkType,
		PubnetNetworkType,
	}
}

func (n NetworkType) Validate() error {
	if !slices.Contains(AllNetworkTypes(), n) {
		return fmt.Errorf("invalid network type %q", n)
	}
	return nil
}

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
