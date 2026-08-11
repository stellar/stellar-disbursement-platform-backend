package services

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// DistributionWalletKeyServiceInterface manages per-distribution-wallet secret material with
// envelope encryption. Every wallet's ciphertext is independent: storing, reading, or rotating
// one wallet's key never touches another wallet's row, and a corrupted or missing key fails
// closed for that wallet alone.
type DistributionWalletKeyServiceInterface interface {
	StoreKey(ctx context.Context, publicKey, privateKey string) error
	GetPrivateKey(ctx context.Context, publicKey string) (string, error)
	RotateWalletDEK(ctx context.Context, publicKey string) error
	DeleteKey(ctx context.Context, publicKey string) error
}

type DistributionWalletKeyService struct {
	keyModel  *store.DistributionWalletKeyModel
	encrypter utils.EnvelopeEncrypter
	// kek is the host-level key-encryption key (the distribution-account encryption
	// passphrase). It wraps each wallet's DEK; it never encrypts wallet secrets directly.
	kek string
}

var _ DistributionWalletKeyServiceInterface = (*DistributionWalletKeyService)(nil)

func NewDistributionWalletKeyService(dbConnectionPool db.DBConnectionPool, kek string) (*DistributionWalletKeyService, error) {
	if dbConnectionPool == nil {
		return nil, fmt.Errorf("dbConnectionPool cannot be nil")
	}
	if kek == "" {
		return nil, fmt.Errorf("kek cannot be empty")
	}

	return &DistributionWalletKeyService{
		keyModel:  store.NewDistributionWalletKeyModel(dbConnectionPool),
		encrypter: utils.EnvelopeEncrypter{},
		kek:       kek,
	}, nil
}

// StoreKey envelope-encrypts privateKey under a fresh wallet-specific DEK and persists it.
func (s *DistributionWalletKeyService) StoreKey(ctx context.Context, publicKey, privateKey string) error {
	ciphertext, encryptedDEK, err := s.encrypter.EncryptWithNewDEK(privateKey, s.kek)
	if err != nil {
		return fmt.Errorf("envelope-encrypting private key for %q: %w", publicKey, err)
	}

	err = s.keyModel.Insert(ctx, store.DistributionWalletKey{
		PublicKey:           publicKey,
		EncryptedPrivateKey: ciphertext,
		EncryptedDEK:        encryptedDEK,
	})
	if err != nil {
		return fmt.Errorf("storing distribution wallet key for %q: %w", publicKey, err)
	}

	return nil
}

// GetPrivateKey loads and decrypts one wallet's private key. Failures (missing row, corrupted
// DEK or ciphertext, wrong KEK) fail closed for this wallet only.
func (s *DistributionWalletKeyService) GetPrivateKey(ctx context.Context, publicKey string) (string, error) {
	key, err := s.keyModel.Get(ctx, publicKey)
	if err != nil {
		return "", fmt.Errorf("loading distribution wallet key for %q: %w", publicKey, err)
	}

	privateKey, err := s.encrypter.DecryptWithDEK(key.EncryptedPrivateKey, key.EncryptedDEK, s.kek)
	if err != nil {
		return "", fmt.Errorf("decrypting distribution wallet key for %q: %w", publicKey, err)
	}

	return privateKey, nil
}

// RotateWalletDEK re-encrypts ONE wallet's private key under a brand-new DEK. Sibling wallets'
// rows are never read or written.
func (s *DistributionWalletKeyService) RotateWalletDEK(ctx context.Context, publicKey string) error {
	key, err := s.keyModel.Get(ctx, publicKey)
	if err != nil {
		return fmt.Errorf("loading distribution wallet key for rotation %q: %w", publicKey, err)
	}

	newCiphertext, newEncryptedDEK, err := s.encrypter.RotateDEK(key.EncryptedPrivateKey, key.EncryptedDEK, s.kek)
	if err != nil {
		return fmt.Errorf("rotating DEK for %q: %w", publicKey, err)
	}

	err = s.keyModel.UpdateCiphertext(ctx, publicKey, newCiphertext, newEncryptedDEK)
	if err != nil {
		return fmt.Errorf("persisting rotated key for %q: %w", publicKey, err)
	}

	return nil
}

// DeleteKey removes one wallet's secret material (used only for never-used wallets; archived
// wallets that have signed transactions keep their key reference for audit per the spec).
func (s *DistributionWalletKeyService) DeleteKey(ctx context.Context, publicKey string) error {
	if err := s.keyModel.Delete(ctx, publicKey); err != nil {
		return fmt.Errorf("deleting distribution wallet key for %q: %w", publicKey, err)
	}
	return nil
}
