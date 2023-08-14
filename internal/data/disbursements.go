package data

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

type Disbursement struct {
	ID                string                    `json:"id" db:"id"`
	Name              string                    `json:"name" db:"name"`
	Country           *Country                  `json:"country,omitempty" db:"country"`
	Wallet            *Wallet                   `json:"wallet,omitempty" db:"wallet"`
	Asset             *Asset                    `json:"asset,omitempty" db:"asset"`
	Status            DisbursementStatus        `json:"status" db:"status"`
	VerificationField VerificationField         `json:"verification_field,omitempty" db:"verification_field"`
	StatusHistory     DisbursementStatusHistory `json:"status_history,omitempty" db:"status_history"`
	FileName          string                    `json:"file_name,omitempty" db:"file_name"`
	FileContent       []byte                    `json:"-" db:"file_content"`
	CreatedAt         time.Time                 `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at" db:"updated_at"`
	*DisbursementStats
}

type DisbursementStatusHistory []DisbursementStatusHistoryEntry

type DisbursementStats struct {
	TotalPayments      int    `json:"total_payments" db:"total_payments"`
	SuccessfulPayments int    `json:"total_payments_sent" db:"total_payments_sent"`
	FailedPayments     int    `json:"total_payments_failed" db:"total_payments_failed"`
	RemainingPayments  int    `json:"total_payments_remaining" db:"total_payments_remaining"`
	AmountDisbursed    string `json:"amount_disbursed" db:"amount_disbursed"`
	TotalAmount        string `json:"total_amount" db:"total_amount"`
	AverageAmount      string `json:"average_amount" db:"average_amount"`
}

type DisbursementUpdate struct {
	ID          string
	FileName    string
	FileContent []byte
}

type VerificationField string

const (
	VerificationFieldDateOfBirth VerificationField = "DATE_OF_BIRTH"
	VerificationFieldPin         VerificationField = "PIN"
	VerificationFieldNationalID  VerificationField = "NATIONAL_ID_NUMBER"
)

type DisbursementStatusHistoryEntry struct {
	UserID    string             `json:"user_id"`
	Status    DisbursementStatus `json:"status"`
	Timestamp time.Time          `json:"timestamp"`
}
type DisbursementModel struct {
	dbConnectionPool db.DBConnectionPool
}

var (
	DefaultDisbursementSortField = SortFieldCreatedAt
	DefaultDisbursementSortOrder = SortOrderDESC
	AllowedDisbursementFilters   = []FilterKey{FilterKeyStatus, FilterKeyCreatedAtAfter, FilterKeyCreatedAtBefore}
	AllowedDisbursementSorts     = []SortField{SortFieldName, SortFieldCreatedAt}
)

func (d *DisbursementModel) Insert(ctx context.Context, disbursement *Disbursement) (string, error) {
	const q = `
		INSERT INTO 
		    disbursements (name, status, status_history, wallet_id, asset_id, country_code)
		VALUES 
		    ($1, $2, $3, $4, $5, $6)
		RETURNING id
		    `
	var newId string
	err := d.dbConnectionPool.GetContext(ctx, &newId, q,
		disbursement.Name,
		disbursement.Status,
		disbursement.StatusHistory,
		disbursement.Wallet.ID,
		disbursement.Asset.ID,
		disbursement.Country.Code,
	)
	if err != nil {
		// check if the error is a duplicate key error
		if strings.Contains(err.Error(), "duplicate key") {
			return "", ErrRecordAlreadyExists
		}
		return "", fmt.Errorf("unable to create disbursement %s: %w", disbursement.Name, err)
	}

	return newId, nil
}

func (d *DisbursementModel) GetWithStatistics(ctx context.Context, id string) (*Disbursement, error) {
	disbursement, err := d.Get(ctx, d.dbConnectionPool, id)
	if err != nil {
		return nil, err
	}

	err = d.populateStatistics(ctx, []*Disbursement{disbursement})
	if err != nil {
		return nil, fmt.Errorf("error populating statistics for disbursement ID %s: %w", id, err)
	}

	return disbursement, nil
}

func (d *DisbursementModel) Get(ctx context.Context, sqlExec db.SQLExecuter, id string) (*Disbursement, error) {
	var disbursement Disbursement

	query := `
		SELECT
			d.id,
			d.name,
			d.status,
			d.status_history,
			d.verification_field,
			COALESCE(d.file_name, '') as file_name,
			d.file_content,
			d.created_at,
			d.updated_at,
			w.id as "wallet.id",
			w.name as "wallet.name",
			w.homepage as "wallet.homepage",
			w.sep_10_client_domain as "wallet.sep_10_client_domain",
			w.deep_link_schema as "wallet.deep_link_schema",
			w.created_at as "wallet.created_at",
			w.updated_at as "wallet.updated_at",
			a.id as "asset.id",
			a.code as "asset.code",
			a.issuer as "asset.issuer",
			a.created_at as "asset.created_at",
			a.updated_at as "asset.updated_at",
			c.code as "country.code",
			c.name as "country.name",
			c.created_at as "country.created_at",
			c.updated_at as "country.updated_at"
		FROM
			disbursements d
		JOIN wallets w on d.wallet_id = w.id
		JOIN assets a on d.asset_id = a.id
		JOIN countries c on d.country_code = c.code
		WHERE 
			d.id = $1
		`
	err := sqlExec.GetContext(ctx, &disbursement, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying disbursement ID %s: %w", id, err)
	}

	return &disbursement, nil
}

func (d *DisbursementModel) GetByName(ctx context.Context, sqlExec db.SQLExecuter, name string) (*Disbursement, error) {
	var disbursement Disbursement

	query := `
		SELECT
			d.id,
			d.name,
			d.status,
			d.status_history,
			d.verification_field,
			COALESCE(d.file_name, '') as file_name,
			d.file_content,
			d.created_at,
			d.updated_at,
			w.id as "wallet.id",
			w.name as "wallet.name",
			w.homepage as "wallet.homepage",
			w.sep_10_client_domain as "wallet.sep_10_client_domain",
			w.deep_link_schema as "wallet.deep_link_schema",
			w.created_at as "wallet.created_at",
			w.updated_at as "wallet.updated_at",
			a.id as "asset.id",
			a.code as "asset.code",
			a.issuer as "asset.issuer",
			a.created_at as "asset.created_at",
			a.updated_at as "asset.updated_at",
			c.code as "country.code",
			c.name as "country.name",
			c.created_at as "country.created_at",
			c.updated_at as "country.updated_at"
		FROM
			disbursements d
		JOIN wallets w on d.wallet_id = w.id
		JOIN assets a on d.asset_id = a.id
		JOIN countries c on d.country_code = c.code
		WHERE
			d.name = $1
		`
	err := sqlExec.GetContext(ctx, &disbursement, query, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying disbursement with name %s: %w", name, err)
	}

	return &disbursement, nil
}

// populateStatistics populates the payment statistics for the given disbursements
func (d *DisbursementModel) populateStatistics(ctx context.Context, disbursements []*Disbursement) error {
	if len(disbursements) == 0 {
		return nil
	}

	disbursementIDs := make([]string, len(disbursements))
	for i, disbursement := range disbursements {
		disbursementIDs[i] = disbursement.ID
	}

	query := `
		SELECT
			disbursement_id,
			count(*) as total_payments,
			sum(case when status = 'SUCCESS' then 1 else 0 end) as total_payments_sent,
			sum(case when status = 'FAILED' then 1 else 0 end) as total_payments_failed,
			sum(case when status IN ('DRAFT', 'READY', 'PENDING', 'PAUSED')  then 1 else 0 end) as total_payments_remaining,
			ROUND(SUM(CASE WHEN status = 'SUCCESS' THEN amount ELSE 0 END), 2) as amount_disbursed,
			ROUND(SUM(amount), 2) as total_amount,
			ROUND(avg(amount), 2) as average_amount
		FROM
			payments
		WHERE
			disbursement_id = ANY ($1)
		GROUP BY
			disbursement_id;
			`

	rows, err := d.dbConnectionPool.QueryxContext(ctx, query, pq.Array(disbursementIDs))
	if err != nil {
		return fmt.Errorf("error querying disbursement statistics: %w", err)
	}
	defer db.CloseRows(ctx, rows)

	statistics := make(map[string]*DisbursementStats)
	for rows.Next() {
		var disbursementID string
		var stats DisbursementStats
		err := rows.Scan(
			&disbursementID,
			&stats.TotalPayments,
			&stats.SuccessfulPayments,
			&stats.FailedPayments,
			&stats.RemainingPayments,
			&stats.AmountDisbursed,
			&stats.TotalAmount,
			&stats.AverageAmount,
		)
		if err != nil {
			return fmt.Errorf("error scanning disbursement statistics: %w", err)
		}
		statistics[disbursementID] = &stats
	}

	if len(statistics) == 0 {
		return nil
	}

	// populate the statistics
	for _, disbursement := range disbursements {
		disbursement.DisbursementStats = statistics[disbursement.ID]
	}
	return nil
}

// Count returns the number of disbursements matching the given query parameters.
func (d *DisbursementModel) Count(ctx context.Context, sqlExec db.SQLExecuter, queryParams *QueryParams) (int, error) {
	var count int
	baseQuery := `
		SELECT
			count(*)
		FROM
			disbursements d
		JOIN wallets w on d.wallet_id = w.id
		JOIN assets a on d.asset_id = a.id
		JOIN countries c on d.country_code = c.code
		`

	query, params := d.newDisbursementQuery(baseQuery, queryParams, false)

	err := sqlExec.GetContext(ctx, &count, query, params...)
	if err != nil {
		return 0, fmt.Errorf("error counting disbursements: %w", err)
	}
	return count, nil
}

// GetAll returns all disbursements matching the given query parameters.
func (d *DisbursementModel) GetAll(ctx context.Context, sqlExec db.SQLExecuter, queryParams *QueryParams) ([]*Disbursement, error) {
	disbursements := []*Disbursement{}

	baseQuery := `
		SELECT
			d.id,
			d.name,
			d.status,
			d.status_history,
			d.verification_field,
			d.created_at,
			d.updated_at,
			COALESCE(d.file_name, '') as file_name,
			w.id as "wallet.id",
			w.name as "wallet.name",
			w.homepage as "wallet.homepage",
			w.sep_10_client_domain as "wallet.sep_10_client_domain",
			w.deep_link_schema as "wallet.deep_link_schema",
			w.created_at as "wallet.created_at",
			w.updated_at as "wallet.updated_at",
			a.id as "asset.id",
			a.code as "asset.code",
			a.issuer as "asset.issuer",
			a.created_at as "asset.created_at",
			a.updated_at as "asset.updated_at",
			c.code as "country.code",
			c.name as "country.name",
			c.created_at as "country.created_at",
			c.updated_at as "country.updated_at"
		FROM
			disbursements d
		JOIN wallets w on d.wallet_id = w.id
		JOIN assets a on d.asset_id = a.id
		JOIN countries c on d.country_code = c.code
		`

	query, params := d.newDisbursementQuery(baseQuery, queryParams, true)
	err := sqlExec.SelectContext(ctx, &disbursements, query, params...)
	if err != nil {
		return nil, fmt.Errorf("error querying disbursements: %w", err)
	}

	// populate the statistics
	if err = d.populateStatistics(ctx, disbursements); err != nil {
		return nil, fmt.Errorf("error populating disbursement statistics: %w", err)
	}
	return disbursements, nil
}

// UpdateStatus updates the status of the given disbursement.
func (d *DisbursementModel) UpdateStatus(ctx context.Context, sqlExec db.SQLExecuter, userID string, disbursementID string, targetStatus DisbursementStatus) error {
	sourceStatuses := targetStatus.SourceStatuses()

	query := `
		UPDATE
			disbursements
		SET
			status = $1,
			status_history = array_append(status_history, create_disbursement_status_history(NOW(), $1, $2))
		WHERE
			id = $3 AND status = ANY($4)
		`
	result, err := sqlExec.ExecContext(ctx, query, targetStatus, userID, disbursementID, pq.Array(sourceStatuses))
	if err != nil {
		return fmt.Errorf("error updating disbursement status: %w", err)
	}
	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return fmt.Errorf("disbursement %s status was not updated from %s to %s", disbursementID, sourceStatuses, targetStatus)
	} else if numRowsAffected == 1 {
		log.Ctx(ctx).Infof("Set disbursement %s status from %s to %s", disbursementID, sourceStatuses, targetStatus)
	} else {
		return fmt.Errorf("unexpected number of rows affected: %d when updating disbursement %s status from %s to %s",
			numRowsAffected,
			disbursementID,
			sourceStatuses,
			targetStatus)
	}

	return nil
}

// newDisbursementQuery generates the full query and parameters for a disbursement search query
func (d *DisbursementModel) newDisbursementQuery(baseQuery string, queryParams *QueryParams, paginated bool) (string, []interface{}) {
	qb := NewQueryBuilder(baseQuery)
	if queryParams.Query != "" {
		qb.AddCondition("d.name ILIKE ?", "%"+queryParams.Query+"%")
	}

	if statusSlice, ok := queryParams.Filters[FilterKeyStatus].([]DisbursementStatus); ok && len(statusSlice) > 0 {
		qb.AddCondition("d.status = ANY(?)", pq.Array(statusSlice))
	}
	if queryParams.Filters[FilterKeyCreatedAtAfter] != nil {
		qb.AddCondition("d.created_at >= ?", queryParams.Filters[FilterKeyCreatedAtAfter])
	}
	if queryParams.Filters[FilterKeyCreatedAtBefore] != nil {
		qb.AddCondition("d.created_at <= ?", queryParams.Filters[FilterKeyCreatedAtBefore])
	}
	if paginated {
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "d")
		qb.AddPagination(queryParams.Page, queryParams.PageLimit)
	}
	query, params := qb.Build()
	return d.dbConnectionPool.Rebind(query), params
}

func (du *DisbursementUpdate) Validate() error {
	if du.FileName == "" {
		return errors.New("file name is required")
	}
	if len(du.FileContent) == 0 {
		return errors.New("file content is required")
	}
	if du.ID == "" {
		return errors.New("disbursement ID is required")
	}
	return nil
}

func (d *DisbursementModel) Update(ctx context.Context, du *DisbursementUpdate) error {
	if err := du.Validate(); err != nil {
		return fmt.Errorf("error validating disbursement update: %w", err)
	}

	query := `
		UPDATE
			disbursements
		SET
			file_name = $1,
			file_content = $2
		WHERE
			id = $3
		`
	result, err := d.dbConnectionPool.ExecContext(ctx, query, du.FileName, du.FileContent, du.ID)
	if err != nil {
		return fmt.Errorf("error updating disbursement: %w", err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}
	if numRowsAffected != 1 {
		return fmt.Errorf("disbursement %s was not updated", du.ID)
	}

	return nil
}

// Value implements the driver.Valuer interface.
func (dsh DisbursementStatusHistory) Value() (driver.Value, error) {
	var statusHistoryJSON []string
	for _, sh := range dsh {
		shJSONBytes, err := json.Marshal(sh)
		if err != nil {
			return nil, fmt.Errorf("error converting status history to json for disbursement: %w", err)
		}
		statusHistoryJSON = append(statusHistoryJSON, string(shJSONBytes))
	}

	return pq.Array(statusHistoryJSON).Value()
}

// Scan implements the sql.Scanner interface.
func (dsh *DisbursementStatusHistory) Scan(src interface{}) error {
	var statusHistoryJSON []string
	if err := pq.Array(&statusHistoryJSON).Scan(src); err != nil {
		return fmt.Errorf("error scanning status history value: %w", err)
	}

	for _, sh := range statusHistoryJSON {
		var shEntry DisbursementStatusHistoryEntry
		err := json.Unmarshal([]byte(sh), &shEntry)
		if err != nil {
			return fmt.Errorf("error unmarshaling status_history column: %w", err)
		}
		*dsh = append(*dsh, shEntry)
	}

	return nil
}

// CompleteDisbursements sets disbursements statuses to complete after all payments are processed and successfully sent.
func (d *DisbursementModel) CompleteDisbursements(ctx context.Context, sqlExec db.SQLExecuter, disbursementIDs []string) error {
	query := `
		WITH incompleted_disbursements AS (
			SELECT
				p.disbursement_id,
				COUNT(p.*)
			FROM
				payments p
				INNER JOIN disbursements d ON d.id = p.disbursement_id
			WHERE
				p.status != $4
				AND d.status = $3
				AND d.id = ANY($2)
			GROUP BY
				p.status,
				p.disbursement_id
			HAVING
				COUNT(p.*) > 0
		) 
		UPDATE
			disbursements
		SET
			status = $1,
			status_history = array_append(status_history, create_disbursement_status_history(NOW(), $1, ''))
		WHERE
			id = ANY($2) 
			AND status = $3 
			AND id NOT IN (SELECT disbursement_id FROM incompleted_disbursements)
	`

	_, err := sqlExec.ExecContext(ctx, query, CompletedDisbursementStatus, pq.Array(disbursementIDs), StartedDisbursementStatus, SuccessPaymentStatus)
	if err != nil {
		return fmt.Errorf("error completing disbursement: %w", err)
	}

	return nil
}
