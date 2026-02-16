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
	CircleTransferID  *string               `db:"circle_transfer_id"`
	CirclePayoutID    *string               `db:"circle_payout_id"`
	Status            *CircleTransferStatus `db:"status"`
	ResponseBody      []byte                `db:"response_body"`
	SourceWalletID    *string               `db:"source_wallet_id"`
	CreatedAt         time.Time             `db:"created_at"`
	UpdatedAt         time.Time             `db:"updated_at"`
	CompletedAt       *time.Time            `db:"completed_at"`
	LastSyncAttemptAt *time.Time            `db:"last_sync_attempt_at"`
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
	CircleTransferID  string               `db:"circle_transfer_id"`
	CirclePayoutID    string               `db:"circle_payout_id"`
	Status            CircleTransferStatus `db:"status"`
	ResponseBody      []byte               `db:"response_body"`
	SourceWalletID    string               `db:"source_wallet_id"`
	CompletedAt       *time.Time           `db:"completed_at"`
	LastSyncAttemptAt *time.Time           `db:"last_sync_attempt_at"`
	SyncAttempts      int                  `db:"sync_attempts"`
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

		circleTransferRequest, err := m.GetIncompleteByPaymentID(ctx, dbTx, paymentID)
		if err != nil && !errors.Is(err, ErrRecordNotFound) {
			return nil, fmt.Errorf("finding incomplete circle transfer by payment ID: %w", err)
		}

		if circleTransferRequest != nil {
			return circleTransferRequest, nil
		}

		return m.Insert(ctx, dbTx, paymentID)
	})
}

func (m CircleTransferRequestModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, paymentID string) (*CircleTransferRequest, error) {
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
	err := sqlExec.GetContext(ctx, &circleTransferRequest, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("inserting circle transfer request: %w", err)
	}

	return &circleTransferRequest, nil
}

func (m CircleTransferRequestModel) GetIncompleteByPaymentID(ctx context.Context, sqlExec db.SQLExecuter, paymentID string) (*CircleTransferRequest, error) {
	queryParams := QueryParams{
		Filters: map[FilterKey]interface{}{
			FilterKeyPaymentID:           paymentID,
			IsNull(FilterKeyCompletedAt): true,
		},
		SortBy:    "created_at",
		SortOrder: SortOrderDESC,
	}
	return m.Get(ctx, sqlExec, queryParams)
}

const (
	maxSyncAttempts = 10
	batchSize       = 10
)

// GetPendingReconciliation returns the pending Circle transfer requests that are in `pending` status and have not
// reached the maximum sync attempts.
func (m CircleTransferRequestModel) GetPendingReconciliation(ctx context.Context, sqlExec db.SQLExecuter) ([]*CircleTransferRequest, error) {
	queryParams := QueryParams{
		Filters: map[FilterKey]interface{}{
			FilterKeyStatus:                  []CircleTransferStatus{CircleTransferStatusPending},
			LowerThan(FilterKeySyncAttempts): maxSyncAttempts,
		},
		SortBy:              "last_sync_attempt_at",
		SortOrder:           SortOrderASC,
		Page:                1,
		PageLimit:           batchSize,
		ForUpdateSkipLocked: true,
	}
	return m.GetAll(ctx, sqlExec, queryParams)
}

const baseCircleQuery = `
	SELECT
		*
	FROM
		circle_transfer_requests c
`

func (m CircleTransferRequestModel) GetAll(ctx context.Context, sqlExec db.SQLExecuter, queryParams QueryParams) ([]*CircleTransferRequest, error) {
	query, params := buildCircleTransferRequestQuery(baseCircleQuery, queryParams, sqlExec)

	var circleTransferRequests []*CircleTransferRequest
	err := sqlExec.SelectContext(ctx, &circleTransferRequests, query, params...)
	if err != nil {
		return nil, fmt.Errorf("getting circle transfer requests: %w", err)
	}

	return circleTransferRequests, nil
}

func (m CircleTransferRequestModel) Get(ctx context.Context, sqlExec db.SQLExecuter, queryParams QueryParams) (*CircleTransferRequest, error) {
	query, params := buildCircleTransferRequestQuery(baseCircleQuery, queryParams, sqlExec)

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

func (m CircleTransferRequestModel) GetCurrentTransfersForPaymentIDs(ctx context.Context, sqlExec db.SQLExecuter, paymentIDs []string) (map[string]*CircleTransferRequest, error) {
	if len(paymentIDs) == 0 {
		return nil, fmt.Errorf("paymentIDs is required")
	}

	query := `
		SELECT DISTINCT ON (payment_id)
			*
		FROM 
			circle_transfer_requests
		WHERE 
			payment_id = ANY($1)
		ORDER BY 
			payment_id, created_at DESC;
	`

	var circleTransferRequests []*CircleTransferRequest
	err := sqlExec.SelectContext(ctx, &circleTransferRequests, query, pq.Array(paymentIDs))
	if err != nil {
		return nil, fmt.Errorf("getting circle transfer requests: %w", err)
	}

	circleTransferRequestsByPaymentID := make(map[string]*CircleTransferRequest)
	if len(circleTransferRequests) == 0 {
		return circleTransferRequestsByPaymentID, nil
	}

	for _, circleTransferRequest := range circleTransferRequests {
		circleTransferRequestsByPaymentID[circleTransferRequest.PaymentID] = circleTransferRequest
	}

	return circleTransferRequestsByPaymentID, nil
}

func (m CircleTransferRequestModel) Update(ctx context.Context, sqlExec db.SQLExecuter, idempotencyKey string, update CircleTransferRequestUpdate) (*CircleTransferRequest, error) {
	if idempotencyKey == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}

	setClause, params := BuildSetClause(update)
	if setClause == "" {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		UPDATE
			circle_transfer_requests
		SET
			%s
		WHERE
			idempotency_key = ?
		RETURNING
			*
	`, setClause)
	params = append(params, idempotencyKey)
	query = sqlExec.Rebind(query)

	var circleTransferRequest CircleTransferRequest
	err := sqlExec.GetContext(ctx, &circleTransferRequest, query, params...)
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
