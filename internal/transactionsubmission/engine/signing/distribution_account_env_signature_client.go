package signing

import (
	"context"
	"fmt"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
)

type DistributionAccountEnvOptions struct {
	DistributionPrivateKey string
	NetworkPassphrase      string
}

func (opts *DistributionAccountEnvOptions) String() string {
	return fmt.Sprintf("DistributionAccountEnvOptions{NetworkPassphrase: %s}", opts.NetworkPassphrase)
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

func (c *DistributionAccountEnvSignatureClient) Type() string {
	return string(SignatureClientTypeDistributionAccountEnv)
}

func (c *DistributionAccountEnvSignatureClient) NetworkPassphrase() string {
	return c.networkPassphrase
}

func (c *DistributionAccountEnvSignatureClient) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error) {
	if stellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.Type())
	}

	signedStellarTx, err = stellarTx.Sign(c.NetworkPassphrase(), c.distributionKP)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.Type(), err)
	}

	return signedStellarTx, nil
}

func (c *DistributionAccountEnvSignatureClient) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	if feeBumpStellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.Type())
	}

	signedFeeBumpStellarTx, err = feeBumpStellarTx.Sign(c.NetworkPassphrase(), c.distributionKP)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.Type(), err)
	}

	return signedFeeBumpStellarTx, nil
}

func (c *DistributionAccountEnvSignatureClient) BatchInsert(ctx context.Context, amount int) (publicKeys []string, err error) {
	return nil, fmt.Errorf("signature client of type %s does not support BatchInsert", c.Type())
}

func (c *DistributionAccountEnvSignatureClient) Delete(ctx context.Context, publicKey string) error {
	return fmt.Errorf("signature client of type %s does not support Delete", c.Type())
}

var _ SignatureClient = &DistributionAccountEnvSignatureClient{}
