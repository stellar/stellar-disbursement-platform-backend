package data

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type CircleTransferRequest struct {
	ID               string     `db:"id" json:"id"`
	PaymentID        string     `db:"payment_id" json:"payment_id"`
	CircleTransferID *string    `db:"circle_transfer_id,omitempty" json:"circle_transfer_id,omitempty"`
	ResponseBody     []byte     `db:"response_body,omitempty" json:"response_body,omitempty"`
	SourceWalletID   *string    `db:"source_wallet_id,omitempty" json:"source_wallet_id,omitempty"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
	CompletedAt      *time.Time `db:"completed_at,omitempty" json:"completed_at,omitempty"`
}

type CircleTransferRequestUpdate struct {
	CircleTransferID string
	ResponseBody     []byte
	SourceWalletID   string
	CompletedAt      time.Time
}

type CircleTransferRequestModel struct {
	dbConnectionPool db.DBConnectionPool
}

func (m CircleTransferRequestModel) FindOrInsert(ctx context.Context, paymentID string) (*CircleTransferRequest, error) {
	if paymentID == "" {
		return nil, fmt.Errorf("paymentID is required")
	}

	query := `
    WITH existing_request AS (
        SELECT * FROM circle_transfer_requests
        WHERE payment_id = $1 AND completed_at IS NULL
        ORDER BY created_at DESC
        LIMIT 1
    ),
    insert_request AS (
        INSERT INTO circle_transfer_requests (payment_id)
        SELECT $1
        WHERE NOT EXISTS (SELECT 1 FROM existing_request)
        RETURNING *
    )
    SELECT * FROM existing_request
    UNION ALL
    SELECT * FROM insert_request
    LIMIT 1
    `

	var circleTransferRequest CircleTransferRequest
	err := m.dbConnectionPool.GetContext(ctx, &circleTransferRequest, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("inserting circle transfer request: %w", err)
	}
	return &circleTransferRequest, nil
}

func (m CircleTransferRequestModel) Update(ctx context.Context, sqlExec db.SQLExecuter, id string, update CircleTransferRequestUpdate) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}

	query := `
	UPDATE circle_transfer_requests
	SET circle_transfer_id = $2, response_body = $3, source_wallet_id = $4, completed_at = $5
	WHERE id = $1
	`

	_, err := sqlExec.ExecContext(ctx, query, id, update.CircleTransferID, update.ResponseBody, update.SourceWalletID, update.CompletedAt)
	if err != nil {
		return fmt.Errorf("updating circle transfer request: %w", err)
	}
	return nil
}
