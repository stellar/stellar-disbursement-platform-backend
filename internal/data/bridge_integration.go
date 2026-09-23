package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

// BridgeIntegration represents a Bridge integration record with simplified status tracking.
type BridgeIntegration struct {
	Status                  BridgeIntegrationStatus `db:"status"`
	KYCLinkID               *string                 `db:"kyc_link_id"`
	CustomerID              *string                 `db:"customer_id"`
	OptedInBy               *string                 `db:"opted_in_by"`
	OptedInAt               *time.Time              `db:"opted_in_at"`
	VirtualAccountID        *string                 `db:"virtual_account_id"`
	VirtualAccountCreatedBy *string                 `db:"virtual_account_created_by"`
	VirtualAccountCreatedAt *time.Time              `db:"virtual_account_created_at"`
	ErrorMessage            *string                 `db:"error_message"`
	CreatedAt               time.Time               `db:"created_at"`
	UpdatedAt               time.Time               `db:"updated_at"`
}

// BridgeIntegrationStatus represents the integration status.
type BridgeIntegrationStatus string

const (
	BridgeIntegrationStatusNotEnabled      BridgeIntegrationStatus = "NOT_ENABLED"
	BridgeIntegrationStatusNotOptedIn      BridgeIntegrationStatus = "NOT_OPTED_IN"
	BridgeIntegrationStatusOptedIn         BridgeIntegrationStatus = "OPTED_IN"
	BridgeIntegrationStatusReadyForDeposit BridgeIntegrationStatus = "READY_FOR_DEPOSIT"
	BridgeIntegrationStatusError           BridgeIntegrationStatus = "ERROR"
)

// BridgeIntegrationModel provides database operations for Bridge integration.
type BridgeIntegrationModel struct {
	dbConnectionPool db.DBConnectionPool
}

var bridgeIntegrationColumns = strings.Join([]string{
	"status",
	"kyc_link_id",
	"customer_id",
	"opted_in_by",
	"opted_in_at",
	"virtual_account_id",
	"virtual_account_created_by",
	"virtual_account_created_at",
	"error_message",
	"created_at",
	"updated_at",
}, ", ")

// Get retrieves the Bridge integration record.
func (m *BridgeIntegrationModel) Get(ctx context.Context) (*BridgeIntegration, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM bridge_integration 
		LIMIT 1`, bridgeIntegrationColumns)

	var integration BridgeIntegration
	err := m.dbConnectionPool.GetContext(ctx, &integration, query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("getting Bridge integration: %w", err)
	}

	return &integration, nil
}

type BridgeIntegrationInsert struct {
	KYCLinkID  *string
	CustomerID string
	OptedInBy  string
}

func (bii BridgeIntegrationInsert) Validate() error {
	if bii.KYCLinkID != nil && strings.TrimSpace(*bii.KYCLinkID) == "" {
		return fmt.Errorf("KYCLinkID is empty")
	}
	if bii.CustomerID == "" {
		return fmt.Errorf("CustomerID is required")
	}
	if bii.OptedInBy == "" {
		return fmt.Errorf("OptedInBy is required")
	}
	return nil
}

// Insert creates a new Bridge integration record.
func (m *BridgeIntegrationModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, insert BridgeIntegrationInsert) (*BridgeIntegration, error) {
	if err := insert.Validate(); err != nil {
		return nil, fmt.Errorf("validating Bridge integration insert: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO bridge_integration (
			kyc_link_id, customer_id, status, opted_in_by, opted_in_at
		) VALUES (
			$1, $2, $3, $4, $5
		) RETURNING %s`, bridgeIntegrationColumns)

	var integration BridgeIntegration
	err := sqlExec.GetContext(ctx, &integration, query,
		insert.KYCLinkID, insert.CustomerID, BridgeIntegrationStatusOptedIn, insert.OptedInBy, time.Now())
	if err != nil {
		return nil, fmt.Errorf("inserting Bridge integration: %w", err)
	}

	return &integration, nil
}

// BridgeIntegrationUpdate represents fields that can be updated.
type BridgeIntegrationUpdate struct {
	Status                  *BridgeIntegrationStatus `db:"status"`
	KYCLinkID               *string                  `db:"kyc_link_id"`
	CustomerID              *string                  `db:"customer_id"`
	OptedInBy               *string                  `db:"opted_in_by"`
	OptedInAt               *time.Time               `db:"opted_in_at"`
	VirtualAccountID        *string                  `db:"virtual_account_id"`
	VirtualAccountCreatedBy *string                  `db:"virtual_account_created_by"`
	VirtualAccountCreatedAt *time.Time               `db:"virtual_account_created_at"`
	ErrorMessage            *string                  `db:"error_message"`
}

// Update updates the Bridge integration record.
func (m *BridgeIntegrationModel) Update(ctx context.Context, update BridgeIntegrationUpdate) (*BridgeIntegration, error) {
	fields := []string{}
	args := []any{}

	if update.Status != nil {
		fields = append(fields, "status = ?")
		args = append(args, *update.Status)
	}

	if update.KYCLinkID != nil {
		fields = append(fields, "kyc_link_id = ?")
		args = append(args, *update.KYCLinkID)
	}

	if update.CustomerID != nil {
		fields = append(fields, "customer_id = ?")
		args = append(args, *update.CustomerID)
	}

	if update.OptedInBy != nil {
		fields = append(fields, "opted_in_by = ?")
		args = append(args, *update.OptedInBy)
	}

	if update.OptedInAt != nil {
		fields = append(fields, "opted_in_at = ?")
		args = append(args, *update.OptedInAt)
	}

	if update.VirtualAccountID != nil {
		fields = append(fields, "virtual_account_id = ?")
		args = append(args, *update.VirtualAccountID)
	}

	if update.VirtualAccountCreatedBy != nil {
		fields = append(fields, "virtual_account_created_by = ?")
		args = append(args, *update.VirtualAccountCreatedBy)
	}

	if update.VirtualAccountCreatedAt != nil {
		fields = append(fields, "virtual_account_created_at = ?")
		args = append(args, *update.VirtualAccountCreatedAt)
	}

	if update.ErrorMessage != nil {
		fields = append(fields, "error_message = ?")
		args = append(args, *update.ErrorMessage)
	}

	if len(fields) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	q := `
		UPDATE bridge_integration 
		SET %s
		RETURNING %s
	`
	q = m.dbConnectionPool.Rebind(fmt.Sprintf(q, strings.Join(fields, ", "), bridgeIntegrationColumns))

	var integration BridgeIntegration
	err := m.dbConnectionPool.GetContext(ctx, &integration, q, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("updating Bridge integration: %w", err)
	}

	return &integration, nil
}

// OtherTenantSchemaFor returns the tenant schema, other than excludeSchema, whose bridge_integration row names
// customerID. It returns ErrRecordNotFound when no other tenant holds it.
func (m *BridgeIntegrationModel) OtherTenantSchemaFor(ctx context.Context, sqlExec db.SQLExecuter, customerID, excludeSchema string) (string, error) {
	// Soft-deleted tenants still count, tenants whose migrations have not run yet are skipped.
	const schemasQuery = `
		SELECT table_schema
		FROM information_schema.tables
		WHERE table_name = 'bridge_integration'
			AND left(table_schema, 4) = 'sdp_'
			AND table_schema <> $1
		ORDER BY table_schema`

	var schemas []string
	if err := sqlExec.SelectContext(ctx, &schemas, schemasQuery, excludeSchema); err != nil {
		return "", fmt.Errorf("listing tenant schemas with a Bridge integration: %w", err)
	}
	if len(schemas) == 0 {
		return "", ErrRecordNotFound
	}

	selects := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		selects = append(selects, fmt.Sprintf("SELECT %s AS schema_name FROM %s.bridge_integration WHERE customer_id = $1",
			pq.QuoteLiteral(schema), pq.QuoteIdentifier(schema)))
	}
	query := strings.Join(selects, " UNION ALL ") + " LIMIT 1"

	var holder string
	if err := sqlExec.GetContext(ctx, &holder, query, customerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrRecordNotFound
		}
		return "", fmt.Errorf("looking up Bridge customer across tenant schemas: %w", err)
	}

	return holder, nil
}
