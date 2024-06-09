package signing

import (
	"context"
	"errors"
	"fmt"
	"sort"

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

var _ SignerRouter = (*SignerRouterImpl)(nil)

type SignerRouterImpl struct {
	strategies        map[schema.AccountType]SignatureClient
	networkPassphrase string
}

func NewSignerRouterImpl(network string, strategies map[schema.AccountType]SignatureClient) SignerRouterImpl {
	return SignerRouterImpl{
		networkPassphrase: network,
		strategies:        strategies,
	}
}

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

	router := map[schema.AccountType]SignatureClient{}
	for _, accType := range accountTypes {
		var newSigClient SignatureClient
		var err error

		switch accType {
		case schema.HostStellarEnv:
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

		case schema.DistributionAccountStellarEnv:
			newSigClient, err = NewDistributionAccountEnvSignatureClient(DistributionAccountEnvOptions{
				NetworkPassphrase:      opts.NetworkPassphrase,
				DistributionPrivateKey: opts.DistributionPrivateKey,
				// AccountType:            accType, // TODO
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

	return &SignerRouterImpl{
		strategies:        router,
		networkPassphrase: opts.NetworkPassphrase,
	}, nil
}

func (r *SignerRouterImpl) RouteSigner(distAcctountType schema.AccountType) (SignatureClient, error) {
	sigClient, ok := r.strategies[distAcctountType]
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
	if len(accounts) == 0 {
		return nil, errors.New("no accounts provided to sign the transaction")
	}

	// Get all signer types:
	sigTypes := map[schema.AccountType][]string{}
	for _, account := range accounts {
		sigTypes[account.Type] = append(sigTypes[account.Type], account.Address)
	}

	// Sort the types to ensure deterministic signing order:
	sortedTypes := []schema.AccountType{}
	for sigType := range sigTypes {
		sortedTypes = append(sortedTypes, sigType)
	}
	sort.Slice(sortedTypes, func(i, j int) bool {
		return sortedTypes[i] < sortedTypes[j]
	})

	signedStellarTx = stellarTx
	for _, sigType := range sortedTypes {
		publicKeys := sigTypes[sigType]
		sigClient, err := r.RouteSigner(sigType)
		if err != nil {
			return nil, fmt.Errorf("routing signer: %w", err)
		}

		signedStellarTx, err = sigClient.SignStellarTransaction(ctx, signedStellarTx, publicKeys...)
		if err != nil {
			return nil, fmt.Errorf("signing stellar transaction for strategy=%s: %w", sigType, err)
		}
	}

	return signedStellarTx, nil
}

func (r *SignerRouterImpl) SignFeeBumpStellarTransaction(
	ctx context.Context,
	feeBumpStellarTx *txnbuild.FeeBumpTransaction,
	accounts ...schema.TransactionAccount,
) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	if len(accounts) == 0 {
		return nil, errors.New("no accounts provided to sign the transaction")
	}

	// Get all signer types:
	sigTypes := map[schema.AccountType][]string{}
	for _, account := range accounts {
		sigTypes[account.Type] = append(sigTypes[account.Type], account.Address)
	}

	// Sort the types to ensure deterministic signing order:
	sortedTypes := []schema.AccountType{}
	for sigType := range sigTypes {
		sortedTypes = append(sortedTypes, sigType)
	}
	sort.Slice(sortedTypes, func(i, j int) bool {
		return sortedTypes[i] < sortedTypes[j]
	})

	signedFeeBumpStellarTx = feeBumpStellarTx
	for _, sigType := range sortedTypes {
		publicKeys := sigTypes[sigType]
		sigClient, err := r.RouteSigner(sigType)
		if err != nil {
			return nil, fmt.Errorf("routing signer: %w", err)
		}

		signedFeeBumpStellarTx, err = sigClient.SignFeeBumpStellarTransaction(ctx, signedFeeBumpStellarTx, publicKeys...)
		if err != nil {
			return nil, fmt.Errorf("signing stellar fee bump transaction for strategy=%s: %w", sigType, err)
		}
	}

	return signedFeeBumpStellarTx, nil
}

func (r *SignerRouterImpl) BatchInsert(
	ctx context.Context,
	accountType schema.AccountType,
	number int,
) (stellarAccounts []schema.TransactionAccount, err error) {
	if number < 1 {
		return nil, errors.New("number of accounts to insert must be greater than 0")
	}

	sigClient, err := r.RouteSigner(accountType)
	if err != nil {
		return nil, fmt.Errorf("routing signer: %w", err)
	}

	publicKeys, err := sigClient.BatchInsert(ctx, number)
	if err != nil && !(errors.Is(err, ErrUnsupportedCommand) && len(publicKeys) > 0) {
		return nil, fmt.Errorf("batch inserting accounts for strategy=%s: %w", accountType, err)
	}

	for _, publicKey := range publicKeys {
		stellarAccounts = append(stellarAccounts, schema.TransactionAccount{
			Type:    accountType,
			Address: publicKey,
			Status:  schema.AccountStatusActive,
		})
	}

	return stellarAccounts, err
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
		return fmt.Errorf("deleting account=%v for strategy=%s: %w", account, account.Type, err)
	}

	return nil
}

func (r *SignerRouterImpl) NetworkPassphrase() string {
	return r.networkPassphrase
}

func (r *SignerRouterImpl) SupportedAccountTypes() []schema.AccountType {
	return maps.Keys(r.strategies)
}
