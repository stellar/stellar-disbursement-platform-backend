package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type Receiver struct {
	ID          string     `json:"id" db:"id"`
	Email       string     `json:"email,omitempty" db:"email"`
	PhoneNumber string     `json:"phone_number,omitempty" db:"phone_number"`
	ExternalID  string     `json:"external_id,omitempty" db:"external_id"`
	CreatedAt   *time.Time `json:"created_at,omitempty" db:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty" db:"updated_at"`
	ReceiverStats
}

type ReceiverContactType string

const (
	ReceiverContactTypeEmail ReceiverContactType = "EMAIL"
	ReceiverContactTypeSMS   ReceiverContactType = "PHONE_NUMBER"
)

func (r Receiver) ContactByType(contactType ReceiverContactType) string {
	switch contactType {
	case ReceiverContactTypeEmail:
		return r.Email
	case ReceiverContactTypeSMS:
		return r.PhoneNumber
	default:
		return ""
	}
}

func GetAllReceiverContactTypes() []ReceiverContactType {
	return []ReceiverContactType{ReceiverContactTypeEmail, ReceiverContactTypeSMS}
}

type ReceiverRegistrationRequest struct {
	Email             string           `json:"email"`
	PhoneNumber       string           `json:"phone_number"`
	OTP               string           `json:"otp"`
	VerificationValue string           `json:"verification"`
	VerificationField VerificationType `json:"verification_field"`
	ReCAPTCHAToken    string           `json:"recaptcha_token"`
}

func ReceiverColumnNames(tableReference, resultAlias string) string {
	columns := GenerateColumnNames(SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		Columns: []string{
			"id",
			"external_id",
			"created_at",
			"updated_at",
		},
	})

	columns = append(columns, GenerateColumnNames(SQLColumnConfig{
		TableReference:        tableReference,
		ResultAlias:           resultAlias,
		CoalesceToEmptyString: true,
		Columns: []string{
			"phone_number",
			"email",
		},
	})...)

	return strings.Join(columns, ",\n")
}

type ReceiverStats struct {
	TotalPayments      string          `json:"total_payments,omitempty" db:"total_payments"`
	SuccessfulPayments string          `json:"successful_payments,omitempty" db:"successful_payments"`
	FailedPayments     string          `json:"failed_payments,omitempty" db:"failed_payments"`
	CanceledPayments   string          `json:"canceled_payments,omitempty" db:"canceled_payments"`
	RemainingPayments  string          `json:"remaining_payments,omitempty" db:"remaining_payments"`
	RegisteredWallets  string          `json:"registered_wallets,omitempty" db:"registered_wallets"`
	ReceivedAmounts    ReceivedAmounts `json:"received_amounts,omitempty" db:"received_amounts"`
}

type Amount struct {
	AssetCode      string `json:"asset_code" db:"asset_code"`
	AssetIssuer    string `json:"asset_issuer" db:"asset_issuer"`
	ReceivedAmount string `json:"received_amount" db:"received_amount"`
}

var (
	DefaultReceiverSortField = SortFieldUpdatedAt
	DefaultReceiverSortOrder = SortOrderDESC
	AllowedReceiverFilters   = []FilterKey{FilterKeyStatus, FilterKeyCreatedAtAfter, FilterKeyCreatedAtBefore}
	AllowedReceiverSorts     = []SortField{SortFieldCreatedAt, SortFieldUpdatedAt}
)

type ReceiverModel struct{}

type ReceiverInsert struct {
	PhoneNumber *string `db:"phone_number"`
	Email       *string `db:"email"`
	ExternalId  *string `db:"external_id"`
}

type ReceiverUpdate ReceiverInsert

func (ru ReceiverUpdate) IsEmpty() bool {
	return ru.Email == nil && ru.ExternalId == nil && ru.PhoneNumber == nil
}

func (ru ReceiverUpdate) Validate() error {
	if ru.IsEmpty() {
		return fmt.Errorf("no values provided to update receiver")
	}

	if ru.Email != nil {
		if err := utils.ValidateEmail(*ru.Email); err != nil {
			return fmt.Errorf("validating email: %w", err)
		}
	}

	if ru.PhoneNumber != nil {
		if err := utils.ValidatePhoneNumber(*ru.PhoneNumber); err != nil {
			return fmt.Errorf("validating phone number: %w", err)
		}
	}

	return nil
}

type ReceivedAmounts []Amount

// Scan implements the sql.Scanner interface.
func (ra *ReceivedAmounts) Scan(src interface{}) error {
	var receivedAmounts sql.NullString
	if err := (&receivedAmounts).Scan(src); err != nil {
		return fmt.Errorf("scanning status history value: %w", err)
	}

	if receivedAmounts.Valid {
		var shEntry []Amount
		err := json.Unmarshal([]byte(receivedAmounts.String), &shEntry)
		if err != nil {
			return fmt.Errorf("unmarshaling status_history column: %w", err)
		}

		*ra = shEntry
	}

	return nil
}

// Get returns a RECEIVER matching the given ID.
func (r *ReceiverModel) Get(ctx context.Context, sqlExec db.SQLExecuter, id string) (*Receiver, error) {
	receiver := Receiver{}

	query := `
	WITH receivers_cte AS (
		SELECT
			*
		FROM receivers r
		WHERE r.id = $1
	), receiver_wallets_cte AS (
		SELECT
			rc.id as receiver_id,
			COUNT(rw) FILTER(WHERE rw.status = 'REGISTERED') as registered_wallets
		FROM receivers_cte rc
		JOIN receiver_wallets rw ON rc.id = rw.receiver_id
		GROUP BY rc.id
	),  receiver_stats AS (
		SELECT
			rc.id as receiver_id,
			COUNT(p) as total_payments,
			COUNT(p) FILTER(WHERE p.status = 'SUCCESS') as successful_payments,
			COUNT(p) FILTER(WHERE p.status = 'FAILED') as failed_payments,
			COUNT(p) FILTER(WHERE p.status = 'CANCELED') as canceled_payments,
			COUNT(p) FILTER(WHERE p.status IN ('DRAFT', 'READY', 'PENDING', 'PAUSED')) as remaining_payments,
			a.code as asset_code,
			a.issuer as asset_issuer,
			COALESCE(SUM(p.amount) FILTER(WHERE p.asset_id = a.id AND p.status = 'SUCCESS'), '0') as received_amount
		FROM receivers_cte rc
		JOIN payments p ON rc.id = p.receiver_id
		JOIN disbursements d ON p.disbursement_id = d.id
		JOIN assets a ON a.id = p.asset_id
		GROUP BY (rc.id, a.code, a.issuer)
	), receiver_stats_aggregate AS (
		SELECT
			rs.receiver_id,
			SUM(rs.total_payments) as total_payments,
			SUM(rs.successful_payments) as successful_payments,
			SUM(rs.failed_payments) as failed_payments,
			SUM(rs.canceled_payments) as canceled_payments,
			SUM(rs.remaining_payments) as remaining_payments,
			jsonb_agg(jsonb_build_object('asset_code', rs.asset_code, 'asset_issuer', rs.asset_issuer, 'received_amount', rs.received_amount::text)) as received_amounts
		FROM receiver_stats rs
		GROUP BY (rs.receiver_id)
	)
	SELECT
		` + ReceiverColumnNames("rc", "") + `,
		COALESCE(total_payments, 0) as total_payments,
		COALESCE(successful_payments, 0) as successful_payments,
		COALESCE(rs.failed_payments, '0') as failed_payments,
		COALESCE(rs.canceled_payments, '0') as canceled_payments,
		COALESCE(rs.remaining_payments, '0') as remaining_payments,
		rs.received_amounts,
		COALESCE(rw.registered_wallets, 0) as registered_wallets
	FROM receivers_cte rc
	LEFT JOIN receiver_stats_aggregate rs ON rs.receiver_id = rc.id
	LEFT JOIN receiver_wallets_cte rw ON rw.receiver_id = rc.id
	`

	err := sqlExec.GetContext(ctx, &receiver, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		} else {
			return nil, fmt.Errorf("querying receiver ID: %w", err)
		}
	}

	return &receiver, nil
}

// Count returns the number of receivers matching the given query parameters.
func (r *ReceiverModel) Count(ctx context.Context, sqlExec db.SQLExecuter, queryParams *QueryParams) (int, error) {
	var count int
	baseQuery := `
		SELECT
			COUNT(DISTINCT r.id)
		FROM receivers r
		LEFT JOIN receiver_wallets rw ON rw.receiver_id = r.id
	`
	query, params := newReceiverQuery(baseQuery, queryParams, sqlExec, QueryTypeSingle)

	err := sqlExec.GetContext(ctx, &count, query, params...)
	if err != nil {
		return 0, fmt.Errorf("counting payments: %w", err)
	}

	return count, nil
}

// GetAll returns all RECEIVERS matching the given query parameters.
func (r *ReceiverModel) GetAll(ctx context.Context, sqlExec db.SQLExecuter, queryParams *QueryParams, queryType QueryType) ([]Receiver, error) {
	receivers := []Receiver{}

	query := `
		WITH receivers_cte AS (
			SELECT
				` + ReceiverColumnNames("r", "") + `
			FROM
				receivers r
		), registered_receiver_wallets_count_cte AS (
			SELECT
				rc.id as receiver_id,
				COUNT(rw) FILTER(WHERE rw.status = 'REGISTERED') as registered_wallets
			FROM receivers_cte rc
			JOIN receiver_wallets rw ON rc.id = rw.receiver_id
			GROUP BY rc.id
		), receiver_stats AS (
			SELECT
				rc.id as receiver_id,
				COUNT(p) as total_payments,
				COUNT(p) FILTER(WHERE p.status = 'SUCCESS') as successful_payments,
				COUNT(p) FILTER(WHERE p.status = 'FAILED') as failed_payments,
				COUNT(p) FILTER(WHERE p.status = 'CANCELED') as canceled_payments,
				COUNT(p) FILTER(WHERE p.status IN ('DRAFT', 'READY', 'PENDING', 'PAUSED')) as remaining_payments,
				a.code as asset_code,
				a.issuer as asset_issuer,
				COALESCE(SUM(p.amount) FILTER(WHERE p.asset_id = a.id AND p.status = 'SUCCESS'), '0') as received_amount
			FROM receivers_cte rc
			JOIN payments p ON rc.id = p.receiver_id
			JOIN disbursements d ON p.disbursement_id = d.id
			JOIN assets a ON a.id = p.asset_id
			GROUP BY (rc.id, a.code, a.issuer)
		), receiver_stats_aggregate AS (
			SELECT
				rs.receiver_id,
				SUM(rs.total_payments) as total_payments,
				SUM(rs.successful_payments) as successful_payments,
				SUM(rs.failed_payments) as failed_payments,
				SUM(rs.canceled_payments) as canceled_payments,
				SUM(rs.remaining_payments) as remaining_payments,
				jsonb_agg(jsonb_build_object('asset_code', rs.asset_code, 'asset_issuer', rs.asset_issuer, 'received_amount', rs.received_amount::text)) as received_amounts
			FROM receiver_stats rs
			GROUP BY (rs.receiver_id)
		)
		SELECT
			distinct(r.id),
			r.external_id,
			COALESCE(r.email, '') as email,
			COALESCE(r.phone_number, '') as phone_number,
			r.created_at,
			r.updated_at,
			COALESCE(total_payments, 0) as total_payments,
			COALESCE(successful_payments, 0) as successful_payments,
			COALESCE(rs.failed_payments, '0') as failed_payments,
			COALESCE(rs.canceled_payments, '0') as canceled_payments,
			COALESCE(rs.remaining_payments, '0') as remaining_payments,
			rs.received_amounts,
			COALESCE(rrwc.registered_wallets, 0) as registered_wallets
		FROM receivers_cte r
		LEFT JOIN receiver_stats_aggregate rs ON rs.receiver_id = r.id
		LEFT JOIN receiver_wallets rw ON rw.receiver_id = r.id
		LEFT JOIN registered_receiver_wallets_count_cte rrwc ON rrwc.receiver_id = r.id
		`

	query, params := newReceiverQuery(query, queryParams, sqlExec, queryType)

	err := sqlExec.SelectContext(ctx, &receivers, query, params...)
	if err != nil {
		return nil, fmt.Errorf("querying receivers: %w", err)
	}

	return receivers, nil
}

// newReceiverQuery generates the full query and parameters for a receiver search query
func newReceiverQuery(baseQuery string, queryParams *QueryParams, sqlExec db.SQLExecuter, queryType QueryType) (string, []interface{}) {
	qb := NewQueryBuilder(baseQuery)
	if queryParams.Query != "" {
		q := "%" + queryParams.Query + "%"
		qb.AddCondition("(r.id ILIKE ? OR r.phone_number ILIKE ? OR r.email ILIKE ?)", q, q, q)
	}
	if queryParams.Filters[FilterKeyIDs] != nil {
		ids := queryParams.Filters[FilterKeyIDs].([]string)
		qb.AddCondition("r.id = ANY(?)", pq.Array(ids))
	}
	if queryParams.Filters[FilterKeyStatus] != nil {
		status := queryParams.Filters[FilterKeyStatus].(ReceiversWalletStatus)
		qb.AddCondition("rw.status = ?", status)
	}
	if queryParams.Filters[FilterKeyCreatedAtAfter] != nil {
		qb.AddCondition("r.created_at >= ?", queryParams.Filters[FilterKeyCreatedAtAfter])
	}
	if queryParams.Filters[FilterKeyCreatedAtBefore] != nil {
		qb.AddCondition("r.created_at <= ?", queryParams.Filters[FilterKeyCreatedAtBefore])
	}

	switch queryType {
	case QueryTypeSelectPaginated:
		qb.AddPagination(queryParams.Page, queryParams.PageLimit)
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "r")
	case QueryTypeSelectAll:
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "r")
	case QueryTypeSingle:
		// no need to sort or paginate.
	}

	query, params := qb.Build()
	return sqlExec.Rebind(query), params
}

type ReceiverIDs []string

// ParseReceiverIDs return the array of receivers IDs.
func (r *ReceiverModel) ParseReceiverIDs(receivers []Receiver) ReceiverIDs {
	receiverIds := make(ReceiverIDs, 0)

	for _, receiver := range receivers {
		receiverIds = append(receiverIds, receiver.ID)
	}

	return receiverIds
}

// Insert inserts a new receiver into the database.
func (r *ReceiverModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, insert ReceiverInsert) (*Receiver, error) {
	query := `
		INSERT INTO receivers (
			phone_number,
			email,
			external_id
		) VALUES (
			$1,
			$2,
		    $3
		) RETURNING
			` + ReceiverColumnNames("", "")

	var receiver Receiver
	err := sqlExec.GetContext(ctx, &receiver, query, insert.PhoneNumber, insert.Email, insert.ExternalId)
	if err != nil {
		return nil, fmt.Errorf("inserting receiver: %w", err)
	}

	return &receiver, nil
}

// Update updates the receiver Email and/or External ID.
func (r *ReceiverModel) Update(ctx context.Context, sqlExec db.SQLExecuter, ID string, receiverUpdate ReceiverUpdate) error {
	if err := receiverUpdate.Validate(); err != nil {
		return fmt.Errorf("validating receiver update: %w", err)
	}

	args := []interface{}{}
	fields := []string{}

	if receiverUpdate.PhoneNumber != nil {
		phoneNumber := *receiverUpdate.PhoneNumber
		fields = append(fields, "phone_number = ?")
		args = append(args, phoneNumber)
	}

	if receiverUpdate.Email != nil {
		email := *receiverUpdate.Email
		fields = append(fields, "email = ?")
		args = append(args, email)
	}

	if receiverUpdate.ExternalId != nil {
		externalID := *receiverUpdate.ExternalId
		fields = append(fields, "external_id = ?")
		args = append(args, externalID)
	}

	args = append(args, ID)

	query := `
		UPDATE
			receivers
		SET
			%s
		WHERE
			id = ?
	`

	query = sqlExec.Rebind(fmt.Sprintf(query, strings.Join(fields, ", ")))

	_, err := sqlExec.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating receiver: %w", err)
	}

	return nil
}

// GetByContacts search for receivers by phone numbers and email.
func (r *ReceiverModel) GetByContacts(ctx context.Context, sqlExec db.SQLExecuter, contacts ...string) ([]*Receiver, error) {
	receivers := []*Receiver{}

	if len(contacts) == 0 {
		return receivers, nil
	}

	query := `
	SELECT
		` + ReceiverColumnNames("r", "") + `
	FROM receivers r
	WHERE r.phone_number = ANY($1) OR r.email = ANY($1)
	`
	err := sqlExec.SelectContext(ctx, &receivers, query, pq.Array(contacts))
	if err != nil {
		return nil, fmt.Errorf("fetching receivers by phone numbers or email: %w", err)
	}
	return receivers, nil
}

// DeleteByContactInfo deletes a receiver by phone number or email. It also deletes the associated entries in other
// tables: messages, payments, receiver_verifications, receiver_wallets, receivers, disbursements,
// submitter_transactions.
func (r *ReceiverModel) DeleteByContactInfo(ctx context.Context, dbConnectionPool db.DBConnectionPool, contactInfo string) error {
	return db.RunInTransaction(ctx, dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
		query := "SELECT id FROM receivers WHERE phone_number = $1 OR email = $1"
		var receiverID string

		err := dbTx.GetContext(ctx, &receiverID, query, contactInfo)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRecordNotFound
			}
			return fmt.Errorf("fetching receiver by contact info %s: %w", contactInfo, err)
		}

		type QueryWithParams struct {
			Query  string
			Params []interface{}
		}

		queries := []QueryWithParams{
			{"DELETE FROM messages WHERE receiver_id = $1", []interface{}{receiverID}},
			{"DELETE FROM receiver_verifications WHERE receiver_id = $1", []interface{}{receiverID}},
			{"DELETE FROM circle_transfer_requests WHERE payment_id IN (SELECT id FROM payments WHERE receiver_id = $1)", []interface{}{receiverID}},
			{"DELETE FROM circle_recipients WHERE receiver_wallet_id IN (SELECT id FROM receiver_wallets WHERE receiver_id = $1)", []interface{}{receiverID}},
			{"DELETE FROM payments WHERE receiver_id = $1", []interface{}{receiverID}},
			{"DELETE FROM receiver_wallets WHERE receiver_id = $1", []interface{}{receiverID}},
			{"DELETE FROM receivers WHERE id = $1", []interface{}{receiverID}},
			{"DELETE FROM disbursements WHERE id NOT IN (SELECT DISTINCT disbursement_id FROM payments)", nil},
		}

		for _, qwp := range queries {
			_, err = dbTx.ExecContext(ctx, qwp.Query, qwp.Params...)
			if err != nil {
				return fmt.Errorf("executing query %q: %w", qwp.Query, err)
			}
		}

		return nil
	})
}
