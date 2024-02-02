package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type SignatureServiceType string

const (
	SignatureServiceTypeDefault SignatureServiceType = "DEFAULT"
)

func (t SignatureServiceType) All() []SignatureServiceType {
	return []SignatureServiceType{SignatureServiceTypeDefault}
}

func ParseSignatureServiceType(sigServiceType string) (SignatureServiceType, error) {
	sigServiceTypeStrUpper := strings.ToUpper(sigServiceType)
	ctType := SignatureServiceType(sigServiceTypeStrUpper)

	if slices.Contains(ctType.All(), ctType) {
		return ctType, nil
	}

	return "", fmt.Errorf("invalid signature service type %q", sigServiceTypeStrUpper)
}

type SignatureServiceOptions struct {
	NetworkPassphrase      string
	DBConnectionPool       db.DBConnectionPool
	DistributionPrivateKey string
	EncryptionPassphrase   string
	LedgerNumberTracker    preconditions.LedgerNumberTracker
	Type                   SignatureServiceType
}

func NewSignatureService(opts SignatureServiceOptions) (SignatureService, error) {
	switch opts.Type {
	case SignatureServiceTypeDefault:
		return NewDefaultSignatureService(DefaultSignatureServiceOptions{
			NetworkPassphrase:      opts.NetworkPassphrase,
			DBConnectionPool:       opts.DBConnectionPool,
			DistributionPrivateKey: opts.DistributionPrivateKey,
			EncryptionPassphrase:   opts.EncryptionPassphrase,
			LedgerNumberTracker:    opts.LedgerNumberTracker,
		})

	default:
		return nil, fmt.Errorf("invalid signature service type: %v", opts.Type)
	}
}

//go:generate mockery --name=SignatureService --case=underscore --structname=MockSignatureService
type SignatureService interface {
	DistributionAccountResolver
	NetworkPassphrase() string
	SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error)
	SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error)
	BatchInsert(ctx context.Context, amount int) (publicKeys []string, err error)
	Delete(ctx context.Context, publicKey string) error
	Type() string
}

type DefaultSignatureServiceOptions struct {
	NetworkPassphrase      string
	DBConnectionPool       db.DBConnectionPool
	DistributionPrivateKey string
	EncryptionPassphrase   string
	Encrypter              utils.PrivateKeyEncrypter
	LedgerNumberTracker    preconditions.LedgerNumberTracker
}

func (opts *DefaultSignatureServiceOptions) Validate() error {
	if opts.NetworkPassphrase == "" {
		return fmt.Errorf("network passphrase cannot be empty")
	}

	if opts.DBConnectionPool == nil {
		return fmt.Errorf("database connection pool cannot be nil")
	}

	if !strkey.IsValidEd25519SecretSeed(opts.DistributionPrivateKey) {
		return fmt.Errorf("distribution private key is not a valid Ed25519 secret")
	}

	if !strkey.IsValidEd25519SecretSeed(opts.EncryptionPassphrase) {
		return fmt.Errorf("encryption passphrase is not a valid Ed25519 secret")
	}

	if opts.LedgerNumberTracker == nil {
		return fmt.Errorf("ledger number tracker cannot be nil")
	}

	return nil
}

type DefaultSignatureService struct {
	networkPassphrase    string
	distributionAccount  string
	distributionKP       *keypair.Full
	dbConnectionPool     db.DBConnectionPool
	chAccModel           store.ChannelAccountStore
	encrypter            utils.PrivateKeyEncrypter
	encryptionPassphrase string
	ledgerNumberTracker  preconditions.LedgerNumberTracker
}

// NewDefaultSignatureService returns a new DefaultSignatureService instance.
func NewDefaultSignatureService(opts DefaultSignatureServiceOptions) (*DefaultSignatureService, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating options: %w", err)
	}

	distributionKP, err := keypair.ParseFull(opts.DistributionPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing distribution seed: %w", err)
	}

	encrypter := opts.Encrypter
	if encrypter == nil {
		encrypter = &utils.DefaultPrivateKeyEncrypter{}
	}

	return &DefaultSignatureService{
		networkPassphrase:    opts.NetworkPassphrase,
		distributionAccount:  distributionKP.Address(),
		distributionKP:       distributionKP,
		dbConnectionPool:     opts.DBConnectionPool,
		chAccModel:           store.NewChannelAccountModel(opts.DBConnectionPool),
		encrypter:            encrypter,
		encryptionPassphrase: opts.EncryptionPassphrase,
		ledgerNumberTracker:  opts.LedgerNumberTracker,
	}, nil
}

var _ SignatureService = (*DefaultSignatureService)(nil)

func (ds *DefaultSignatureService) Type() string {
	return string(SignatureServiceTypeDefault)
}

func (ds *DefaultSignatureService) NetworkPassphrase() string {
	return ds.networkPassphrase
}

func (ds *DefaultSignatureService) getKPsForAccounts(ctx context.Context, stellarAccounts ...string) ([]*keypair.Full, error) {
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

		if account == ds.DistributionAccount() {
			kps = append(kps, ds.distributionKP)
			continue
		}

		// Can return ErrRecordNotFound
		chAcc, err := ds.chAccModel.Get(ctx, ds.dbConnectionPool, account, 0)
		if err != nil {
			return nil, fmt.Errorf("getting secret for channel account %q: %w", account, err)
		}

		chAccPrivateKey := chAcc.PrivateKey
		if !strkey.IsValidEd25519SecretSeed(chAccPrivateKey) {
			chAccPrivateKey, err = ds.encrypter.Decrypt(chAccPrivateKey, ds.encryptionPassphrase)
			if err != nil {
				return nil, fmt.Errorf("cannot decrypt private key: %w", err)
			}
		}

		kp, err := keypair.ParseFull(chAccPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parsing secret for channel account %q: %w", account, err)
		}
		kps = append(kps, kp)
	}

	return kps, nil
}

func (ds *DefaultSignatureService) SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error) {
	if stellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil")
	}

	kps, err := ds.getKPsForAccounts(ctx, stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for accounts %v: %w", stellarAccounts, err)
	}

	signedStellarTx, err = stellarTx.Sign(ds.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction: %w", err)
	}

	return signedStellarTx, nil
}

func (ds *DefaultSignatureService) SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error) {
	if feeBumpStellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil")
	}

	kps, err := ds.getKPsForAccounts(ctx, stellarAccounts...)
	if err != nil {
		return nil, fmt.Errorf("getting keypairs for accounts %v: %w", stellarAccounts, err)
	}

	signedFeeBumpStellarTx, err = feeBumpStellarTx.Sign(ds.NetworkPassphrase(), kps...)
	if err != nil {
		return nil, fmt.Errorf("signing transaction: %w", err)
	}

	return signedFeeBumpStellarTx, nil
}

func (ds *DefaultSignatureService) BatchInsert(ctx context.Context, amount int) (publicKeys []string, err error) {
	if amount < 1 {
		return nil, fmt.Errorf("the amnount of accounts to insert need to be greater than zero")
	}

	currentLedgerNumber, err := ds.ledgerNumberTracker.GetLedgerNumber()
	if err != nil {
		return nil, fmt.Errorf("getting current ledger number: %w", err)
	}
	lockedToLedgerNumber := currentLedgerNumber + preconditions.IncrementForMaxLedgerBounds

	batchInsertPayload := []*store.ChannelAccount{}
	for i := 0; i < amount; i++ {
		kp, innerErr := keypair.Random()
		if innerErr != nil {
			return nil, fmt.Errorf("generating random keypair: %w", innerErr)
		}

		publicKey := kp.Address()
		privateKey := kp.Seed()
		encryptedPrivateKey, innerErr := ds.encrypter.Encrypt(privateKey, ds.encryptionPassphrase)
		if innerErr != nil {
			return nil, fmt.Errorf("encrypting channel account private key: %w", innerErr)
		}

		batchInsertPayload = append(batchInsertPayload, &store.ChannelAccount{
			PublicKey:  publicKey,
			PrivateKey: encryptedPrivateKey,
		})
		publicKeys = append(publicKeys, publicKey)
	}

	err = ds.chAccModel.BatchInsertAndLock(ctx, batchInsertPayload, currentLedgerNumber, lockedToLedgerNumber)
	if err != nil {
		return nil, fmt.Errorf("batch inserting channel accounts: %w", err)
	}

	return publicKeys, nil
}

func (ds *DefaultSignatureService) Delete(ctx context.Context, publicKey string) error {
	currentLedgerNumber, err := ds.ledgerNumberTracker.GetLedgerNumber()
	if err != nil {
		return fmt.Errorf("getting current ledger number: %w", err)
	}
	lockedToLedgerNumber := currentLedgerNumber + preconditions.IncrementForMaxLedgerBounds

	err = ds.chAccModel.DeleteIfLockedUntil(ctx, publicKey, lockedToLedgerNumber)
	if err != nil {
		return fmt.Errorf("deleting channel account %q from database: %w", publicKey, err)
	}

	return nil
}

var _ DistributionAccountResolver = (*DefaultSignatureService)(nil)

func (ds *DefaultSignatureService) DistributionAccount() string {
	return ds.distributionAccount
}

func (ds *DefaultSignatureService) HostDistributionAccount() string {
	return ds.distributionAccount
}
