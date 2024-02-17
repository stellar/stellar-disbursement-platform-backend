package signing

import (
	"context"
	"fmt"
	"strings"

	"github.com/stellar/go/txnbuild"
	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
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

type SignatureClientType string

const (
	ChannelAccountDBSignatureClientType       SignatureClientType = "CHANNEL_ACCOUNT_DB"
	DistributionAccountEnvSignatureClientType SignatureClientType = "DISTRIBUTION_ACCOUNT_ENV"
	DistributionAccountDBSignatureClientType  SignatureClientType = "DISTRIBUTION_ACCOUNT_DB"
	HostAccountEnvSignatureClientType         SignatureClientType = "HOST_ACCOUNT_ENV"
)

func AllSignatureClientTypes() []SignatureClientType {
	return []SignatureClientType{ChannelAccountDBSignatureClientType, DistributionAccountEnvSignatureClientType, DistributionAccountDBSignatureClientType, HostAccountEnvSignatureClientType}
}

func ParseSignatureClientType(sigClientType string) (SignatureClientType, error) {
	sigClientTypeStrUpper := strings.ToUpper(sigClientType)
	scType := SignatureClientType(sigClientTypeStrUpper)

	if slices.Contains(AllSignatureClientTypes(), scType) {
		return scType, nil
	}

	return "", fmt.Errorf("invalid signature client type %q", sigClientTypeStrUpper)
}

func DistributionSignatureClientTypes() []SignatureClientType {
	return []SignatureClientType{DistributionAccountEnvSignatureClientType, DistributionAccountDBSignatureClientType}
}

func ParseSignatureClientDistributionType(sigClientType string) (SignatureClientType, error) {
	sigClientTypeStrUpper := strings.ToUpper(sigClientType)
	scType := SignatureClientType(sigClientTypeStrUpper)

	if slices.Contains(DistributionSignatureClientTypes(), scType) {
		return scType, nil
	}

	return "", fmt.Errorf("invalid signature client distribution type %q", sigClientTypeStrUpper)
}

type SignatureClientOptions struct {
	// Shared:
	NetworkPassphrase string

	// DistributionAccountEnv:
	DistributionPrivateKey string

	// ChannelAccountDB:
	DBConnectionPool     db.DBConnectionPool
	EncryptionPassphrase string
	LedgerNumberTracker  preconditions.LedgerNumberTracker
	Encrypter            utils.PrivateKeyEncrypter // (optional)
}

func NewSignatureClient(sigType SignatureClientType, opts SignatureClientOptions) (SignatureClient, error) {
	switch sigType {
	case DistributionAccountEnvSignatureClientType, HostAccountEnvSignatureClientType:
		return NewDistributionAccountEnvSignatureClient(DistributionAccountEnvOptions{
			NetworkPassphrase:      opts.NetworkPassphrase,
			DistributionPrivateKey: opts.DistributionPrivateKey,
		})

	case ChannelAccountDBSignatureClientType:
		return NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
			NetworkPassphrase:    opts.NetworkPassphrase,
			DBConnectionPool:     opts.DBConnectionPool,
			EncryptionPassphrase: opts.EncryptionPassphrase,
			LedgerNumberTracker:  opts.LedgerNumberTracker,
			Encrypter:            opts.Encrypter,
		})

	default:
		return nil, fmt.Errorf("invalid signature client type: %v", sigType)
	}
}
