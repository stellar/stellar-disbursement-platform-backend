package utils

import (
	"fmt"
	"slices"

	"github.com/stellar/go-stellar-sdk/network"
)

type NetworkType string

const (
	FuturenetNetworkType NetworkType = "futurenet"
	TestnetNetworkType   NetworkType = "testnet"
	PubnetNetworkType    NetworkType = "pubnet"
)

func AllNetworkTypes() []NetworkType {
	return []NetworkType{
		FuturenetNetworkType,
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

func (n NetworkType) IsPubnet() bool {
	return n == PubnetNetworkType
}

func (n NetworkType) IsTestnet() bool {
	return n == TestnetNetworkType
}

func GetNetworkTypeFromNetworkPassphrase(networkPassphrase string) (NetworkType, error) {
	switch networkPassphrase {
	case network.PublicNetworkPassphrase:
		return PubnetNetworkType, nil
	case network.TestNetworkPassphrase:
		return TestnetNetworkType, nil
	case network.FutureNetworkPassphrase:
		return FuturenetNetworkType, nil
	default:
		return "", fmt.Errorf("invalid network passphrase provided %q", networkPassphrase)
	}
}

// IsTestNetwork checks if the given network passphrase is a test network
func IsTestNetwork(networkPassphrase string) bool {
	testNetworks := []string{network.TestNetworkPassphrase, network.FutureNetworkPassphrase}
	return slices.Contains(testNetworks, networkPassphrase)
}
