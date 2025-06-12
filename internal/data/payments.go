package data

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type Payment struct {
	ID                   string `json:"id" db:"id"`
	Amount               string `json:"amount" db:"amount"`
	StellarTransactionID string `json:"stellar_transaction_id" db:"stellar_transaction_id"`
	// TODO: evaluate if we will keep or remove StellarOperationID
	StellarOperationID      string               `json:"stellar_operation_id" db:"stellar_operation_id"`
	Status                  PaymentStatus        `json:"status" db:"status"`
	StatusHistory           PaymentStatusHistory `json:"status_history,omitempty" db:"status_history"`
	PaymentType             PaymentType          `json:"payment_type" db:"payment_type"`
	Disbursement            *Disbursement        `json:"disbursement,omitempty" db:"disbursement"`
	Asset                   Asset                `json:"asset"`
	ReceiverWallet          *ReceiverWallet      `json:"receiver_wallet,omitempty" db:"receiver_wallet"`
	CreatedAt               time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time            `json:"updated_at" db:"updated_at"`
	ExternalPaymentID       string               `json:"external_payment_id,omitempty" db:"external_payment_id"`
	CircleTransferRequestID *string              `json:"circle_transfer_request_id,omitempty"`
}

type PaymentType string

const (
	PaymentTypeDisbursement PaymentType = "DISBURSEMENT"
	PaymentTypeDirect       PaymentType = "DIRECT"
)

func (pt PaymentType) Validate() error {
	switch pt {
	case PaymentTypeDisbursement, PaymentTypeDirect:
		return nil
	default:
		return fmt.Errorf("invalid payment type: %s", pt)
	}
}

type PaymentStatusHistoryEntry struct {
	Status        PaymentStatus `json:"status"`
	StatusMessage string        `json:"status_message"`
	Timestamp     time.Time     `json:"timestamp"`
}

type PaymentModel struct {
	dbConnectionPool db.DBConnectionPool
}

var (
	DefaultPaymentSortField = SortFieldUpdatedAt
	DefaultPaymentSortOrder = SortOrderDESC
	AllowedPaymentFilters   = []FilterKey{FilterKeyStatus, FilterKeyCreatedAtAfter, FilterKeyCreatedAtBefore, FilterKeyReceiverID, FilterKeyPaymentType}
	AllowedPaymentSorts     = []SortField{SortFieldCreatedAt, SortFieldUpdatedAt}
)

type PaymentInsert struct {
	ReceiverID        string      `db:"receiver_id"`
	DisbursementID    *string     `db:"disbursement_id"`
	PaymentType       PaymentType `db:"payment_type"`
	Amount            string      `db:"amount"`
	AssetID           string      `db:"asset_id"`
	ReceiverWalletID  string      `db:"receiver_wallet_id"`
	ExternalPaymentID *string     `db:"external_payment_id"`
}

type PaymentUpdate struct {
	Status               PaymentStatus `db:"status"`
	StatusMessage        string
	StellarTransactionID string `db:"stellar_transaction_id"`
}

type PaymentStatusHistory []PaymentStatusHistoryEntry

// Value implements the driver.Valuer interface.
func (psh PaymentStatusHistory) Value() (driver.Value, error) {
	var statusHistoryJSON []string
	for _, sh := range psh {
		shJSONBytes, err := json.Marshal(sh)
		if err != nil {
			return nil, fmt.Errorf("error converting status history to json for message: %w", err)
		}
		statusHistoryJSON = append(statusHistoryJSON, string(shJSONBytes))
	}

	return pq.Array(statusHistoryJSON).Value()
}

var _ driver.Valuer = (*PaymentStatusHistory)(nil)

// Scan implements the sql.Scanner interface.
func (psh *PaymentStatusHistory) Scan(src interface{}) error {
	var statusHistoryJSON []string
	if err := pq.Array(&statusHistoryJSON).Scan(src); err != nil {
		return fmt.Errorf("error scanning status history value: %w", err)
	}

	for _, sh := range statusHistoryJSON {
		var shEntry PaymentStatusHistoryEntry
		err := json.Unmarshal([]byte(sh), &shEntry)
		if err != nil {
			return fmt.Errorf("error unmarshaling status_history column: %w", err)
		}
		*psh = append(*psh, shEntry)
	}

	return nil
}

var _ sql.Scanner = (*PaymentStatusHistory)(nil)

func (p *PaymentInsert) Validate() error {
	if strings.TrimSpace(p.ReceiverID) == "" {
		return fmt.Errorf("receiver_id is required")
	}

	if err := p.PaymentType.Validate(); err != nil {
		return fmt.Errorf("payment_type is invalid: %w", err)
	}

	if p.PaymentType == PaymentTypeDisbursement && (p.DisbursementID == nil || *p.DisbursementID == "") {
		return fmt.Errorf("disbursement_id is required for disbursement payments")
	}

	if p.PaymentType == PaymentTypeDirect && p.DisbursementID != nil {
		return fmt.Errorf("disbursement_id must be null for direct payments")
	}

	if err := utils.ValidateAmount(p.Amount); err != nil {
		return fmt.Errorf("amount is invalid: %w", err)
	}

	if strings.TrimSpace(p.AssetID) == "" {
		return fmt.Errorf("asset_id is required")
	}

	if strings.TrimSpace(p.ReceiverWalletID) == "" {
		return fmt.Errorf("receiver_wallet_id is required")
	}

	return nil
}

func (p *PaymentUpdate) Validate() error {
	if err := p.Status.Validate(); err != nil {
		return fmt.Errorf("status is invalid: %w", err)
	}
	if strings.TrimSpace(p.StellarTransactionID) == "" {
		return fmt.Errorf("stellar transaction id is required")
	}

	return nil
}

func PaymentColumnNames(tableReference, resultAlias string) string {
	columns := SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		RawColumns: []string{
			"id",
			"amount",
			"status",
			"status_history",
			"payment_type",
			"created_at",
			"updated_at",
		},
		CoalesceColumns: []string{
			"stellar_transaction_id",
			"stellar_operation_id",
			"external_payment_id",
		},
	}.Build()

	return strings.Join(columns, ",\n")
}

var basePaymentQuery = `
SELECT
    ` + PaymentColumnNames("p", "") + `,
    ` + DisbursementColumnNames("d", "disbursement") + `,
    ` + AssetColumnNames("a", "asset", false) + `,
    ` + ReceiverWalletColumnNames("rw", "receiver_wallet") + `,
    ` + ReceiverColumnNames("r", "receiver_wallet.receiver") + `,
    ` + WalletColumnNames("w", "receiver_wallet.wallet", false) + `
FROM
    payments p
    LEFT JOIN disbursements d ON p.disbursement_id = d.id
    JOIN assets a ON p.asset_id = a.id
    JOIN receiver_wallets rw ON rw.id = p.receiver_wallet_id
    JOIN receivers r ON r.id = rw.receiver_id
    JOIN wallets w ON w.id = rw.wallet_id
`

func (p *PaymentModel) GetAllReadyToPatchCompletionAnchorTransactions(ctx context.Context, sqlExec db.SQLExecuter) ([]Payment, error) {
	query := basePaymentQuery + `
		WHERE
			p.status = ANY($1) -- ARRAY['SUCCESS', 'FAILURE']::payment_status[]
			AND rw.status = $2 -- 'REGISTERED'::receiver_wallet_status
			AND rw.anchor_platform_transaction_id IS NOT NULL
			AND rw.anchor_platform_transaction_synced_at IS NULL
		ORDER BY
			p.created_at
		FOR UPDATE OF p, rw SKIP LOCKED
	`

	payments := make([]Payment, 0)
	err := sqlExec.SelectContext(ctx, &payments, query, pq.Array([]PaymentStatus{SuccessPaymentStatus, FailedPaymentStatus}), RegisteredReceiversWalletStatus)
	if err != nil {
		return nil, fmt.Errorf("getting payments: %w", err)
	}

	return payments, nil
}

func (p *PaymentModel) Get(ctx context.Context, id string, sqlExec db.SQLExecuter) (*Payment, error) {
	queryParams := &QueryParams{
		Filters: map[FilterKey]interface{}{
			FilterKeyID: id,
		},
	}
	payments, err := p.GetAll(ctx, queryParams, sqlExec, QueryTypeSingle)
	if err != nil {
		return nil, fmt.Errorf("getting payment by ID %s: %w", id, err)
	}

	if len(payments) == 0 {
		return nil, ErrRecordNotFound
	}

	return &payments[0], nil
}

func (p *PaymentModel) GetBatchForUpdate(ctx context.Context, sqlExec db.SQLExecuter, batchSize int) ([]*Payment, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be greater than 0")
	}

	// Get disbursement payments
	disbursementQuery := basePaymentQuery + `
        WHERE
            p.status = $1 -- 'READY'::payment_status
            AND rw.status = $2 -- 'REGISTERED'::receiver_wallet_status
            AND d.status = $3 -- 'STARTED'::disbursement_status
            AND p.payment_type = 'DISBURSEMENT'
        ORDER BY p.disbursement_id ASC, p.updated_at ASC
        FOR UPDATE SKIP LOCKED`

	var disbursementPayments []*Payment
	err := sqlExec.SelectContext(ctx, &disbursementPayments, disbursementQuery,
		ReadyPaymentStatus, RegisteredReceiversWalletStatus, StartedDisbursementStatus)
	if err != nil {
		return nil, fmt.Errorf("getting disbursement payments: %w", err)
	}

	// Get direct payments
	directQuery := basePaymentQuery + `
        WHERE
            p.status = $1 -- 'READY'::payment_status
            AND rw.status = $2 -- 'REGISTERED'::receiver_wallet_status
            AND p.payment_type = 'DIRECT'
        ORDER BY p.updated_at ASC
        FOR UPDATE OF p, rw SKIP LOCKED`

	var directPayments []*Payment
	err = sqlExec.SelectContext(ctx, &directPayments, directQuery,
		ReadyPaymentStatus, RegisteredReceiversWalletStatus)
	if err != nil {
		return nil, fmt.Errorf("getting direct payments: %w", err)
	}

	allPayments := make([]*Payment, 0, len(disbursementPayments)+len(directPayments))
	allPayments = append(allPayments, disbursementPayments...)
	allPayments = append(allPayments, directPayments...)

	// Apply the batch size limit after combining
	// we can enhance this feature to make 70%/30% disbursment/direct payments processing if needed instead of straightforward limiting
	if len(allPayments) > batchSize {
		allPayments = allPayments[:batchSize]
	}

	return allPayments, nil
}

// Count returns the number of payments matching the given query parameters.
func (p *PaymentModel) Count(ctx context.Context, queryParams *QueryParams, sqlExec db.SQLExecuter) (int, error) {
	var count int
	baseQuery := `
		SELECT
			count(*)
		FROM
			payments p
			LEFT JOIN disbursements d ON p.disbursement_id = d.id
			JOIN assets a ON p.asset_id = a.id
            JOIN receiver_wallets rw ON rw.id = p.receiver_wallet_id
			JOIN receivers r ON r.id = rw.receiver_id
 			JOIN wallets w ON w.id = rw.wallet_id
		`

	query, params := newPaymentQuery(baseQuery, queryParams, sqlExec, QueryTypeSingle)

	err := sqlExec.GetContext(ctx, &count, query, params...)
	if err != nil {
		return 0, fmt.Errorf("error counting payments: %w", err)
	}
	return count, nil
}

// GetAll returns all PAYMENTS matching the given query parameters.
func (p *PaymentModel) GetAll(ctx context.Context, queryParams *QueryParams, sqlExec db.SQLExecuter, queryType QueryType) ([]Payment, error) {
	payments := []Payment{}

	query, params := newPaymentQuery(basePaymentQuery, queryParams, sqlExec, queryType)

	err := sqlExec.SelectContext(ctx, &payments, query, params...)
	if err != nil {
		return nil, fmt.Errorf("selecting payments: %w", err)
	}

	return payments, nil
}

// DeleteAllDraftForDisbursement deletes all payments for a given disbursement.
func (p *PaymentModel) DeleteAllDraftForDisbursement(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) error {
	query := `
		DELETE FROM payments
		WHERE disbursement_id = $1
			AND status = $2
		`

	result, err := sqlExec.ExecContext(ctx, query, disbursementID, DraftPaymentStatus)
	if err != nil {
		return fmt.Errorf("error deleting payments for disbursement: %w", err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}

	log.Ctx(ctx).Infof("Deleted %d payments for disbursement %s", numRowsAffected, disbursementID)

	return nil
}

// InsertAll inserts a batch of payments into the database.
func (p *PaymentModel) InsertAll(ctx context.Context, sqlExec db.SQLExecuter, inserts []PaymentInsert) error {
	for _, payment := range inserts {
		err := payment.Validate()
		if err != nil {
			return fmt.Errorf("error validating payment: %w", err)
		}
	}
	query := `
		INSERT INTO payments (
			amount,
			asset_id,
			receiver_id,
			disbursement_id,
		    receiver_wallet_id,
			external_payment_id,
			payment_type
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		)
		`

	for _, payment := range inserts {
		_, err := sqlExec.ExecContext(ctx, query, payment.Amount, payment.AssetID, payment.ReceiverID, payment.DisbursementID, payment.ReceiverWalletID, payment.ExternalPaymentID, payment.PaymentType)
		if err != nil {
			return fmt.Errorf("error inserting payment: %w", err)
		}
	}

	return nil
}

// UpdateStatusByDisbursementID updates the status of all payments with a given status for a given disbursement.
func (p *PaymentModel) UpdateStatusByDisbursementID(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string, targetStatus PaymentStatus) error {
	sourceStatuses := targetStatus.SourceStatuses()

	query := `
		UPDATE payments
		SET status = $1,
			status_history = array_append(status_history, create_payment_status_history(NOW(), $1, NULL))
		WHERE disbursement_id = $2
		AND status = ANY($3)
	`

	result, err := sqlExec.ExecContext(ctx, query, targetStatus, disbursementID, pq.Array(sourceStatuses))
	if err != nil {
		return fmt.Errorf("error updating payment statuses for disbursement %s: %w", disbursementID, err)
	}
	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}

	log.Ctx(ctx).Infof("Set %d payments for disbursement %s from %s to %s", numRowsAffected, disbursementID, sourceStatuses, targetStatus)

	return nil
}

var getReadyPaymentsBaseQuery = basePaymentQuery + `
	WHERE
		p.status = $1 -- 'READY'::payment_status
		AND rw.status = $2 -- 'REGISTERED'::receiver_wallet_status
		AND d.status = $3 -- 'STARTED'::disbursement_status
`

var getReadyPaymentsBaseArgs = []any{ReadyPaymentStatus, RegisteredReceiversWalletStatus, StartedDisbursementStatus}

func (p *PaymentModel) GetReadyByDisbursementID(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) ([]*Payment, error) {
	query := getReadyPaymentsBaseQuery + "AND p.disbursement_id = $4"

	var payments []*Payment
	args := append(getReadyPaymentsBaseArgs, disbursementID)
	err := sqlExec.SelectContext(ctx, &payments, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting ready payments for disbursement ID %s: %w", disbursementID, err)
	}

	return payments, nil
}

func (p *PaymentModel) GetReadyByID(ctx context.Context, sqlExec db.SQLExecuter, paymentIDs ...string) ([]*Payment, error) {
	query := basePaymentQuery + `
        WHERE
            p.id = ANY($1)
            AND p.status = $2 -- 'READY'::payment_status
            AND rw.status = $3 -- 'REGISTERED'::receiver_wallet_status
            AND (
                (p.payment_type = 'DISBURSEMENT' AND d.status = $4) -- 'STARTED'::disbursement_status
                OR 
                (p.payment_type = 'DIRECT')
            )
        ORDER BY p.updated_at ASC`

	var payments []*Payment
	err := sqlExec.SelectContext(ctx, &payments, query,
		pq.Array(paymentIDs), ReadyPaymentStatus, RegisteredReceiversWalletStatus, StartedDisbursementStatus)
	if err != nil {
		return nil, fmt.Errorf("getting ready payments by IDs: %w", err)
	}

	return payments, nil
}

func (p *PaymentModel) GetReadyByReceiverWalletID(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID string) ([]*Payment, error) {
	query := basePaymentQuery + `
        WHERE
            rw.id = $1
            AND p.status = $2 -- 'READY'::payment_status
            AND rw.status = $3 -- 'REGISTERED'::receiver_wallet_status
            AND (
                (p.payment_type = 'DISBURSEMENT' AND d.status = $4) -- 'STARTED'::disbursement_status
                OR 
                (p.payment_type = 'DIRECT')
            )`

	var payments []*Payment
	err := sqlExec.SelectContext(ctx, &payments, query,
		receiverWalletID, ReadyPaymentStatus, RegisteredReceiversWalletStatus, StartedDisbursementStatus)
	if err != nil {
		return nil, fmt.Errorf("getting ready payments by receiver wallet ID %s: %w", receiverWalletID, err)
	}

	return payments, nil
}

func (p *PaymentModel) UpdateStatuses(ctx context.Context, sqlExec db.SQLExecuter, payments []*Payment, toStatus PaymentStatus) (int64, error) {
	if len(payments) == 0 {
		log.Ctx(ctx).Debugf("No payments to update")
		return 0, nil
	}
	// Validate transition
	for _, payment := range payments {
		if err := payment.Status.TransitionTo(toStatus); err != nil {
			return 0, fmt.Errorf("cannot transition from %s to %s for payment %s: %w", payment.Status, toStatus, payment.ID, err)
		}
	}
	var paymentIDs []string
	for _, payment := range payments {
		paymentIDs = append(paymentIDs, payment.ID)
	}

	query := `
		UPDATE payments
		SET status = $1,
			status_history = array_append(status_history, create_payment_status_history(NOW(), $1, NULL))
		WHERE id = ANY($2)
	`

	result, err := sqlExec.ExecContext(ctx, query, toStatus, pq.Array(paymentIDs))
	if err != nil {
		return 0, fmt.Errorf("error updating payment statuses: %w", err)
	}
	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("error getting number of rows affected: %w", err)
	}

	return numRowsAffected, nil
}

// Update updates a payment's fields with the given update.
func (p *PaymentModel) Update(ctx context.Context, sqlExec db.SQLExecuter, payment *Payment, update *PaymentUpdate) error {
	if err := update.Validate(); err != nil {
		return fmt.Errorf("error validating payment update: %w", err)
	}

	if err := payment.Status.TransitionTo(update.Status); err != nil {
		return fmt.Errorf("cannot transition from %s to %s for payment %s: %w", payment.Status, update.Status, payment.ID, err)
	}

	query := `
		UPDATE payments
		SET status = $1,
			status_history = array_append(status_history, create_payment_status_history(NOW(), $1, $2)),
			stellar_transaction_id = COALESCE($3, stellar_transaction_id)
		WHERE id = $4
	`

	result, err := sqlExec.ExecContext(ctx, query, update.Status, update.StatusMessage, update.StellarTransactionID, payment.ID)
	if err != nil {
		return fmt.Errorf("error updating payment with id %s: %w", payment.ID, err)
	}
	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected for payment with id %s: %w", payment.ID, err)
	}
	if numRowsAffected == 0 {
		return fmt.Errorf("payment %s status was not updated from %s to %s", payment.ID, payment.Status, update.Status)
	} else if numRowsAffected == 1 {
		log.Ctx(ctx).Infof("Set payment %s status from %s to %s", payment.ID, payment.Status, update.Status)
	} else {
		return fmt.Errorf("unexpected number of rows affected: %d when updating payment %s status from %s to %s", numRowsAffected, payment.ID, payment.Status, update.Status)
	}

	return nil
}

func (p *PaymentModel) RetryFailedPayments(ctx context.Context, sqlExec db.SQLExecuter, email string, paymentIDs ...string) error {
	if len(paymentIDs) == 0 {
		return fmt.Errorf("payment ids is required: %w", ErrMissingInput)
	}

	if email == "" {
		return fmt.Errorf("user email is required: %w", ErrMissingInput)
	}

	const query = `
		WITH previous_payments_stellar_transaction_ids AS (
			SELECT
				id,
				stellar_transaction_id,
				$2 AS status_message
			FROM
				payments
			WHERE
				id = ANY($1)
				AND status = 'FAILED'::payment_status
		)
		UPDATE
			payments
		SET
			status = 'READY'::payment_status,
			stellar_transaction_id = '',
			status_history = array_append(status_history, create_payment_status_history(NOW(), 'READY', CONCAT(pp.status_message, pp.stellar_transaction_id)))
		FROM
			previous_payments_stellar_transaction_ids pp
		WHERE
			payments.id = pp.id
		`

	statusMessage := fmt.Sprintf("User %s has requested to retry the payment - Previous Stellar Transaction ID: ", email)

	res, err := sqlExec.ExecContext(ctx, query, pq.Array(paymentIDs), statusMessage)
	if err != nil {
		return fmt.Errorf("error retrying failed payments: %w", err)
	}

	numRowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}

	if numRowsAffected != int64(len(paymentIDs)) {
		return ErrMismatchNumRowsAffected
	}

	// This ensures that we are going to sync the payment transaction on the Anchor Platform again.
	const updateReceiverWallets = `
		UPDATE
			receiver_wallets
		SET
			anchor_platform_transaction_synced_at = NULL
		WHERE
			id IN (
				SELECT receiver_wallet_id FROM payments WHERE id = ANY($1)
			)
		`
	_, err = sqlExec.ExecContext(ctx, updateReceiverWallets, pq.Array(paymentIDs))
	if err != nil {
		return fmt.Errorf("resetting the receiver wallets' anchor platform transaction synced at: %w", err)
	}

	return nil
}

// GetByIDs returns a list of payments for the given IDs.
func (p *PaymentModel) GetByIDs(ctx context.Context, sqlExec db.SQLExecuter, paymentIDs []string) ([]Payment, error) {
	queryParams := &QueryParams{
		Filters: map[FilterKey]interface{}{
			FilterKeyID: paymentIDs,
		},
		SortBy:    SortFieldUpdatedAt,
		SortOrder: SortOrderASC,
	}

	payments, err := p.GetAll(ctx, queryParams, sqlExec, QueryTypeSelectAll)
	if err != nil {
		return nil, fmt.Errorf("getting payments by IDs: %w", err)
	}

	return payments, nil
}

func addArrayOrSingleCondition[T any](qb *QueryBuilder, fieldName string, value interface{}) {
	if slice, ok := value.([]T); ok && len(slice) > 0 {
		qb.AddCondition(fieldName+" = ANY(?)", pq.Array(slice))
	} else {
		qb.AddCondition(fieldName+" = ?", value)
	}
}

// newPaymentQuery generates the full query and parameters for a payment search query.
func newPaymentQuery(baseQuery string, queryParams *QueryParams, sqlExec db.SQLExecuter, queryType QueryType) (string, []any) {
	qb := NewQueryBuilder(baseQuery)
	if queryParams.Query != "" {
		q := "%" + queryParams.Query + "%"
		qb.AddCondition("(p.id ILIKE ? OR p.external_payment_id ILIKE ? OR rw.stellar_address ILIKE ? OR COALESCE(d.name, '') ILIKE ?)", q, q, q, q)
	}
	if queryParams.Filters[FilterKeyID] != nil {
		addArrayOrSingleCondition[string](qb, "p.id", queryParams.Filters[FilterKeyID])
	}
	if queryParams.Filters[FilterKeyStatus] != nil {
		addArrayOrSingleCondition[PaymentStatus](qb, "p.status", queryParams.Filters[FilterKeyStatus])
	}
	if queryParams.Filters[FilterKeyReceiverID] != nil {
		qb.AddCondition("p.receiver_id = ?", queryParams.Filters[FilterKeyReceiverID])
	}
	if queryParams.Filters[FilterKeyCreatedAtAfter] != nil {
		qb.AddCondition("p.created_at >= ?", queryParams.Filters[FilterKeyCreatedAtAfter])
	}
	if queryParams.Filters[FilterKeyCreatedAtBefore] != nil {
		qb.AddCondition("p.created_at <= ?", queryParams.Filters[FilterKeyCreatedAtBefore])
	}
	if queryParams.Filters[FilterKeyPaymentType] != nil {
		addArrayOrSingleCondition[PaymentType](qb, "p.payment_type", queryParams.Filters[FilterKeyPaymentType])
	}

	switch queryType {
	case QueryTypeSelectPaginated:
		qb.AddPagination(queryParams.Page, queryParams.PageLimit)
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "p")
	case QueryTypeSelectAll:
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "p")
	case QueryTypeSingle:
		// no need to sort or paginate.
	}

	query, params := qb.Build()
	return sqlExec.Rebind(query), params
}

// CancelPaymentsWithinPeriodDays cancels automatically payments that are in "READY" status after a certain time period in days.
func (p *PaymentModel) CancelPaymentsWithinPeriodDays(ctx context.Context, sqlExec db.SQLExecuter, periodInDays int64) error {
	query := `
		UPDATE 
			payments
		SET 
			status = 'CANCELED'::payment_status,
			status_history = array_append(status_history, create_payment_status_history(NOW(), 'CANCELED', NULL))
		WHERE 
			status = 'READY'::payment_status
			AND (
				SELECT (value->>'timestamp')::timestamp
				FROM unnest(status_history) AS value
				WHERE value->>'status' = 'READY' 
				ORDER BY (value->>'timestamp')::timestamp DESC 
				LIMIT 1
			) <= $1
	`

	result, err := sqlExec.ExecContext(ctx, query, time.Now().AddDate(0, 0, -int(periodInDays)))
	if err != nil {
		return fmt.Errorf("error canceling payments: %w", err)
	}
	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}
	if numRowsAffected == 0 {
		log.Ctx(ctx).Debug("No payments were canceled")
	}

	return nil
}

// UpdateStatus updates the status of a payment.
func (p *PaymentModel) UpdateStatus(
	ctx context.Context,
	sqlExec db.SQLExecuter,
	paymentID string,
	status PaymentStatus,
	statusMsg *string,
	stellarTransactionHash string,
) error {
	if paymentID == "" {
		return fmt.Errorf("paymentID is required")
	}

	err := status.Validate()
	if err != nil {
		return fmt.Errorf("status is invalid: %w", err)
	}

	args := []interface{}{status, statusMsg, paymentID}
	query := `
		UPDATE
			payments
		SET 
			status = $1::payment_status,
			status_history = array_append(status_history, create_payment_status_history(NOW(), $1, $2))
			%s
		WHERE
			id = $3
	`
	var optionalQuerySet string
	if stellarTransactionHash != "" {
		args = append(args, stellarTransactionHash)
		optionalQuerySet = fmt.Sprintf(", stellar_transaction_id = $%d", len(args))
	}
	query = fmt.Sprintf(query, optionalQuerySet)

	result, err := sqlExec.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("marking payment as %s: %w", status, err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting number of rows affected: %w", err)
	}
	if numRowsAffected == 0 {
		return fmt.Errorf("payment with ID %s was not found: %w", paymentID, ErrRecordNotFound)
	}

	return nil
}

func (p *PaymentModel) CreateDirectPayment(ctx context.Context, sqlExec db.SQLExecuter, insert PaymentInsert) (string, error) {
	if err := insert.Validate(); err != nil {
		return "", fmt.Errorf("validating payment: %w", err)
	}

	query := `
        INSERT INTO payments (
            amount,
            asset_id,
            receiver_id,
            receiver_wallet_id,
            external_payment_id,
            payment_type,
            status,
            status_history
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7,
            ARRAY[create_payment_status_history(NOW(), $7::payment_status, NULL)]
        )
        RETURNING id
    `

	var paymentID string
	if err := sqlExec.GetContext(ctx, &paymentID, query,
		insert.Amount,
		insert.AssetID,
		insert.ReceiverID,
		insert.ReceiverWalletID,
		insert.ExternalPaymentID,
		insert.PaymentType,
		DraftPaymentStatus,
	); err != nil {
		return "", fmt.Errorf("inserting direct payment: %w", err)
	}

	return paymentID, nil
}
