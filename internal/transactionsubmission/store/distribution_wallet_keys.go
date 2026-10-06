package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

// DistributionWalletKey is one wallet's envelope-encrypted secret material: the private key is
// encrypted with the wallet's own DEK, and the DEK is wrapped by the host KEK. Rows are fully
// independent of each other and of the legacy `vault` table.
type DistributionWalletKey struct {
	PublicKey           string    `db:"public_key"`
	EncryptedPrivateKey string    `db:"encrypted_private_key"`
	EncryptedDEK        string    `db:"encrypted_dek"`
	CreatedAt           time.Time `db:"created_at"`
	UpdatedAt           time.Time `db:"updated_at"`
}

func (k DistributionWalletKey) String() string {
	return fmt.Sprintf("%T{PublicKey: %s, CreatedAt: %v, UpdatedAt: %v}", k, k.PublicKey, k.CreatedAt, k.UpdatedAt)
}

type DistributionWalletKeyModel struct {
	DBConnectionPool db.DBConnectionPool
}

func NewDistributionWalletKeyModel(dbConnectionPool db.DBConnectionPool) *DistributionWalletKeyModel {
	return &DistributionWalletKeyModel{DBConnectionPool: dbConnectionPool}
}

// Insert stores one wallet's envelope-encrypted secret material.
func (m *DistributionWalletKeyModel) Insert(ctx context.Context, key DistributionWalletKey) error {
	if key.PublicKey == "" {
		return fmt.Errorf("public key cannot be empty")
	}
	if key.EncryptedPrivateKey == "" {
		return fmt.Errorf("encrypted private key cannot be empty")
	}
	if key.EncryptedDEK == "" {
		return fmt.Errorf("encrypted DEK cannot be empty")
	}

	const q = `
		INSERT INTO
			distribution_wallet_keys (public_key, encrypted_private_key, encrypted_dek)
		VALUES
			($1, $2, $3)
	`
	_, err := m.DBConnectionPool.ExecContext(ctx, q, key.PublicKey, key.EncryptedPrivateKey, key.EncryptedDEK)
	if err != nil {
		return fmt.Errorf("inserting distribution wallet key: %w", err)
	}

	return nil
}

// Get returns the DistributionWalletKey for the provided publicKey.
func (m *DistributionWalletKeyModel) Get(ctx context.Context, publicKey string) (*DistributionWalletKey, error) {
	const q = `
		SELECT
			*
		FROM
			distribution_wallet_keys
		WHERE
			public_key = $1
	`

	var key DistributionWalletKey
	err := m.DBConnectionPool.GetContext(ctx, &key, q, publicKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("getting distribution wallet key %q: %w", publicKey, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("getting distribution wallet key %q: %w", publicKey, err)
	}

	return &key, nil
}

// UpdateCiphertext replaces one wallet's (encrypted_private_key, encrypted_dek) pair, used by
// DEK rotation. Only the row for publicKey is touched.
func (m *DistributionWalletKeyModel) UpdateCiphertext(ctx context.Context, publicKey, encryptedPrivateKey, encryptedDEK string) error {
	if encryptedPrivateKey == "" || encryptedDEK == "" {
		return fmt.Errorf("encrypted private key and encrypted DEK cannot be empty")
	}

	const q = `
		UPDATE distribution_wallet_keys
		SET encrypted_private_key = $2, encrypted_dek = $3
		WHERE public_key = $1
	`
	res, err := m.DBConnectionPool.ExecContext(ctx, q, publicKey, encryptedPrivateKey, encryptedDEK)
	if err != nil {
		return fmt.Errorf("updating distribution wallet key %q: %w", publicKey, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for distribution wallet key %q: %w", publicKey, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("updating distribution wallet key %q: %w", publicKey, ErrRecordNotFound)
	}

	return nil
}

// Delete removes one wallet's secret material.
func (m *DistributionWalletKeyModel) Delete(ctx context.Context, publicKey string) error {
	const q = `DELETE FROM distribution_wallet_keys WHERE public_key = $1`
	res, err := m.DBConnectionPool.ExecContext(ctx, q, publicKey)
	if err != nil {
		return fmt.Errorf("deleting distribution wallet key %q: %w", publicKey, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for distribution wallet key %q: %w", publicKey, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("deleting distribution wallet key %q: %w", publicKey, ErrRecordNotFound)
	}

	return nil
}
