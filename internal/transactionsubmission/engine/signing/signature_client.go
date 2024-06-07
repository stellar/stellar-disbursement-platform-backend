package signing

import (
	"context"
	"fmt"

	"github.com/stellar/go/txnbuild"

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
		return NewAccountEnvSignatureClient(AccountEnvOptions{
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
		return NewDistributionAccountDBVaultSignatureClient(DistributionAccountDBVaultSignatureClientOptions{
			NetworkPassphrase:    opts.NetworkPassphrase,
			DBConnectionPool:     opts.DBConnectionPool,
			EncryptionPassphrase: opts.DistAccEncryptionPassphrase,
			Encrypter:            opts.Encrypter,
		})

	default:
		return nil, fmt.Errorf("cannot find a Stellar signature client for accountType=%v", accType)
	}
}
