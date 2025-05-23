package data

import (
	"context"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type ReceiverRegistrationAttemptModel struct {
	dbConnectionPool db.DBConnectionPool
}

type ReceiverRegistrationAttempt struct {
	PhoneNumber   string    `db:"phone_number"`
	Email         string    `db:"email"`
	AttemptTs     time.Time `db:"attempt_ts"`
	ClientDomain  string    `db:"client_domain"`
	TransactionID string    `db:"transaction_id"`
	WalletAddress string    `db:"wallet_address"`
	WalletMemo    string    `db:"wallet_memo"`
}

// InsertReceiverRegistrationAttempt logs a failed wallet-registration attempt.
func (m *ReceiverRegistrationAttemptModel) InsertReceiverRegistrationAttempt(ctx context.Context, attempt ReceiverRegistrationAttempt) error {
	_, err := m.dbConnectionPool.ExecContext(ctx, `
        INSERT INTO receiver_registration_attempts
            (phone_number, email, attempt_ts, client_domain, transaction_id, wallet_address, wallet_memo)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `,
		attempt.PhoneNumber,
		attempt.Email,
		attempt.AttemptTs,
		attempt.ClientDomain,
		attempt.TransactionID,
		attempt.WalletAddress,
		attempt.WalletMemo,
	)
	return err
}
