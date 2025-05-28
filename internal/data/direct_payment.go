package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// DirectPaymentInsert represents the data needed to insert a direct payment
type DirectPaymentInsert struct {
	ReceiverID        string  `db:"receiver_id"`
	Amount            string  `db:"amount"`
	AssetID           string  `db:"asset_id"`
	ReceiverWalletID  string  `db:"receiver_wallet_id"`
	ExternalPaymentID *string `db:"external_payment_id"`
}

// Validate validates the direct payment insert
func (dpi *DirectPaymentInsert) Validate() error {
	if strings.TrimSpace(dpi.ReceiverID) == "" {
		return fmt.Errorf("receiver_id is required")
	}

	if err := utils.ValidateAmount(dpi.Amount); err != nil {
		return fmt.Errorf("amount is invalid: %w", err)
	}

	if strings.TrimSpace(dpi.AssetID) == "" {
		return fmt.Errorf("asset_id is required")
	}

	if strings.TrimSpace(dpi.ReceiverWalletID) == "" {
		return fmt.Errorf("receiver_wallet_id is required")
	}

	return nil
}

// DirectPaymentModel handles direct payment database operations
type DirectPaymentModel struct {
	dbConnectionPool db.DBConnectionPool
}

// Insert creates a new direct payment in the database
func (dp *DirectPaymentModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, insert DirectPaymentInsert) (string, error) {
	if err := insert.Validate(); err != nil {
		return "", fmt.Errorf("validating direct payment insert: %w", err)
	}

	query := `
		INSERT INTO payments (
			amount,
			asset_id,
			receiver_id,
			disbursement_id,
			receiver_wallet_id,
			external_payment_id,
			status,
			status_history
		) VALUES (
			$1,
			$2,
			$3,
			NULL,
			$4,
			$5,
			'READY',
			ARRAY[create_payment_status_history(NOW(), 'READY', 'Direct payment created')]
		)
		RETURNING id
	`

	var newID string
	err := sqlExec.GetContext(ctx, &newID, query,
		insert.Amount,
		insert.AssetID,
		insert.ReceiverID,
		insert.ReceiverWalletID,
		insert.ExternalPaymentID,
	)
	if err != nil {
		return "", fmt.Errorf("inserting direct payment: %w", err)
	}

	return newID, nil
}

// GetDirectPaymentsByReceiverWalletID returns all direct payments for a specific receiver wallet
func (dp *DirectPaymentModel) GetDirectPaymentsByReceiverWalletID(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID string) ([]*Payment, error) {
	query := `
		SELECT
			` + PaymentColumnNames("p", "") + `,
			` + AssetColumnNames("a", "asset", false) + `
		FROM
			payments p
			JOIN assets a ON p.asset_id = a.id
		WHERE 
			p.disbursement_id IS NULL 
			AND p.receiver_wallet_id = $1
		ORDER BY p.created_at DESC
	`

	var payments []*Payment
	err := sqlExec.SelectContext(ctx, &payments, query, receiverWalletID)
	if err != nil {
		return nil, fmt.Errorf("getting direct payments for receiver wallet ID %s: %w", receiverWalletID, err)
	}

	return payments, nil
}

// GetDirectPaymentsByReceiverID returns all direct payments for a specific receiver
func (dp *DirectPaymentModel) GetDirectPaymentsByReceiverID(ctx context.Context, sqlExec db.SQLExecuter, receiverID string) ([]*Payment, error) {
	query := `
		SELECT
			` + PaymentColumnNames("p", "") + `,
			` + AssetColumnNames("a", "asset", false) + `,
			` + ReceiverWalletColumnNames("rw", "receiver_wallet") + `
		FROM
			payments p
			JOIN assets a ON p.asset_id = a.id
			JOIN receiver_wallets rw ON p.receiver_wallet_id = rw.id
		WHERE 
			p.disbursement_id IS NULL 
			AND p.receiver_id = $1
		ORDER BY p.created_at DESC
	`

	var payments []*Payment
	err := sqlExec.SelectContext(ctx, &payments, query, receiverID)
	if err != nil {
		return nil, fmt.Errorf("getting direct payments for receiver ID %s: %w", receiverID, err)
	}

	return payments, nil
}

// GetAllDirectPayments returns direct payments with optional filtering and pagination
func (dp *DirectPaymentModel) GetAllDirectPayments(ctx context.Context, sqlExec db.SQLExecuter, queryParams *QueryParams) ([]*Payment, error) {
	baseQuery := `
		SELECT
			` + PaymentColumnNames("p", "") + `,
			` + AssetColumnNames("a", "asset", false) + `,
			` + ReceiverWalletColumnNames("rw", "receiver_wallet") + `
		FROM
			payments p
			JOIN assets a ON p.asset_id = a.id
			JOIN receiver_wallets rw ON p.receiver_wallet_id = rw.id
		WHERE
			p.disbursement_id IS NULL
	`

	qb := NewQueryBuilder(baseQuery)

	// Add filters
	if receiverID, ok := queryParams.Filters[FilterKeyReceiverID]; ok {
		qb.AddCondition("p.receiver_id = ?", receiverID)
	}
	if status, ok := queryParams.Filters[FilterKeyStatus]; ok {
		qb.AddCondition("p.status = ?", status)
	}
	if createdAfter, ok := queryParams.Filters[FilterKeyCreatedAtAfter]; ok {
		qb.AddCondition("p.created_at >= ?", createdAfter)
	}
	if createdBefore, ok := queryParams.Filters[FilterKeyCreatedAtBefore]; ok {
		qb.AddCondition("p.created_at <= ?", createdBefore)
	}

	// Add search
	if queryParams.Query != "" {
		q := "%" + queryParams.Query + "%"
		qb.AddCondition("(p.id ILIKE ? OR p.external_payment_id ILIKE ? OR rw.stellar_address ILIKE ?)", q, q, q)
	}

	// Add pagination and sorting
	qb.AddPagination(queryParams.Page, queryParams.PageLimit)
	qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "p")

	query, args := qb.BuildAndRebind(sqlExec)

	var payments []*Payment
	err := sqlExec.SelectContext(ctx, &payments, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting direct payments: %w", err)
	}

	return payments, nil
}
