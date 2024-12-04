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
	ReceiverWalletID  string                 `db:"receiver_wallet_id"`
	IdempotencyKey    string                 `db:"idempotency_key"`
	CircleRecipientID *string                `db:"circle_recipient_id"`
	Status            *CircleRecipientStatus `db:"status"`
	CreatedAt         time.Time              `db:"created_at"`
	UpdatedAt         time.Time              `db:"updated_at"`
	SyncAttempts      int                    `db:"sync_attempts"`
	LastSyncAttemptAt *time.Time             `db:"last_sync_attempt_at"`
}

type CircleRecipientStatus string

const (
	CircleRecipientStatusPending CircleRecipientStatus = "pending"
	CircleRecipientStatusSuccess CircleRecipientStatus = "complete" // means success
	CircleRecipientStatusFailed  CircleRecipientStatus = "failed"
)

func CompletedCircleRecipientStatuses() []CircleRecipientStatus {
	return []CircleRecipientStatus{CircleRecipientStatusSuccess, CircleRecipientStatusFailed}
}

func (s CircleRecipientStatus) IsCompleted() bool {
	return slices.Contains(CompletedCircleRecipientStatuses(), s)
}

func ParseRecipientStatus(statusStr string) (CircleRecipientStatus, error) {
	statusStr = strings.TrimSpace(strings.ToLower(statusStr))

	switch statusStr {
	case string(CircleRecipientStatusPending):
		return CircleRecipientStatusPending, nil
	case string(CircleRecipientStatusSuccess):
		return CircleRecipientStatusSuccess, nil
	case string(CircleRecipientStatusFailed):
		return CircleRecipientStatusFailed, nil
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

func (m CircleRecipientModel) Update(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID string, update CircleRecipientUpdate) (*CircleRecipient, error) {
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
	query = sqlExec.Rebind(query)

	var circleRecipient CircleRecipient
	err := sqlExec.GetContext(ctx, &circleRecipient, query, params...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("circle recipient with receiver_wallet_id %s not found: %w", receiverWalletID, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("updating circle recipient: %w", err)
	}

	return &circleRecipient, nil
}

func (m CircleRecipientModel) GetByReceiverWalletID(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID string) (*CircleRecipient, error) {
	queryParams := QueryParams{
		Filters: map[FilterKey]interface{}{
			FilterKeyReceiverWalletID: receiverWalletID,
		},
	}
	return m.Get(ctx, m.dbConnectionPool, queryParams)
}

func (m CircleRecipientModel) Get(ctx context.Context, sqlExec db.SQLExecuter, queryParams QueryParams) (*CircleRecipient, error) {
	query, params := buildCircleRecipientQuery(baseCircleRecipientQuery, queryParams, sqlExec)

	var circleRecipient CircleRecipient
	err := sqlExec.GetContext(ctx, &circleRecipient, query, params...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("getting circle recipient: %w", err)
	}

	return &circleRecipient, nil
}

const baseCircleRecipientQuery = `
	SELECT
		*
	FROM
		circle_recipients c
`

func buildCircleRecipientQuery(baseQuery string, queryParams QueryParams, sqlExec db.SQLExecuter) (string, []interface{}) {
	qb := NewQueryBuilder(baseQuery)

	if queryParams.Filters[FilterKeyStatus] != nil {
		if statusSlice, ok := queryParams.Filters[FilterKeyStatus].([]CircleRecipientStatus); ok {
			if len(statusSlice) > 0 {
				qb.AddCondition("c.status = ANY(?)", pq.Array(statusSlice))
			}
		} else {
			qb.AddCondition("c.status = ?", queryParams.Filters[FilterKeyStatus])
		}
	}

	if receiverWalletID := queryParams.Filters[FilterKeyReceiverWalletID]; receiverWalletID != nil {
		qb.AddCondition("c.receiver_wallet_id = ?", receiverWalletID)
	}

	if queryParams.Filters[LowerThan(FilterKeySyncAttempts)] != nil {
		qb.AddCondition("c.sync_attempts < ?", queryParams.Filters[LowerThan(FilterKeySyncAttempts)])
	}

	if queryParams.SortBy != "" && queryParams.SortOrder != "" {
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "c")
	}

	if queryParams.PageLimit > 0 && queryParams.Page > 0 {
		qb.AddPagination(queryParams.Page, queryParams.PageLimit)
	}

	qb.forUpdateSkipLocked = queryParams.ForUpdateSkipLocked

	query, params := qb.Build()
	return sqlExec.Rebind(query), params
}
