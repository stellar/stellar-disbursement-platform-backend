package signing

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type DistributionAccountDBVaultSignatureClientOptions struct {
	NetworkPassphrase    string
	DBConnectionPool     db.DBConnectionPool
	EncryptionPassphrase string
	Encrypter            sdpUtils.PrivateKeyEncrypter
}

func (opts *DistributionAccountDBVaultSignatureClientOptions) Validate() error {
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

type DistributionAccountDBVaultSignatureClient struct {
	networkPassphrase    string
	dbVault              store.DBVault
	walletKeys           *store.DistributionWalletKeyModel
	envelopeEncrypter    sdpUtils.EnvelopeEncrypter
	encrypter            sdpUtils.PrivateKeyEncrypter
	encryptionPassphrase string
}

// NewDistributionAccountDBVaultSignatureClient returns a new instance of the DistributionAccountDB SignatureClient.
func NewDistributionAccountDBVaultSignatureClient(opts DistributionAccountDBVaultSignatureClientOptions) (*DistributionAccountDBVaultSignatureClient, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	encrypter := opts.Encrypter
	if encrypter == nil {
		encrypter = &sdpUtils.DefaultPrivateKeyEncrypter{}
	}

	return &DistributionAccountDBVaultSignatureClient{
		networkPassphrase:    opts.NetworkPassphrase,
		dbVault:              store.NewDBVaultModel(opts.DBConnectionPool),
		walletKeys:           store.NewDistributionWalletKeyModel(opts.DBConnectionPool),
		envelopeEncrypter:    sdpUtils.EnvelopeEncrypter{},
		encrypter:            encrypter,
		encryptionPassphrase: opts.EncryptionPassphrase,
	}, nil
}

var _ SignatureClient = &DistributionAccountDBVaultSignatureClient{}

func (c *DistributionAccountDBVaultSignatureClient) getKPsForPublicKeys(ctx context.Context, publicKeys ...string) ([]*keypair.Full, error) {
	if len(publicKeys) == 0 {
		return nil, fmt.Errorf("no publicKeys provided")
	}

	publicKeysAlreadyAccountedFor := map[string]struct{}{}
	kps := []*keypair.Full{}
	for i, publicKey := range publicKeys {
		if _, ok := publicKeysAlreadyAccountedFor[publicKey]; ok {
			continue
		}
		publicKeysAlreadyAccountedFor[publicKey] = struct{}{}

		if publicKey == "" {
			return nil, fmt.Errorf("publicKey %d is empty", i)
		}

		// Per-wallet envelope-encrypted keys take precedence (multi-distribution-wallet);
		// the legacy vault remains the fallback for pre-multi-wallet accounts. Real failures
		// on the wallet-keys path fail closed for that wallet — they never fall through to
		// the vault, so corruption cannot be masked.
		var sigPrivateKey string
		walletKey, wkErr := c.walletKeys.Get(ctx, publicKey)
		switch {
		case wkErr == nil:
			sigPrivateKey, wkErr = c.envelopeEncrypter.DecryptWithDEK(walletKey.EncryptedPrivateKey, walletKey.EncryptedDEK, c.encryptionPassphrase)
			if wkErr != nil {
				return nil, fmt.Errorf("decrypting distribution wallet key %q in %T: %w", publicKey, c, wkErr)
			}
		case errors.Is(wkErr, store.ErrRecordNotFound):
			// Can return ErrRecordNotFound
			dbVaultEntry, err := c.dbVault.Get(ctx, publicKey)
			if err != nil {
				return nil, fmt.Errorf("getting dbVaultEntry for distribution account %q in %T: %w", publicKey, c, err)
			}

			sigPrivateKey, err = c.encrypter.Decrypt(dbVaultEntry.EncryptedPrivateKey, c.encryptionPassphrase)
			if err != nil {
				return nil, fmt.Errorf("cannot decrypt private key: %w", err)
			}
		default:
			return nil, fmt.Errorf("getting distribution wallet key %q in %T: %w", publicKey, c, wkErr)
		}

		kp, err := keypair.ParseFull(sigPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parsing secret for account %q in %T: %w", publicKey, c, err)
		}
		kps = append(kps, kp)
	}

	return kps, nil
}

func (c *DistributionAccountDBVaultSignatureClient) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, publicKeys ...string) (signedStellarTx *txnbuild.Transaction, err error) {
	if stellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.name())
	}

	kps, err := c.getKPsForPublicKeys(ctx, publicKeys...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for publicKeys %v in %s: %w", publicKeys, c.name(), err)
	}

	signedStellarTx, err = stellarTx.Sign(c.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.name(), err)
	}

	return signedStellarTx, nil
}

func (c *DistributionAccountDBVaultSignatureClient) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, publicKeys ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	if feeBumpStellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.name())
	}

	kps, err := c.getKPsForPublicKeys(ctx, publicKeys...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for publicKeys %v in %s: %w", publicKeys, c.name(), err)
	}

	signedFeeBumpStellarTx, err = feeBumpStellarTx.Sign(c.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.name(), err)
	}

	return signedFeeBumpStellarTx, nil
}

func (c *DistributionAccountDBVaultSignatureClient) BatchInsert(ctx context.Context, number int) (publicKeys []string, err error) {
	if number < 1 {
		return nil, fmt.Errorf("the number of publicKeys to insert needs to be greater than zero")
	}

	batchInsertPayload := []*store.DBVaultEntry{}
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

		batchInsertPayload = append(batchInsertPayload, &store.DBVaultEntry{
			PublicKey:           publicKey,
			EncryptedPrivateKey: encryptedPrivateKey,
		})
		publicKeys = append(publicKeys, publicKey)
	}

	err = c.dbVault.BatchInsert(ctx, batchInsertPayload)
	if err != nil {
		return nil, fmt.Errorf("batch inserting dbVaultEntries: %w", err)
	}

	return publicKeys, nil
}

func (c *DistributionAccountDBVaultSignatureClient) Delete(ctx context.Context, publicKey string) error {
	err := c.dbVault.Delete(ctx, publicKey)
	if err != nil {
		return fmt.Errorf("deleting dbVaultEntry %q from database: %w", publicKey, err)
	}

	return nil
}

func (c *DistributionAccountDBVaultSignatureClient) name() string {
	return sdpUtils.GetTypeName(c)
}

func (c *DistributionAccountDBVaultSignatureClient) NetworkPassphrase() string {
	return c.networkPassphrase
}
