package signing

import (
	"context"
	"fmt"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type DistributionAccountDBSignatureClientOptions struct {
	NetworkPassphrase    string
	DBConnectionPool     db.DBConnectionPool
	EncryptionPassphrase string
	Encrypter            utils.PrivateKeyEncrypter
}

func (opts *DistributionAccountDBSignatureClientOptions) Validate() error {
	if opts.NetworkPassphrase == "" {
		return fmt.Errorf("network passphrase cannot be empty")
	}

	if opts.DBConnectionPool == nil {
		return fmt.Errorf("database connection pool cannot be nil")
	}

	if !strkey.IsValidEd25519SecretSeed(opts.EncryptionPassphrase) {
		return fmt.Errorf("encryption passphrase is not a valid Ed25519 secret")
	}

	return nil
}

type DistributionAccountDBSignatureClient struct {
	networkPassphrase     string
	stellarSignatoryModel store.StellarSignatoryStore
	encrypter             utils.PrivateKeyEncrypter
	encryptionPassphrase  string
}

// NewDistributionAccountDBSignatureClient returns a new DefaultSignatureService instance.
func NewDistributionAccountDBSignatureClient(opts DistributionAccountDBSignatureClientOptions) (*DistributionAccountDBSignatureClient, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	encrypter := opts.Encrypter
	if encrypter == nil {
		encrypter = &utils.DefaultPrivateKeyEncrypter{}
	}

	return &DistributionAccountDBSignatureClient{
		networkPassphrase:     opts.NetworkPassphrase,
		stellarSignatoryModel: store.NewStellarSignatoryModel(opts.DBConnectionPool),
		encrypter:             encrypter,
		encryptionPassphrase:  opts.EncryptionPassphrase,
	}, nil
}

var _ SignatureClient = &DistributionAccountDBSignatureClient{}

func (c *DistributionAccountDBSignatureClient) getKPsForAccounts(ctx context.Context, stellarAccounts ...string) ([]*keypair.Full, error) {
	if len(stellarAccounts) == 0 {
		return nil, fmt.Errorf("no accounts provided")
	}

	accountsAlreadyAccountedFor := map[string]struct{}{}
	kps := []*keypair.Full{}
	for i, account := range stellarAccounts {
		if _, ok := accountsAlreadyAccountedFor[account]; ok {
			continue
		}
		accountsAlreadyAccountedFor[account] = struct{}{}

		if account == "" {
			return nil, fmt.Errorf("account %d is empty", i)
		}

		// Can return ErrRecordNotFound
		signatory, err := c.stellarSignatoryModel.Get(ctx, account)
		if err != nil {
			return nil, fmt.Errorf("getting secret for distribution account %q: %w", account, err)
		}

		sigPrivateKey, err := c.encrypter.Decrypt(signatory.EncryptedPrivateKey, c.encryptionPassphrase)
		if err != nil {
			return nil, fmt.Errorf("cannot decrypt private key: %w", err)
		}

		kp, err := keypair.ParseFull(sigPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parsing secret for distribution account %q: %w", account, err)
		}
		kps = append(kps, kp)
	}

	return kps, nil
}

func (c *DistributionAccountDBSignatureClient) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error) {
	if stellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.Type())
	}

	kps, err := c.getKPsForAccounts(ctx, stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for accounts %v in %s: %w", stellarAccounts, c.Type(), err)
	}

	signedStellarTx, err = stellarTx.Sign(c.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.Type(), err)
	}

	return signedStellarTx, nil
}

func (c *DistributionAccountDBSignatureClient) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	if feeBumpStellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.Type())
	}

	kps, err := c.getKPsForAccounts(ctx, stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for accounts %v in %s: %w", stellarAccounts, c.Type(), err)
	}

	signedFeeBumpStellarTx, err = feeBumpStellarTx.Sign(c.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.Type(), err)
	}

	return signedFeeBumpStellarTx, nil
}

func (c *DistributionAccountDBSignatureClient) BatchInsert(ctx context.Context, number int) (publicKeys []string, err error) {
	if number < 1 {
		return nil, fmt.Errorf("the number of accounts to insert need to be greater than zero")
	}

	batchInsertPayload := []*store.StellarSignatory{}
	for range make([]interface{}, number) {
		kp, innerErr := keypair.Random()
		if innerErr != nil {
			return nil, fmt.Errorf("generating random keypair: %w", innerErr)
		}

		publicKey := kp.Address()
		privateKey := kp.Seed()
		encryptedPrivateKey, innerErr := c.encrypter.Encrypt(privateKey, c.encryptionPassphrase)
		if innerErr != nil {
			return nil, fmt.Errorf("encrypting distribution account private key: %w", innerErr)
		}

		batchInsertPayload = append(batchInsertPayload, &store.StellarSignatory{
			PublicKey:           publicKey,
			EncryptedPrivateKey: encryptedPrivateKey,
		})
		publicKeys = append(publicKeys, publicKey)
	}

	err = c.stellarSignatoryModel.BatchInsert(ctx, batchInsertPayload)
	if err != nil {
		return nil, fmt.Errorf("batch inserting distribution accounts: %w", err)
	}

	return publicKeys, nil
}

func (c *DistributionAccountDBSignatureClient) Delete(ctx context.Context, publicKey string) error {
	err := c.stellarSignatoryModel.Delete(ctx, publicKey)
	if err != nil {
		return fmt.Errorf("deleting stellar signatory %q from database: %w", publicKey, err)
	}

	return nil
}

func (c *DistributionAccountDBSignatureClient) Type() string {
	return string(DistributionAccountDBSignatureClientType)
}

func (c *DistributionAccountDBSignatureClient) NetworkPassphrase() string {
	return c.networkPassphrase
}
