package signing

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type ChannelAccountDBSignatureClientOptions struct {
	NetworkPassphrase    string
	DBConnectionPool     db.DBConnectionPool
	EncryptionPassphrase string
	Encrypter            sdpUtils.PrivateKeyEncrypter
	LedgerNumberTracker  preconditions.LedgerNumberTracker
}

func (opts *ChannelAccountDBSignatureClientOptions) Validate() error {
	if opts.NetworkPassphrase == "" {
		return fmt.Errorf("network passphrase cannot be empty")
	}

	if opts.DBConnectionPool == nil {
		return fmt.Errorf("database connection pool cannot be nil")
	}

	if !strkey.IsValidEd25519SecretSeed(opts.EncryptionPassphrase) {
		return fmt.Errorf("encryption passphrase is not a valid Ed25519 secret")
	}

	if opts.LedgerNumberTracker == nil {
		return fmt.Errorf("ledger number tracker cannot be nil")
	}

	return nil
}

type ChannelAccountDBSignatureClient struct {
	networkPassphrase    string
	dbConnectionPool     db.DBConnectionPool
	chAccModel           store.ChannelAccountStore
	encrypter            sdpUtils.PrivateKeyEncrypter
	encryptionPassphrase string
	ledgerNumberTracker  preconditions.LedgerNumberTracker
}

// NewChannelAccountDBSignatureClient returns a new instance of the ChannelAccountDB SignatureClient.
func NewChannelAccountDBSignatureClient(opts ChannelAccountDBSignatureClientOptions) (*ChannelAccountDBSignatureClient, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	encrypter := opts.Encrypter
	if encrypter == nil {
		encrypter = &sdpUtils.DefaultPrivateKeyEncrypter{}
	}

	return &ChannelAccountDBSignatureClient{
		networkPassphrase:    opts.NetworkPassphrase,
		dbConnectionPool:     opts.DBConnectionPool,
		chAccModel:           store.NewChannelAccountModel(opts.DBConnectionPool),
		encrypter:            encrypter,
		encryptionPassphrase: opts.EncryptionPassphrase,
		ledgerNumberTracker:  opts.LedgerNumberTracker,
	}, nil
}

var _ SignatureClient = &ChannelAccountDBSignatureClient{}

func (c *ChannelAccountDBSignatureClient) getKPsForPublicKeys(ctx context.Context, stellarAccounts ...string) ([]*keypair.Full, error) {
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
		chAcc, err := c.chAccModel.Get(ctx, c.dbConnectionPool, account, 0)
		if err != nil {
			return nil, fmt.Errorf("getting secret for channel account %q: %w", account, err)
		}

		chAccPrivateKey, err := c.encrypter.Decrypt(chAcc.PrivateKey, c.encryptionPassphrase)
		if err != nil {
			return nil, fmt.Errorf("cannot decrypt private key: %w", err)
		}

		kp, err := keypair.ParseFull(chAccPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parsing secret for channel account %q: %w", account, err)
		}
		kps = append(kps, kp)
	}

	return kps, nil
}

func (c *ChannelAccountDBSignatureClient) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error) {
	if stellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.name())
	}

	kps, err := c.getKPsForPublicKeys(ctx, stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for accounts %v in %s: %w", stellarAccounts, c.name(), err)
	}

	signedStellarTx, err = stellarTx.Sign(c.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.name(), err)
	}

	return signedStellarTx, nil
}

func (c *ChannelAccountDBSignatureClient) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	if feeBumpStellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", c.name())
	}

	kps, err := c.getKPsForPublicKeys(ctx, stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for accounts %v in %s: %w", stellarAccounts, c.name(), err)
	}

	signedFeeBumpStellarTx, err = feeBumpStellarTx.Sign(c.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction in %s: %w", c.name(), err)
	}

	return signedFeeBumpStellarTx, nil
}

func (c *ChannelAccountDBSignatureClient) BatchInsert(ctx context.Context, number int) (publicKeys []string, err error) {
	if number < 1 {
		return nil, fmt.Errorf("the number of accounts to insert need to be greater than zero")
	}

	currentLedgerNumber, err := c.ledgerNumberTracker.GetLedgerNumber()
	if err != nil {
		return nil, fmt.Errorf("getting current ledger number: %w", err)
	}
	lockedToLedgerNumber := currentLedgerNumber + preconditions.IncrementForMaxLedgerBounds

	batchInsertPayload := []*store.ChannelAccount{}
	for range make([]interface{}, number) {
		kp, innerErr := keypair.Random()
		if innerErr != nil {
			return nil, fmt.Errorf("generating random keypair: %w", innerErr)
		}

		publicKey := kp.Address()
		privateKey := kp.Seed()
		encryptedPrivateKey, innerErr := c.encrypter.Encrypt(privateKey, c.encryptionPassphrase)
		if innerErr != nil {
			return nil, fmt.Errorf("encrypting channel account private key: %w", innerErr)
		}

		batchInsertPayload = append(batchInsertPayload, &store.ChannelAccount{
			PublicKey:  publicKey,
			PrivateKey: encryptedPrivateKey,
		})
		publicKeys = append(publicKeys, publicKey)
	}

	err = c.chAccModel.BatchInsertAndLock(ctx, batchInsertPayload, currentLedgerNumber, lockedToLedgerNumber)
	if err != nil {
		return nil, fmt.Errorf("batch inserting channel accounts: %w", err)
	}

	return publicKeys, nil
}

func (c *ChannelAccountDBSignatureClient) Delete(ctx context.Context, publicKey string) error {
	currentLedgerNumber, err := c.ledgerNumberTracker.GetLedgerNumber()
	if err != nil {
		return fmt.Errorf("getting current ledger number: %w", err)
	}
	lockedToLedgerNumber := currentLedgerNumber + preconditions.IncrementForMaxLedgerBounds

	err = c.chAccModel.DeleteIfLockedUntil(ctx, publicKey, lockedToLedgerNumber)
	if err != nil {
		return fmt.Errorf("deleting channel account %q from database: %w", publicKey, err)
	}

	return nil
}

func (c *ChannelAccountDBSignatureClient) name() string {
	return sdpUtils.GetTypeName(c)
}

func (c *ChannelAccountDBSignatureClient) NetworkPassphrase() string {
	return c.networkPassphrase
}
