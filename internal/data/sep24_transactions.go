package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type SEP24Transaction struct {
	ID        string    `json:"id" db:"id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type SEP24TransactionModel struct {
	dbConnectionPool db.DBConnectionPool
}

// Insert creates a new SEP24 transaction record.
func (m *SEP24TransactionModel) Insert(ctx context.Context, transactionID string) (*SEP24Transaction, error) {
	if transactionID == "" {
		return nil, fmt.Errorf("transaction ID is required")
	}

	const query = `
		INSERT INTO sep24_transactions (id, created_at)
		VALUES ($1, NOW())
		RETURNING id, created_at
	`

	var transaction SEP24Transaction
	err := m.dbConnectionPool.GetContext(ctx, &transaction, query, transactionID)
	if err != nil {
		var pqError *pq.Error
		if errors.As(err, &pqError) {
			if pqError.Code == "23505" { // unique_violation
				return nil, ErrRecordAlreadyExists
			}
		}
		return nil, fmt.Errorf("error inserting SEP24 transaction: %w", err)
	}

	return &transaction, nil
}

// GetByID retrieves a SEP24 transaction by ID.
func (m *SEP24TransactionModel) GetByID(ctx context.Context, transactionID string) (*SEP24Transaction, error) {
	const query = `
		SELECT id, created_at
		FROM sep24_transactions
		WHERE id = $1
	`

	var transaction SEP24Transaction
	err := m.dbConnectionPool.GetContext(ctx, &transaction, query, transactionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying SEP24 transaction: %w", err)
	}

	return &transaction, nil
}
