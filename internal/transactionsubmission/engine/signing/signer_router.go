package signing

import (
	"context"
	"fmt"

	"github.com/stellar/go/txnbuild"
	"golang.org/x/exp/maps"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

//go:generate mockery --name=SignerRouter --case=underscore --structname=MockSignerRouter
type SignerRouter interface {
	NetworkPassphrase() string
	SupportedAccountTypes() []schema.AccountType
	SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...schema.TransactionAccount) (signedStellarTx *txnbuild.Transaction, err error)
	SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...schema.TransactionAccount) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error)
	BatchInsert(ctx context.Context, accountType schema.AccountType, number int) (stellarAccounts []schema.TransactionAccount, err error)
	Delete(ctx context.Context, stellarAccount schema.TransactionAccount) error
}

type SignerRouterImpl map[schema.AccountType]SignatureClient

// var _ SignatureClient = (*SignerRouterInterface)(nil)

type SignatureRouterOptions struct {
	// Shared for ALL signers:
	NetworkPassphrase string

	// HOST.STELLAR.ENV:
	HostPrivateKey string

	// DISTRIBUTION_ACCOUNT.STELLAR.ENV:
	DistributionPrivateKey string

	// DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT:
	DistAccEncryptionPassphrase string

	// CHANNEL_ACCOUNT.STELLAR.DB:
	ChAccEncryptionPassphrase string
	LedgerNumberTracker       preconditions.LedgerNumberTracker

	// *.STELLAR.DB_VAULT:
	DBConnectionPool db.DBConnectionPool
	Encrypter        utils.PrivateKeyEncrypter // (optional)
}

func NewSignerRouter(opts SignatureRouterOptions, accountTypes ...schema.AccountType) (SignerRouter, error) {
	if len(accountTypes) == 0 {
		accountTypes = []schema.AccountType{
			schema.HostStellarEnv,
			schema.ChannelAccountStellarDB,
			schema.DistributionAccountStellarEnv,
			schema.DistributionAccountStellarDBVault,
		}
	}

	router := SignerRouterImpl{}
	for _, accType := range accountTypes {
		var newSigClient SignatureClient
		var err error

		switch accType {
		case schema.HostStellarEnv, schema.DistributionAccountStellarEnv:
			newSigClient, err = NewDistributionAccountEnvSignatureClient(DistributionAccountEnvOptions{
				NetworkPassphrase:      opts.NetworkPassphrase,
				DistributionPrivateKey: opts.HostPrivateKey,
				// AccountType:            accType, // TODO
			})

		case schema.ChannelAccountStellarDB:
			newSigClient, err = NewChannelAccountDBSignatureClient(ChannelAccountDBSignatureClientOptions{
				NetworkPassphrase:    opts.NetworkPassphrase,
				DBConnectionPool:     opts.DBConnectionPool,
				EncryptionPassphrase: opts.ChAccEncryptionPassphrase,
				LedgerNumberTracker:  opts.LedgerNumberTracker,
				Encrypter:            opts.Encrypter,
			})

		case schema.DistributionAccountStellarDBVault:
			newSigClient, err = NewDistributionAccountDBSignatureClient(DistributionAccountDBSignatureClientOptions{
				NetworkPassphrase:    opts.NetworkPassphrase,
				DBConnectionPool:     opts.DBConnectionPool,
				EncryptionPassphrase: opts.DistAccEncryptionPassphrase,
				Encrypter:            opts.Encrypter,
			})

		default:
			return nil, fmt.Errorf("cannot find a Stellar signature client for accountType=%v", accType)
		}

		if err != nil {
			return nil, fmt.Errorf("creating a new %q signature client: %w", accType, err)
		}

		router[accType] = newSigClient
	}

	return &router, nil
}

func (r *SignerRouterImpl) RouteSigner(distAcctountType schema.AccountType) (SignatureClient, error) {
	sigClient, ok := (*r)[distAcctountType]
	if !ok {
		return nil, fmt.Errorf("type %q is not supported by SignerRouter", distAcctountType)
	}

	return sigClient, nil
}

func (r *SignerRouterImpl) SignStellarTransaction(
	ctx context.Context,
	stellarTx *txnbuild.Transaction,
	accounts ...schema.TransactionAccount,
) (signedStellarTx *txnbuild.Transaction, err error) {
	// Get all signer types:
	sigTypes := map[schema.AccountType][]string{}
	for _, account := range accounts {
		sigTypes[account.Type] = append(sigTypes[account.Type], account.Address)
	}

	signedStellarTx = stellarTx
	for sigType, publicKeys := range sigTypes {
		sigClient, err := r.RouteSigner(sigType)
		if err != nil {
			return nil, fmt.Errorf("routing signer: %w", err)
		}

		signedStellarTx, err = sigClient.SignStellarTransaction(ctx, signedStellarTx, publicKeys...)
		if err != nil {
			return nil, fmt.Errorf("signing stellar transaction: %w", err)
		}
	}

	return signedStellarTx, nil
}

func (r *SignerRouterImpl) SignFeeBumpStellarTransaction(
	ctx context.Context,
	feeBumpStellarTx *txnbuild.FeeBumpTransaction,
	accounts ...schema.TransactionAccount,
) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	// Get all signer types:
	sigTypes := map[schema.AccountType][]string{}
	for _, account := range accounts {
		sigTypes[account.Type] = append(sigTypes[account.Type], account.Address)
	}

	signedFeeBumpStellarTx = feeBumpStellarTx
	for sigType, publicKeys := range sigTypes {
		sigClient, err := r.RouteSigner(sigType)
		if err != nil {
			return nil, fmt.Errorf("routing signer: %w", err)
		}

		signedFeeBumpStellarTx, err = sigClient.SignFeeBumpStellarTransaction(ctx, signedFeeBumpStellarTx, publicKeys...)
		if err != nil {
			return nil, fmt.Errorf("signing stellar transaction: %w", err)
		}
	}

	return signedFeeBumpStellarTx, nil
}

func (r *SignerRouterImpl) BatchInsert(
	ctx context.Context,
	accountType schema.AccountType,
	number int,
) (stellarAccounts []schema.TransactionAccount, err error) {
	sigClient, err := r.RouteSigner(accountType)
	if err != nil {
		return nil, fmt.Errorf("routing signer: %w", err)
	}

	publicKeys, err := sigClient.BatchInsert(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("batch inserting accounts for accountType=%s: %w", accountType, err)
	}

	for _, publicKey := range publicKeys {
		stellarAccounts = append(stellarAccounts, schema.TransactionAccount{
			Type:    accountType,
			Address: publicKey,
		})
	}

	return stellarAccounts, nil
}

func (r *SignerRouterImpl) Delete(
	ctx context.Context,
	account schema.TransactionAccount,
) error {
	sigClient, err := r.RouteSigner(account.Type)
	if err != nil {
		return fmt.Errorf("routing signer: %w", err)
	}

	err = sigClient.Delete(ctx, account.Address)
	if err != nil {
		return fmt.Errorf("deleting account %v: %w", account, err)
	}

	return nil
}

func (r *SignerRouterImpl) NetworkPassphrase() string {
	for _, sigClient := range *r {
		return sigClient.NetworkPassphrase()
	}

	// TODO: 🤔
	return "INVALID_NETWORK_PASSPHRASE"
}

func (r *SignerRouterImpl) SupportedAccountTypes() []schema.AccountType {
	return maps.Keys(*r)
}
