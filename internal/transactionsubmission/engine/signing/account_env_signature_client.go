package signing

import (
	"context"
	"fmt"
	"slices"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"

	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type AccountEnvOptions struct {
	DistributionPrivateKey string
	NetworkPassphrase      string
	AccountType            schema.AccountType
}

func (opts AccountEnvOptions) String() string {
	return fmt.Sprintf("%T{NetworkPassphrase: %s}", opts, opts.NetworkPassphrase)
}

func (opts *AccountEnvOptions) Validate() error {
	if opts.NetworkPassphrase == "" {
		return fmt.Errorf("network passphrase cannot be empty")
	}

	if !strkey.IsValidEd25519SecretSeed(opts.DistributionPrivateKey) {
		return fmt.Errorf("distribution private key is not a valid Ed25519 secret")
	}

	suuportedAccTypes := []schema.AccountType{schema.HostStellarEnv, schema.DistributionAccountStellarEnv}
	if !slices.Contains(suuportedAccTypes, opts.AccountType) {
		return fmt.Errorf("the provided account type %s does not match any of the supported account types: %v", opts.AccountType, suuportedAccTypes)
	}

	return nil
}

type AccountEnvSignatureClient struct {
	networkPassphrase   string
	distributionAccount string
	distributionKP      *keypair.Full
	accountType         schema.AccountType
}

func (c *AccountEnvSignatureClient) String() string {
	return fmt.Sprintf("AccountEnvSignatureClient{distributionAccount: %s, networkPassphrase: %s}", c.distributionAccount, c.networkPassphrase)
}

// NewAccountEnvSignatureClient returns a new AccountEnvSignatureClient instance
func NewAccountEnvSignatureClient(opts AccountEnvOptions) (*AccountEnvSignatureClient, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	distributionKP, err := keypair.ParseFull(opts.DistributionPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing distribution seed: %w", err)
	}

	return &AccountEnvSignatureClient{
		networkPassphrase:   opts.NetworkPassphrase,
		distributionAccount: distributionKP.Address(),
		distributionKP:      distributionKP,
		accountType:         opts.AccountType,
	}, nil
}

var _ SignatureClient = (*AccountEnvSignatureClient)(nil)

// validateStellarAccounts ensures that the distribution account is the only account signing the transaction
func (c *AccountEnvSignatureClient) validateStellarAccounts(stellarAccounts ...string) error {
	if len(stellarAccounts) == 0 {
		return fmt.Errorf("stellar accounts cannot be empty in %s", c.name())
	}

	// Ensure that the distribution account is the only account signing the transaction
	for _, stellarAccount := range stellarAccounts {
		if stellarAccount != c.distributionAccount {
			return fmt.Errorf("stellar account %s is not allowed to sign in %s", stellarAccount, c.name())
		}
	}

	return nil
}

func (c *AccountEnvSignatureClient) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error) {
	if stellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.name())
	}

	err = c.validateStellarAccounts(stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("validating stellar accounts: %w", err)
	}

	signedStellarTx, err = stellarTx.Sign(c.NetworkPassphrase(), c.distributionKP)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.name(), err)
	}

	return signedStellarTx, nil
}

func (c *AccountEnvSignatureClient) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	if feeBumpStellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.name())
	}

	err = c.validateStellarAccounts(stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("validating stellar accounts: %w", err)
	}

	signedFeeBumpStellarTx, err = feeBumpStellarTx.Sign(c.NetworkPassphrase(), c.distributionKP)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.name(), err)
	}

	return signedFeeBumpStellarTx, nil
}

func (c *AccountEnvSignatureClient) BatchInsert(ctx context.Context, number int) (publicKeys []string, err error) {
	if number <= 0 {
		return nil, fmt.Errorf("number must be greater than 0")
	}

	publicKeys = make([]string, number)
	for i := 0; i < number; i++ {
		publicKeys[i] = c.distributionAccount
	}
	err = fmt.Errorf("BatchInsert called for signature client type %s: %w", c.name(), ErrUnsupportedCommand)
	return publicKeys, err
}

func (c *AccountEnvSignatureClient) Delete(ctx context.Context, publicKey string) error {
	err := c.validateStellarAccounts(publicKey)
	if err != nil {
		return fmt.Errorf("validating stellar account to delete: %w", err)
	}
	return fmt.Errorf("Delete called for signature client type %s: %w", c.name(), ErrUnsupportedCommand)
}

func (c *AccountEnvSignatureClient) name() string {
	return fmt.Sprintf("%s.%s", sdpUtils.GetTypeName(c), c.accountType)
}

func (c *AccountEnvSignatureClient) NetworkPassphrase() string {
	return c.networkPassphrase
}
