package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type SEPNonceModel struct {
	dbConnectionPool db.DBConnectionPool
}

func NewSEPNonceModel(dbConnectionPool db.DBConnectionPool) *SEPNonceModel {
	return &SEPNonceModel{dbConnectionPool: dbConnectionPool}
}

func (m *SEPNonceModel) Store(ctx context.Context, nonce string, expiresAt time.Time) error {
	const q = `
        INSERT INTO sep_nonces (nonce, expires_at)
        VALUES ($1, $2)
    `

	_, err := m.dbConnectionPool.ExecContext(ctx, q, nonce, expiresAt)
	if err != nil {
		return fmt.Errorf("storing sep nonce: %w", err)
	}
	return nil
}

func (m *SEPNonceModel) Consume(ctx context.Context, nonce string) (time.Time, bool, error) {
	const q = `
    	DELETE FROM sep_nonces
        WHERE nonce = $1
        RETURNING expires_at
    `

	var expiresAt time.Time
	err := m.dbConnectionPool.GetContext(ctx, &expiresAt, q, nonce)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("consuming sep nonce: %w", err)
	}
	return expiresAt, true, nil
}

func (m *SEPNonceModel) DeleteExpired(ctx context.Context) error {
	const q = `
        DELETE FROM sep_nonces
        WHERE expires_at <= NOW()
    `

	_, err := m.dbConnectionPool.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("deleting expired sep nonces: %w", err)
	}
	return nil
}
