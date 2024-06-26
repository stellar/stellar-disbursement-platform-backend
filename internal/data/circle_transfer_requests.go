package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type CircleTransferRequest struct {
	IdempotencyKey    string                `db:"idempotency_key"`
	PaymentID         string                `db:"payment_id"`
	CircleTransferID  *string               `db:"circle_transfer_id,omitempty"`
	Status            *CircleTransferStatus `db:"status"`
	ResponseBody      []byte                `db:"response_body,omitempty"`
	SourceWalletID    *string               `db:"source_wallet_id,omitempty"`
	CreatedAt         time.Time             `db:"created_at"`
	UpdatedAt         time.Time             `db:"updated_at"`
	CompletedAt       *time.Time            `db:"completed_at,omitempty"`
	LastSyncAttemptAt *time.Time            `db:"last_sync_attempt_at,omitempty"`
	SyncAttempts      int                   `db:"sync_attempts"`
}

type CircleTransferStatus string

const (
	CircleTransferStatusPending CircleTransferStatus = "pending"
	CircleTransferStatusSuccess CircleTransferStatus = "complete" // means success
	CircleTransferStatusFailed  CircleTransferStatus = "failed"
)

func CompletedCircleStatuses() []CircleTransferStatus {
	return []CircleTransferStatus{CircleTransferStatusSuccess, CircleTransferStatusFailed}
}

func (s CircleTransferStatus) IsCompleted() bool {
	return slices.Contains(CompletedCircleStatuses(), s)
}

type CircleTransferRequestUpdate struct {
	CircleTransferID string
	Status           CircleTransferStatus
	ResponseBody     []byte
	SourceWalletID   string
	CompletedAt      *time.Time
}

type CircleTransferRequestModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (m CircleTransferRequestModel) GetOrInsert(ctx context.Context, paymentID string) (*CircleTransferRequest, error) {
	if paymentID == "" {
		return nil, fmt.Errorf("paymentID is required")
	}

	return db.RunInTransactionWithResult(ctx, m.dbConnectionPool, nil, func(dbTx db.DBTransaction) (*CircleTransferRequest, error) {
		// validate that the payment ID exists
		var paymentIDExists bool
		err := dbTx.GetContext(ctx, &paymentIDExists, "SELECT EXISTS(SELECT 1 FROM payments WHERE id = $1)", paymentID)
		if err != nil {
			return nil, fmt.Errorf("getting payment by ID: %w", err)
		}
		if !paymentIDExists {
			return nil, fmt.Errorf("payment with ID %s does not exist: %w", paymentID, ErrRecordNotFound)
		}

		circleTransferRequest, err := m.FindIncompleteByPaymentID(ctx, m.dbConnectionPool, paymentID)
		if err != nil && !errors.Is(err, ErrRecordNotFound) {
			return nil, fmt.Errorf("finding incomplete circle transfer by payment ID: %w", err)
		}

		if circleTransferRequest != nil {
			return circleTransferRequest, nil
		}

		return m.Insert(ctx, paymentID)
	})
}

func (m CircleTransferRequestModel) Insert(ctx context.Context, paymentID string) (*CircleTransferRequest, error) {
	if paymentID == "" {
		return nil, fmt.Errorf("paymentID is required")
	}

	query := `
		INSERT INTO circle_transfer_requests
			(payment_id)
		VALUES
			($1)
		RETURNING
			*
	`

	var circleTransferRequest CircleTransferRequest
	err := m.dbConnectionPool.GetContext(ctx, &circleTransferRequest, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("inserting circle transfer request: %w", err)
	}

	return &circleTransferRequest, nil
}

func (m CircleTransferRequestModel) FindIncompleteByPaymentID(ctx context.Context, sqlExec db.SQLExecuter, paymentID string) (*CircleTransferRequest, error) {
	if paymentID == "" {
		return nil, fmt.Errorf("paymentID is required")
	}

	queryParams := QueryParams{
		Filters: map[FilterKey]interface{}{
			FilterKeyPaymentID:           paymentID,
			IsNull(FilterKeyCompletedAt): true,
		},
		SortBy:    "created_at",
		SortOrder: "DESC",
	}
	return m.Get(ctx, m.dbConnectionPool, queryParams)
}

const baseQuery = `
	SELECT
		*
	FROM
		circle_transfer_requests c
`

func (m CircleTransferRequestModel) GetAll(ctx context.Context, sqlExec db.SQLExecuter, queryParams QueryParams) ([]*CircleTransferRequest, error) {
	query, params := buildCircleTransferRequestQuery(baseQuery, queryParams, sqlExec)

	var circleTransferRequests []*CircleTransferRequest
	err := sqlExec.SelectContext(ctx, &circleTransferRequests, query, params...)
	if err != nil {
		return nil, fmt.Errorf("getting circle transfer requests: %w", err)
	}

	return circleTransferRequests, nil
}

func (m CircleTransferRequestModel) Get(ctx context.Context, sqlExec db.SQLExecuter, queryParams QueryParams) (*CircleTransferRequest, error) {
	query, params := buildCircleTransferRequestQuery(baseQuery, queryParams, sqlExec)

	var circleTransferRequests CircleTransferRequest
	err := sqlExec.GetContext(ctx, &circleTransferRequests, query, params...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("getting circle transfer request: %w", err)
	}

	return &circleTransferRequests, nil
}

func (m CircleTransferRequestModel) Update(ctx context.Context, sqlExec db.SQLExecuter, idempotencyKey string, update CircleTransferRequestUpdate) (*CircleTransferRequest, error) {
	if idempotencyKey == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}

	query := `
		UPDATE
			circle_transfer_requests
		SET
			circle_transfer_id = $2,
			status = $3,
			response_body = $4,
			source_wallet_id = $5,
			completed_at = $6
		WHERE
			idempotency_key = $1
		RETURNING
			*
	`

	var circleTransferRequest CircleTransferRequest
	err := sqlExec.GetContext(ctx, &circleTransferRequest, query, idempotencyKey, update.CircleTransferID, update.Status, update.ResponseBody, update.SourceWalletID, update.CompletedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("circle transfer request with idempotency key %s not found: %w", idempotencyKey, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("updating circle transfer request: %w", err)
	}

	return &circleTransferRequest, nil
}

func buildCircleTransferRequestQuery(baseQuery string, queryParams QueryParams, sqlExec db.SQLExecuter) (string, []interface{}) {
	qb := NewQueryBuilder(baseQuery)

	if queryParams.Filters[FilterKeyStatus] != nil {
		if statusSlice, ok := queryParams.Filters[FilterKeyStatus].([]CircleTransferStatus); ok {
			if len(statusSlice) > 0 {
				qb.AddCondition("c.status = ANY(?)", pq.Array(statusSlice))
			}
		} else {
			qb.AddCondition("c.status = ?", queryParams.Filters[FilterKeyStatus])
		}
	}

	if paymentID := queryParams.Filters[FilterKeyPaymentID]; paymentID != nil {
		qb.AddCondition("c.payment_id = ?", paymentID)
	}

	if queryParams.Filters[IsNull(FilterKeyCompletedAt)] != nil {
		qb.AddCondition("c." + string(IsNull(FilterKeyCompletedAt)))
	}

	if queryParams.SortBy != "" && queryParams.SortOrder != "" {
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "c")
	}

	if queryParams.PageLimit > 0 && queryParams.Page > 0 {
		qb.AddPagination(queryParams.Page, queryParams.PageLimit)
	}

	query, params := qb.Build()
	return sqlExec.Rebind(query), params
}
