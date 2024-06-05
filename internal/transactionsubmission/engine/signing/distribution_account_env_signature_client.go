package signing

import (
	"context"
	"fmt"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type DistributionAccountEnvOptions struct {
	DistributionPrivateKey string
	NetworkPassphrase      string
}

func (opts DistributionAccountEnvOptions) String() string {
	return fmt.Sprintf("%T{NetworkPassphrase: %s}", opts, opts.NetworkPassphrase)
}

func (opts *DistributionAccountEnvOptions) Validate() error {
	if opts.NetworkPassphrase == "" {
		return fmt.Errorf("network passphrase cannot be empty")
	}

	if !strkey.IsValidEd25519SecretSeed(opts.DistributionPrivateKey) {
		return fmt.Errorf("distribution private key is not a valid Ed25519 secret")
	}

	return nil
}

type DistributionAccountEnvSignatureClient struct {
	networkPassphrase   string
	distributionAccount string
	distributionKP      *keypair.Full
}

func (c *DistributionAccountEnvSignatureClient) String() string {
	return fmt.Sprintf("DistributionAccountEnvSignatureClient{distributionAccount: %s, networkPassphrase: %s}", c.distributionAccount, c.networkPassphrase)
}

// NewDistributionAccountEnvSignatureClient returns a new DistributionAccountEnvSignatureClient instance
func NewDistributionAccountEnvSignatureClient(opts DistributionAccountEnvOptions) (*DistributionAccountEnvSignatureClient, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	distributionKP, err := keypair.ParseFull(opts.DistributionPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing distribution seed: %w", err)
	}

	return &DistributionAccountEnvSignatureClient{
		networkPassphrase:   opts.NetworkPassphrase,
		distributionAccount: distributionKP.Address(),
		distributionKP:      distributionKP,
	}, nil
}

var _ SignatureClient = (*DistributionAccountEnvSignatureClient)(nil)

// validateStellarAccounts ensures that the distribution account is the only account signing the transaction
func (c *DistributionAccountEnvSignatureClient) validateStellarAccounts(stellarAccounts ...string) error {
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

func (c *DistributionAccountEnvSignatureClient) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error) {
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

func (c *DistributionAccountEnvSignatureClient) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
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

func (c *DistributionAccountEnvSignatureClient) BatchInsert(ctx context.Context, number int) (publicKeys []string, err error) {
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

func (c *DistributionAccountEnvSignatureClient) Delete(ctx context.Context, publicKey string) error {
	err := c.validateStellarAccounts(publicKey)
	if err != nil {
		return fmt.Errorf("validating stellar account to delete: %w", err)
	}
	return fmt.Errorf("Delete called for signature client type %s: %w", c.name(), ErrUnsupportedCommand)
}

func (c *DistributionAccountEnvSignatureClient) name() string {
	return sdpUtils.GetTypeName(c)
}

func (c *DistributionAccountEnvSignatureClient) NetworkPassphrase() string {
	return c.networkPassphrase
}
