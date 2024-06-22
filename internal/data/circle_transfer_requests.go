package data

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type CircleTransferRequest struct {
	IdempotencyKey   string                `db:"idempotency_key"`
	PaymentID        string                `db:"payment_id"`
	CircleTransferID *string               `db:"circle_transfer_id,omitempty"`
	Status           *CircleTransferStatus `db:"status"`
	ResponseBody     []byte                `db:"response_body,omitempty"`
	SourceWalletID   *string               `db:"source_wallet_id,omitempty"`
	CreatedAt        time.Time             `db:"created_at"`
	UpdatedAt        time.Time             `db:"updated_at"`
	CompletedAt      *time.Time            `db:"completed_at,omitempty"`
}

type CircleTransferStatus string

const (
	CircleTransferStatusPending CircleTransferStatus = "pending"
	CircleTransferStatusSuccess CircleTransferStatus = "success"
	CircleTransferStatusFailed  CircleTransferStatus = "failed"
)

type CircleTransferRequestUpdate struct {
	CircleTransferID string
	Status           CircleTransferStatus
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

	return db.RunInTransactionWithResult(ctx, m.dbConnectionPool, nil, func(tx db.DBTransaction) (*CircleTransferRequest, error) {
		// validate payment ID exists
		var paymentIDExists bool
		err := tx.GetContext(ctx, &paymentIDExists, "SELECT EXISTS(SELECT 1 FROM payments WHERE id = $1)", paymentID)
		if err != nil || !paymentIDExists {
			return nil, fmt.Errorf("payment ID %s is not valid: %w", paymentID, err)
		}

		circleTransferRequest, err := m.FindNotCompletedByPaymentID(ctx, m.dbConnectionPool, paymentID)
		if err != nil {
			return nil, fmt.Errorf("finding circle transfer request: %w", err)
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
	INSERT INTO circle_transfer_requests (payment_id)
	VALUES ($1)
	RETURNING *
	`

	var circleTransferRequest CircleTransferRequest
	err := m.dbConnectionPool.GetContext(ctx, &circleTransferRequest, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("inserting circle transfer request: %w", err)
	}
	return &circleTransferRequest, nil
}

func (m CircleTransferRequestModel) FindNotCompletedByPaymentID(ctx context.Context, sqlExec db.SQLExecuter, paymentID string) (*CircleTransferRequest, error) {
	if paymentID == "" {
		return nil, fmt.Errorf("paymentID is required")
	}

	query := `
		SELECT * FROM circle_transfer_requests
		WHERE payment_id = $1 AND completed_at IS NULL
		ORDER BY created_at DESC
	`

	var circleTransferRequests []CircleTransferRequest
	err := sqlExec.SelectContext(ctx, &circleTransferRequests, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("finding circle transfer request: %w", err)
	}

	if len(circleTransferRequests) == 0 {
		return nil, nil
	} else if len(circleTransferRequests) > 1 {
		return nil, fmt.Errorf("multiple incomplete transfer requests found for paymentID %s", paymentID)
	}

	return &circleTransferRequests[0], nil
}

func (m CircleTransferRequestModel) Update(ctx context.Context, sqlExec db.SQLExecuter, idempotencyKey string, update CircleTransferRequestUpdate) error {
	if idempotencyKey == "" {
		return fmt.Errorf("idempotencyKey is required")
	}

	query := `
	UPDATE circle_transfer_requests
	SET circle_transfer_id = $2, status = $3, response_body = $4, source_wallet_id = $5, completed_at = $6
	WHERE idempotency_key = $1
	`

	_, err := sqlExec.ExecContext(ctx, query, idempotencyKey, update.CircleTransferID, update.Status, update.ResponseBody, update.SourceWalletID, update.CompletedAt)
	if err != nil {
		return fmt.Errorf("updating circle transfer request: %w", err)
	}
	return nil
}
