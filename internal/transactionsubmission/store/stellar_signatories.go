package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type StellarSignatory struct {
	PublicKey           string     `db:"public_key"`
	EncryptedPrivateKey string     `db:"encrypted_private_key"`
	UpdatedAt           *time.Time `db:"updated_at"`
	CreatedAt           *time.Time `db:"created_at"`
}

type StellarSignatoryModel struct {
	DBConnectionPool db.DBConnectionPool
}

func NewStellarSignatoryModel(dbConnectionPool db.DBConnectionPool) *StellarSignatoryModel {
	return &StellarSignatoryModel{DBConnectionPool: dbConnectionPool}
}

// BatchInsert inserts a batch of (publicKey, privateKey) pairs into the database.
func (m *StellarSignatoryModel) BatchInsert(ctx context.Context, stellarSignatories []*StellarSignatory) error {
	if len(stellarSignatories) == 0 {
		return nil
	}

	publicKeys := make([]string, len(stellarSignatories))
	encryptedPrivateKeys := make([]string, len(stellarSignatories))

	for i, sAccKeys := range stellarSignatories {
		if sAccKeys.PublicKey == "" {
			return fmt.Errorf("public key cannot be empty")
		}
		if sAccKeys.EncryptedPrivateKey == "" {
			return fmt.Errorf("private key cannot be empty")
		}

		publicKeys[i] = sAccKeys.PublicKey
		encryptedPrivateKeys[i] = sAccKeys.EncryptedPrivateKey
	}

	const q = `
		INSERT INTO 
			stellar_signatories (public_key, encrypted_private_key)
		SELECT * 
			FROM UNNEST($1::text[], $2::text[])
	`

	_, err := m.DBConnectionPool.ExecContext(ctx, q, pq.Array(publicKeys), pq.Array(encryptedPrivateKeys))
	if err != nil {
		return fmt.Errorf("inserting stellar signatory: %w", err)
	}

	return nil
}

func (m *StellarSignatoryModel) Get(ctx context.Context, publicKey string) (*StellarSignatory, error) {
	query := `
		SELECT
			*
		FROM
			stellar_signatories 
		WHERE
			public_key = $1
		`

	var stellarSignatory StellarSignatory
	err := m.DBConnectionPool.GetContext(ctx, &stellarSignatory, query, publicKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("could not find stellar signatory %q: %w", publicKey, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("querying for stellar signatory %q: %w", publicKey, err)
	}

	return &stellarSignatory, nil
}

// Delete deletes a row with the provided publicKey from the database.
func (m *StellarSignatoryModel) Delete(ctx context.Context, publicKey string) error {
	query := `
		DELETE
		FROM
			stellar_signatories
		WHERE
			public_key = $1
		`

	res, err := m.DBConnectionPool.ExecContext(ctx, query, publicKey)
	if err != nil {
		return fmt.Errorf("deleting stellar signatory %q: %w", publicKey, err)
	}

	numRowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return fmt.Errorf("could not find nor delete account %q: %w", publicKey, ErrRecordNotFound)
	}

	return nil
}

// var _ ChannelAccountStore = &ChannelAccountModel{}
