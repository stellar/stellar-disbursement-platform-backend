package signing

import (
	"context"
	"fmt"
	"strings"

	"github.com/stellar/go/txnbuild"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var ErrUnsupportedCommand = fmt.Errorf("unsupported command for signature client")

//go:generate mockery --name=SignatureClient --case=underscore --structname=MockSignatureClient
type SignatureClient interface {
	NetworkPassphrase() string
	SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error)
	SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error)
	BatchInsert(ctx context.Context, number int) (publicKeys []string, err error)
	Delete(ctx context.Context, publicKey string) error
	Type() string
}

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

type SignatureClientOptions struct {
	// Shared:
	NetworkPassphrase string

	// DistributionAccountEnv:
	DistributionPrivateKey string

	// DistributionAccountDB:
	DistAccEncryptionPassphrase string

	// ChannelAccountDB:
	ChAccEncryptionPassphrase string

	// *AccountDB:
	DBConnectionPool    db.DBConnectionPool
	LedgerNumberTracker preconditions.LedgerNumberTracker
	Encrypter           utils.PrivateKeyEncrypter // (optional)
}

func NewSignatureClient(accType schema.AccountType, opts SignatureClientOptions) (SignatureClient, error) {
	switch accType {
	case schema.HostStellarEnv, schema.DistributionAccountStellarEnv:
		return NewDistributionAccountEnvSignatureClient(DistributionAccountEnvOptions{
			NetworkPassphrase:      opts.NetworkPassphrase,
			DistributionPrivateKey: opts.DistributionPrivateKey,
		})

	case schema.ChannelAccountStellarDB:
		return NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
			NetworkPassphrase:    opts.NetworkPassphrase,
			DBConnectionPool:     opts.DBConnectionPool,
			EncryptionPassphrase: opts.ChAccEncryptionPassphrase,
			LedgerNumberTracker:  opts.LedgerNumberTracker,
			Encrypter:            opts.Encrypter,
		})

	case schema.DistributionAccountStellarDBVault:
		return NewDistributionAccountDBSignatureClient(DistributionAccountDBSignatureClientOptions{
			NetworkPassphrase:    opts.NetworkPassphrase,
			DBConnectionPool:     opts.DBConnectionPool,
			EncryptionPassphrase: opts.DistAccEncryptionPassphrase,
			Encrypter:            opts.Encrypter,
		})

	default:
		return nil, fmt.Errorf("cannot find a Stellar signature client for accountType=%v", accType)
	}
}
