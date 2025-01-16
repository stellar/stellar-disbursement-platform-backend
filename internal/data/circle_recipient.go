package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type CircleRecipient struct {
	ReceiverWalletID  string                `db:"receiver_wallet_id"`
	IdempotencyKey    string                `db:"idempotency_key"`
	CircleRecipientID string                `db:"circle_recipient_id"`
	Status            CircleRecipientStatus `db:"status"`
	CreatedAt         time.Time             `db:"created_at"`
	UpdatedAt         time.Time             `db:"updated_at"`
	SyncAttempts      int                   `db:"sync_attempts"`
	LastSyncAttemptAt time.Time             `db:"last_sync_attempt_at"`
	ResponseBody      []byte                `db:"response_body"`
}

type CircleRecipientStatus string

func (s *CircleRecipientStatus) Scan(value interface{}) error {
	if value == nil {
		*s = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*s = CircleRecipientStatus(v)
	case []uint8:
		*s = CircleRecipientStatus(string(v)) // Convert byte slice to string
	default:
		return fmt.Errorf("invalid type for CircleRecipientStatus: %T", value)
	}

	return nil
}

const (
	CircleRecipientStatusPending  CircleRecipientStatus = "pending"
	CircleRecipientStatusActive   CircleRecipientStatus = "active" // means success
	CircleRecipientStatusInactive CircleRecipientStatus = "inactive"
	CircleRecipientStatusDenied   CircleRecipientStatus = "denied"
	CircleRecipientStatusFailed   CircleRecipientStatus = "failed"
)

func CompletedCircleRecipientStatuses() []CircleRecipientStatus {
	return []CircleRecipientStatus{CircleRecipientStatusActive, CircleRecipientStatusDenied, CircleRecipientStatusFailed, CircleRecipientStatusInactive}
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
	case string(CircleRecipientStatusFailed):
		return CircleRecipientStatusFailed, nil
	default:
		return "", fmt.Errorf("unknown recipient status %q", statusStr)
	}
}

type CircleRecipientUpdate struct {
	IdempotencyKey    string                `db:"idempotency_key"`
	CircleRecipientID string                `db:"circle_recipient_id"`
	Status            CircleRecipientStatus `db:"status"`
	SyncAttempts      int                   `db:"sync_attempts"`
	LastSyncAttemptAt time.Time             `db:"last_sync_attempt_at"`
	ResponseBody      []byte                `db:"response_body"`
}

type CircleRecipientModel struct {
	dbConnectionPool db.DBConnectionPool
}

const circleRecipientFields = `
	receiver_wallet_id,
	idempotency_key,
	COALESCE(circle_recipient_id, '') AS circle_recipient_id,
	status,
	created_at,
	updated_at,
	sync_attempts,
	COALESCE(last_sync_attempt_at, '0001-01-01 00:00:00+00') AS last_sync_attempt_at,
	response_body
`

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
	` + circleRecipientFields

	var circleRecipient CircleRecipient
	err := m.dbConnectionPool.GetContext(ctx, &circleRecipient, query, receiverWalletID)
	if err != nil {
		return nil, fmt.Errorf("getting context: %w", err)
	}

	return &circleRecipient, nil
}

// ResetRecipientsForRetryIfNeeded resets the status of the circle recipients for the given payment IDs to NULL if the status is not active.
func (m CircleRecipientModel) ResetRecipientsForRetryIfNeeded(ctx context.Context, sqlExec db.SQLExecuter, paymentIDs ...string) ([]*CircleRecipient, error) {
	if len(paymentIDs) == 0 {
		return nil, fmt.Errorf("at least one payment ID is required: %w", ErrMissingInput)
	}

	const query = `
		UPDATE
			circle_recipients
		SET
			status = NULL,
			sync_attempts = 0,
			last_sync_attempt_at = NULL,
			response_body = NULL
		WHERE 
			receiver_wallet_id IN (
				SELECT DISTINCT receiver_wallet_id
				FROM payments
				WHERE id = ANY($1)
			)
		 	AND (status != $2 OR status IS NULL)
		RETURNING
	` + circleRecipientFields

	var updatedRecipients []*CircleRecipient
	err := sqlExec.SelectContext(ctx, &updatedRecipients, query, pq.Array(paymentIDs), CircleRecipientStatusActive)
	if err != nil {
		return nil, fmt.Errorf("getting context: %w", err)
	}

	return updatedRecipients, nil
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
		RETURNING `+circleRecipientFields,
		setClause)
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

	query := fmt.Sprintf(`
		SELECT
			%s
		FROM
			circle_recipients c
		WHERE
			c.receiver_wallet_id = $1
	`, circleRecipientFields)

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
