package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type CircleRecipient struct {
	ReceiverWalletID  string                 `db:"receiver_wallet_id"`
	IdempotencyKey    string                 `db:"idempotency_key"`
	CircleRecipientID *string                `db:"circle_recipient_id"`
	Status            *CircleRecipientStatus `db:"status"`
	CreatedAt         time.Time              `db:"created_at"`
	UpdatedAt         time.Time              `db:"updated_at"`
	SyncAttempts      int                    `db:"sync_attempts"`
	LastSyncAttemptAt *time.Time             `db:"last_sync_attempt_at"`
	ResponseBody      []byte                 `db:"response_body"`
}

type CircleRecipientStatus string

const (
	CircleRecipientStatusPending  CircleRecipientStatus = "pending"
	CircleRecipientStatusActive   CircleRecipientStatus = "active" // means success
	CircleRecipientStatusInactive CircleRecipientStatus = "inactive"
	CircleRecipientStatusDenied   CircleRecipientStatus = "denied"
)

func CompletedCircleRecipientStatuses() []CircleRecipientStatus {
	return []CircleRecipientStatus{CircleRecipientStatusActive, CircleRecipientStatusDenied}
}

func (s CircleRecipientStatus) IsCompleted() bool {
	return slices.Contains(CompletedCircleRecipientStatuses(), s)
}

func ParseRecipientStatus(statusStr string) (CircleRecipientStatus, error) {
	statusStr = strings.TrimSpace(strings.ToLower(statusStr))

	switch statusStr {
	case string(CircleRecipientStatusPending):
		return CircleRecipientStatusPending, nil
	case string(CircleRecipientStatusActive):
		return CircleRecipientStatusActive, nil
	case string(CircleRecipientStatusInactive):
		return CircleRecipientStatusInactive, nil
	case string(CircleRecipientStatusDenied):
		return CircleRecipientStatusDenied, nil
	default:
		return "", fmt.Errorf("unknown recipient status %q", statusStr)
	}
}

type CircleRecipientUpdate struct {
	IdempotencyKey    string                 `db:"idempotency_key"`
	CircleRecipientID *string                `db:"circle_recipient_id"`
	Status            *CircleRecipientStatus `db:"status"`
	SyncAttempts      int                    `db:"sync_attempts"`
	LastSyncAttemptAt *time.Time             `db:"last_sync_attempt_at"`
	ResponseBody      []byte                 `db:"response_body"`
}

type CircleRecipientModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (m CircleRecipientModel) Insert(ctx context.Context, receiverWalletID string) (*CircleRecipient, error) {
	if receiverWalletID == "" {
		return nil, fmt.Errorf("receiverWalletID is required")
	}

	query := `
		INSERT INTO circle_recipients
			(receiver_wallet_id)
		VALUES
			($1)
		RETURNING
			*
	`

	var circleRecipient CircleRecipient
	err := m.dbConnectionPool.GetContext(ctx, &circleRecipient, query, receiverWalletID)
	if err != nil {
		return nil, fmt.Errorf("inserting circle recipient: %w", err)
	}

	return &circleRecipient, nil
}

func (m CircleRecipientModel) Update(ctx context.Context, receiverWalletID string, update CircleRecipientUpdate) (*CircleRecipient, error) {
	if receiverWalletID == "" {
		return nil, fmt.Errorf("receiverWalletID is required")
	}

	setClause, params := BuildSetClause(update)
	if setClause == "" {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		UPDATE
			circle_recipients
		SET
			%s
		WHERE
			receiver_wallet_id = ?
		RETURNING
			*
	`, setClause)
	params = append(params, receiverWalletID)
	query = m.dbConnectionPool.Rebind(query)

	var circleRecipient CircleRecipient
	err := m.dbConnectionPool.GetContext(ctx, &circleRecipient, query, params...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("circle recipient with receiver_wallet_id %s not found: %w", receiverWalletID, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("updating circle recipient: %w", err)
	}

	return &circleRecipient, nil
}

func (m CircleRecipientModel) GetByReceiverWalletID(ctx context.Context, receiverWalletID string) (*CircleRecipient, error) {
	if receiverWalletID == "" {
		return nil, fmt.Errorf("receiverWalletID is required")
	}

	const query = `
		SELECT
			*
		FROM
			circle_recipients c
		WHERE
			c.receiver_wallet_id = $1
	`

	var circleRecipient CircleRecipient
	err := m.dbConnectionPool.GetContext(ctx, &circleRecipient, query, receiverWalletID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("getting circle recipient: %w", err)
	}

	return &circleRecipient, nil
}
