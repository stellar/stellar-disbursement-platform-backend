package signing

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/exp/maps"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type DistributionSignatureClientType string

func (s DistributionSignatureClientType) AccountType() (schema.AccountType, error) {
	switch strings.TrimSpace(strings.ToUpper(string(s))) {
	case string(DistributionAccountEnvSignatureClientType):
		return schema.DistributionAccountStellarEnv, nil
	case string(DistributionAccountDBSignatureClientType):
		return schema.DistributionAccountStellarDBVault, nil
	default:
		return "", fmt.Errorf("invalid distribution account type %q", s)
	}
}

const (
	DistributionAccountEnvSignatureClientType DistributionSignatureClientType = "DISTRIBUTION_ACCOUNT_ENV"
	DistributionAccountDBSignatureClientType  DistributionSignatureClientType = "DISTRIBUTION_ACCOUNT_DB"
)

func DistributionSignatureClientTypes() []DistributionSignatureClientType {
	return maps.Keys(DistSigClientsDescription)
}

var DistSigClientsDescription = map[DistributionSignatureClientType]string{
	DistributionAccountEnvSignatureClientType: "uses the the same distribution account for all tenants, as well as for the HOST, through the secret configured in DISTRIBUTION_SEED.",
	DistributionAccountDBSignatureClientType:  "uses the one different distribution account private key per tenant, and stores them in the database, encrypted with the DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE.",
}

func DistSigClientsDescriptionStr() string {
	var descriptions []string
	for sigClientType, description := range DistSigClientsDescription {
		descriptions = append(descriptions, fmt.Sprintf("%s: %s", sigClientType, description))
	}

	return strings.Join(descriptions, " ")
}

func ParseDistributionSignatureClientType(sigClientType string) (DistributionSignatureClientType, error) {
	sigClientTypeStrUpper := strings.ToUpper(sigClientType)
	scType := DistributionSignatureClientType(sigClientTypeStrUpper)

	if slices.Contains(DistributionSignatureClientTypes(), scType) {
		return scType, nil
	}

	return "", fmt.Errorf("invalid distribution signature client type %q", sigClientTypeStrUpper)
}
