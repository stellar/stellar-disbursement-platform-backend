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

type DBVaultEntry struct {
	PublicKey           string    `db:"public_key"`
	EncryptedPrivateKey string    `db:"encrypted_private_key"`
	UpdatedAt           time.Time `db:"updated_at"`
	CreatedAt           time.Time `db:"created_at"`
}

func (e DBVaultEntry) String() string {
	return fmt.Sprintf("%T{PublicKey: %s, CreatedAt: %v, UpdatedAt: %v}", e, e.PublicKey, e.CreatedAt, e.UpdatedAt)
}

type DBVaultModel struct {
	DBConnectionPool db.DBConnectionPool
}

func NewDBVaultModel(dbConnectionPool db.DBConnectionPool) *DBVaultModel {
	return &DBVaultModel{DBConnectionPool: dbConnectionPool}
}

// BatchInsert inserts a batch of (publicKey, encryptedPrivateKey) pairs into the database vault.
func (m *DBVaultModel) BatchInsert(ctx context.Context, dbVaultEntries []*DBVaultEntry) error {
	if len(dbVaultEntries) == 0 {
		return nil
	}

	publicKeys := make([]string, len(dbVaultEntries))
	encryptedPrivateKeys := make([]string, len(dbVaultEntries))

	for i, sAccKeys := range dbVaultEntries {
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
			vault (public_key, encrypted_private_key)
		SELECT * 
			FROM UNNEST($1::text[], $2::text[])
	`

	_, err := m.DBConnectionPool.ExecContext(ctx, q, pq.Array(publicKeys), pq.Array(encryptedPrivateKeys))
	if err != nil {
		return fmt.Errorf("inserting dbVaultEntry: %w", err)
	}

	return nil
}

// Get returns a DBVaultEntry with the provided publicKey from the database vault.
func (m *DBVaultModel) Get(ctx context.Context, publicKey string) (*DBVaultEntry, error) {
	query := `
		SELECT
			*
		FROM
			vault 
		WHERE
			public_key = $1
		`

	var dbVaultEntry DBVaultEntry
	err := m.DBConnectionPool.GetContext(ctx, &dbVaultEntry, query, publicKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("could not find dbVaultEntry %q: %w", publicKey, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("querying for dbVaultEntry %q: %w", publicKey, err)
	}

	return &dbVaultEntry, nil
}

// Delete deletes an entry with the provided publicKey from the database vault.
func (m *DBVaultModel) Delete(ctx context.Context, publicKey string) error {
	query := `
		DELETE
		FROM
			vault
		WHERE
			public_key = $1
		`

	res, err := m.DBConnectionPool.ExecContext(ctx, query, publicKey)
	if err != nil {
		return fmt.Errorf("deleting dbVaultEntry %q: %w", publicKey, err)
	}

	numRowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting number of rows affected: %w", err)
	}
	if numRowsAffected == 0 {
		return fmt.Errorf("could not find nor delete dbVaultEntry %q: %w", publicKey, ErrRecordNotFound)
	}

	return nil
}

var _ DBVault = &DBVaultModel{}
