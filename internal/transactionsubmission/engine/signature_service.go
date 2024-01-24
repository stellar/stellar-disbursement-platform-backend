package engine

import (
	"context"
	"fmt"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

//go:generate mockery --name=SignatureService --case=underscore --structname=MockSignatureService
type SignatureService interface {
	DistributionAccount() string
	NetworkPassphrase() string
	SignStellarTransaction(ctx context.Context, stellarTx *txnbuild.Transaction, stellarAccounts ...string) (signedStellarTx *txnbuild.Transaction, err error)
	SignFeeBumpStellarTransaction(ctx context.Context, feeBumpStellarTx *txnbuild.FeeBumpTransaction, stellarAccounts ...string) (signedFeeBumpStellarTx *txnbuild.FeeBumpTransaction, err error)
	BatchInsert(ctx context.Context, kps []*keypair.Full, shouldEncryptSeed bool, currLedgerNumber int) (err error)
	Delete(ctx context.Context, publicKey string, currLedgerNumber int) error
}

type DefaultSignatureServiceOptions struct {
	NetworkPassphrase      string
	DBConnectionPool       db.DBConnectionPool
	DistributionPrivateKey string
	EncryptionPassphrase   string
	Encrypter              utils.PrivateKeyEncrypter
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
}

func NewDefaultSignatureServiceNew(opts DefaultSignatureServiceOptions) (*DefaultSignatureService, error) {
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
	}, nil
}

// NewDefaultSignatureService returns a new DefaultSignatureService instance.
func NewDefaultSignatureService(networkPassphrase string, dbConnectionPool db.DBConnectionPool, distributionSeed string, chAccStore store.ChannelAccountStore, encrypter utils.PrivateKeyEncrypter, encrypterPass string) (*DefaultSignatureService, error) {
	if dbConnectionPool == nil {
		return nil, fmt.Errorf("db connection pool cannot be nil")
	}
	if chAccStore == nil {
		return nil, fmt.Errorf("channel account store cannot be nil")
	}

	if (networkPassphrase != network.TestNetworkPassphrase) && (networkPassphrase != network.PublicNetworkPassphrase) {
		return nil, fmt.Errorf("invalid network passphrase: %q", networkPassphrase)
	}

	distributionKP, err := keypair.ParseFull(distributionSeed)
	if err != nil {
		return nil, fmt.Errorf("parsing distribution seed: %w", err)
	}

	if encrypter == nil {
		return nil, fmt.Errorf("private key encrypter cannot be nil")
	}

	if encrypterPass == "" {
		return nil, fmt.Errorf("private key encrypter passphrase cannot be empty")
	}

	return &DefaultSignatureService{
		networkPassphrase:    networkPassphrase,
		distributionAccount:  distributionKP.Address(),
		distributionKP:       distributionKP,
		dbConnectionPool:     dbConnectionPool,
		chAccModel:           chAccStore,
		encrypter:            encrypter,
		encryptionPassphrase: encrypterPass,
	}, nil
}

func (ds *DefaultSignatureService) DistributionAccount() string {
	return ds.distributionAccount
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

func (ds *DefaultSignatureService) BatchInsert(ctx context.Context, kps []*keypair.Full, shouldEncryptSeed bool, currLedgerNumber int) (err error) {
	if len(kps) == 0 {
		return fmt.Errorf("no keypairs provided")
	}

	batchInsertPayload := []*store.ChannelAccount{}
	for _, kp := range kps {
		publicKey := kp.Address()
		privateKey := kp.Seed()
		if shouldEncryptSeed {
			privateKey, err = ds.encrypter.Encrypt(privateKey, ds.encryptionPassphrase)
			if err != nil {
				return fmt.Errorf("encrypting channel account private key: %w", err)
			}
		}

		batchInsertPayload = append(batchInsertPayload, &store.ChannelAccount{
			PublicKey:  publicKey,
			PrivateKey: privateKey,
		})
	}

	err = ds.chAccModel.BatchInsertAndLock(ctx, batchInsertPayload, currLedgerNumber, currLedgerNumber+IncrementForMaxLedgerBounds)
	if err != nil {
		return fmt.Errorf("batch inserting channel accounts: %w", err)
	}

	return nil
}

func (ds *DefaultSignatureService) Delete(ctx context.Context, publicKey string, lockedToLedgerNumber int) error {
	err := ds.chAccModel.DeleteIfLockedUntil(ctx, publicKey, lockedToLedgerNumber)
	if err != nil {
		return fmt.Errorf("deleting channel account %q from database: %w", publicKey, err)
	}

	return nil
}

var _ SignatureService = &DefaultSignatureService{}
