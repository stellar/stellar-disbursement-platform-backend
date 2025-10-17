package data

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

var (
	ErrSponsoredTransactionNotFound      = errors.New("sponsored transaction not found")
	ErrInvalidSponsoredTransactionStatus = errors.New("invalid sponsored transaction status")
)

type SponsoredTransactionStatus string

const (
	// The transaction has been created but not yet processed.
	PendingSponsoredTransactionStatus SponsoredTransactionStatus = "PENDING"
	// The transaction is currently being processed.
	ProcessingSponsoredTransactionStatus SponsoredTransactionStatus = "PROCESSING"
	// The transaction has been successfully processed.
	SuccessSponsoredTransactionStatus SponsoredTransactionStatus = "SUCCESS"
	// The transaction has failed.
	FailedSponsoredTransactionStatus SponsoredTransactionStatus = "FAILED"
)

func (status SponsoredTransactionStatus) Validate() error {
	switch SponsoredTransactionStatus(strings.ToUpper(string(status))) {
	case PendingSponsoredTransactionStatus, SuccessSponsoredTransactionStatus, ProcessingSponsoredTransactionStatus, FailedSponsoredTransactionStatus:
		return nil
	default:
		return fmt.Errorf("invalid sponsored transaction status %q", status)
	}
}

type SponsoredTransaction struct {
	ID              string     `json:"id" db:"id"`
	Account         string     `json:"account" db:"account"`
	OperationXDR    string     `json:"operation_xdr" db:"operation_xdr"`
	CreatedAt       *time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at" db:"updated_at"`
	TransactionHash string     `json:"transaction_hash" db:"transaction_hash"`
	Status          string     `json:"status" db:"status"`
}

type SponsoredTransactionModel struct{}

type SponsoredTransactionInsert struct {
	ID           string                     `db:"id"`
	Account      string                     `db:"account"`
	OperationXDR string                     `db:"operation_xdr"`
	Status       SponsoredTransactionStatus `db:"status"`
}

func (sti SponsoredTransactionInsert) Validate() error {
	if sti.ID == "" {
		return fmt.Errorf("id cannot be empty")
	}

	if sti.Account == "" {
		return fmt.Errorf("account cannot be empty")
	}

	if sti.OperationXDR == "" {
		return fmt.Errorf("operation XDR cannot be empty")
	}

	if err := sti.Status.Validate(); err != nil {
		return fmt.Errorf("validating status: %w", err)
	}

	return nil
}

func SponsoredTransactionColumnNames(tableReference, resultAlias string) string {
	config := SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		RawColumns: []string{
			"id",
			"account",
			"operation_xdr",
			"created_at",
			"updated_at",
			"status",
		},
		CoalesceStringColumns: []string{
			"transaction_hash",
		},
	}
	columns := config.Build()

	return strings.Join(columns, ", ")
}

func (m *SponsoredTransactionModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, insert SponsoredTransactionInsert) (*SponsoredTransaction, error) {
	if err := insert.Validate(); err != nil {
		return nil, fmt.Errorf("validating sponsored transaction insert: %w", err)
	}

	query := `
		INSERT INTO sponsored_transactions (id, account, operation_xdr, status)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + SponsoredTransactionColumnNames("", "")

	var transaction SponsoredTransaction
	err := sqlExec.GetContext(ctx, &transaction, query, insert.ID, insert.Account, insert.OperationXDR, insert.Status)
	if err != nil {
		return nil, fmt.Errorf("inserting sponsored transaction: %w", err)
	}

	return &transaction, nil
}

func (m *SponsoredTransactionModel) GetByID(ctx context.Context, sqlExec db.SQLExecuter, id string) (*SponsoredTransaction, error) {
	if id == "" {
		return nil, fmt.Errorf("transaction ID is required")
	}

	query := `
		SELECT ` + SponsoredTransactionColumnNames("", "") + `
		FROM sponsored_transactions
		WHERE id = $1
	`

	var transaction SponsoredTransaction
	err := sqlExec.GetContext(ctx, &transaction, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("getting sponsored transaction by ID %s: %w", id, err)
	}

	return &transaction, nil
}

type SponsoredTransactionUpdate struct {
	Status          SponsoredTransactionStatus `db:"status"`
	TransactionHash string                     `db:"transaction_hash"`
}

func (stu SponsoredTransactionUpdate) Validate() error {
	if utils.IsEmpty(stu) {
		return fmt.Errorf("no values provided to update sponsored transaction")
	}

	if stu.Status != "" {
		if err := stu.Status.Validate(); err != nil {
			return fmt.Errorf("validating status: %w", err)
		}
	}

	if stu.TransactionHash != "" {
		if len(stu.TransactionHash) != 64 {
			return fmt.Errorf("transaction hash must be 64 characters, got %d", len(stu.TransactionHash))
		}
		if _, err := hex.DecodeString(stu.TransactionHash); err != nil {
			return fmt.Errorf("transaction hash must be valid hexadecimal: %w", err)
		}
	}

	return nil
}

func (m *SponsoredTransactionModel) Update(ctx context.Context, sqlExec db.SQLExecuter, id string, update SponsoredTransactionUpdate) error {
	if id == "" {
		return fmt.Errorf("transaction ID is required")
	}

	if err := update.Validate(); err != nil {
		return fmt.Errorf("validating sponsored transaction update: %w", err)
	}

	setClause, params := BuildSetClause(update)
	if setClause == "" {
		return fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		UPDATE sponsored_transactions
		SET %s
		WHERE id = ?
	`, setClause)
	params = append(params, id)
	query = sqlExec.Rebind(query)

	result, err := sqlExec.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("updating sponsored transaction: %w", err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return fmt.Errorf("no sponsored transactions could be found: %w", ErrRecordNotFound)
	}

	return nil
}
