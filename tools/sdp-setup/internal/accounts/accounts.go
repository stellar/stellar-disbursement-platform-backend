package accounts

import (
	"fmt"
	"strings"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type Info struct {
	SEP10Public        string
	SEP10Private       string
	DistributionPublic string
	DistributionSeed   string
}

func Generate(network utils.NetworkType, distributionSeed string) Info {
	sep10Kp := keypair.MustRandom()
	var distKp *keypair.Full
	if strings.TrimSpace(distributionSeed) != "" {
		distKp = keypair.MustParseFull(distributionSeed)
	} else {
		distKp = keypair.MustRandom()
	}

	fmt.Printf("   ✓ SEP10 signing account: %s\n", sep10Kp.Address())
	fmt.Printf("   ✓ Distribution account: %s\n", distKp.Address())

	if network.IsTestnet() {
		fmt.Println("   💰 Funding distribution with XLM via Friendbot...")
		if err := fundTestnetAccount(distKp.Address()); err != nil {
			fmt.Printf("   ⚠️  Failed to fund account: %v\n", err)
		} else {
			fmt.Println("   ✓ Distribution funded with XLM")
		}
	} else {
		fmt.Printf("   ⚠️  Mainnet: make sure to fund account %s with XLM\n", distKp.Address())
	}

	return Info{
		SEP10Public:        sep10Kp.Address(),
		SEP10Private:       sep10Kp.Seed(),
		DistributionPublic: distKp.Address(),
		DistributionSeed:   distKp.Seed(),
	}
}

func fundTestnetAccount(address string) error {
	client := horizonclient.DefaultTestNetClient
	_, err := client.Fund(address)
	if err != nil {
		return fmt.Errorf("funding testnet account %s: %w", address, err)
	}
	return nil
}

// ValidateSecret validates a Stellar secret key
func ValidateSecret(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("required")
	}

	// Use stellar-go validation for Ed25519 secret seeds
	if !strkey.IsValidEd25519SecretSeed(s) {
		return fmt.Errorf("invalid secret key")
	}

	return nil
}
