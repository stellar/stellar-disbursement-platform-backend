package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type PasskeySessionModel struct {
	dbConnectionPool db.DBConnectionPool
}

type PasskeySession struct {
	SessionType string    `db:"session_type"`
	SessionData []byte    `db:"session_data"`
	ExpiresAt   time.Time `db:"expires_at"`
}

func NewPasskeySessionModel(dbConnectionPool db.DBConnectionPool) *PasskeySessionModel {
	return &PasskeySessionModel{dbConnectionPool: dbConnectionPool}
}

func (m *PasskeySessionModel) Store(ctx context.Context, challenge string, sessionType string, sessionData []byte, expiresAt time.Time) error {
	const q = `
		INSERT INTO passkey_sessions (challenge, session_type, session_data, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (challenge) DO NOTHING
	`

	result, err := m.dbConnectionPool.ExecContext(ctx, q, challenge, sessionType, sessionData, expiresAt)
	if err != nil {
		return fmt.Errorf("storing passkey session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("storing passkey session: %w", err)
	}
	if rowsAffected == 0 {
		return ErrRecordAlreadyExists
	}

	return nil
}

func (m *PasskeySessionModel) Get(ctx context.Context, challenge string) (*PasskeySession, error) {
	const q = `
		SELECT session_type, session_data, expires_at
		FROM passkey_sessions
		WHERE challenge = $1
	`

	var session PasskeySession
	err := m.dbConnectionPool.GetContext(ctx, &session, q, challenge)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting passkey session: %w", err)
	}

	return &session, nil
}

func (m *PasskeySessionModel) Delete(ctx context.Context, challenge string) error {
	const q = `
		DELETE FROM passkey_sessions
		WHERE challenge = $1
	`

	_, err := m.dbConnectionPool.ExecContext(ctx, q, challenge)
	if err != nil {
		return fmt.Errorf("deleting passkey session: %w", err)
	}
	return nil
}

func (m *PasskeySessionModel) DeleteExpired(ctx context.Context) error {
	const q = `
		DELETE FROM passkey_sessions
		WHERE expires_at <= NOW()
	`

	_, err := m.dbConnectionPool.ExecContext(ctx, q)
	if err != nil {
		return fmt.Errorf("deleting expired passkey sessions: %w", err)
	}
	return nil
}
