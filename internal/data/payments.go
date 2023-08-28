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

	"github.com/stellar/go/support/log"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type Payment struct {
	ID                   string `json:"id" db:"id"`
	Amount               string `json:"amount" db:"amount"`
	StellarTransactionID string `json:"stellar_transaction_id" db:"stellar_transaction_id"`
	// TODO: evaluate if we will keep or remove StellarOperationID
	StellarOperationID string               `json:"stellar_operation_id" db:"stellar_operation_id"`
	Status             PaymentStatus        `json:"status" db:"status"`
	StatusHistory      PaymentStatusHistory `json:"status_history,omitempty" db:"status_history"`
	Disbursement       *Disbursement        `json:"disbursement,omitempty" db:"disbursement"`
	Asset              Asset                `json:"asset"`
	ReceiverWallet     *ReceiverWallet      `json:"receiver_wallet,omitempty" db:"receiver_wallet"`
	CreatedAt          time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at" db:"updated_at"`
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
	AllowedPaymentFilters   = []FilterKey{FilterKeyStatus, FilterKeyCreatedAtAfter, FilterKeyCreatedAtBefore, FilterKeyReceiverID}
	AllowedPaymentSorts     = []SortField{SortFieldCreatedAt, SortFieldUpdatedAt}
)

type PaymentInsert struct {
	ReceiverID       string `db:"receiver_id"`
	DisbursementID   string `db:"disbursement_id"`
	Amount           string `db:"amount"`
	AssetID          string `db:"asset_id"`
	ReceiverWalletID string `db:"receiver_wallet_id"`
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

func (p *PaymentInsert) Validate() error {
	if strings.TrimSpace(p.ReceiverID) == "" {
		return fmt.Errorf("receiver_id is required")
	}

	if strings.TrimSpace(p.DisbursementID) == "" {
		return fmt.Errorf("disbursement_id is required")
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

func (p *PaymentModel) Get(ctx context.Context, id string, sqlExec db.SQLExecuter) (*Payment, error) {
	payment := Payment{}

	query := `
		SELECT
			p.id,
			p.amount,
			COALESCE(p.stellar_transaction_id, '') as stellar_transaction_id,
			COALESCE(p.stellar_operation_id, '') as stellar_operation_id,
			p.status,
			p.status_history,
			p.created_at,
			p.updated_at,
			d.id as "disbursement.id",
			d.name as "disbursement.name",
			d.status as "disbursement.status",
			d.created_at as "disbursement.created_at",
			d.updated_at as "disbursement.updated_at",
			a.id as "asset.id",
			a.code as "asset.code",
			a.issuer as "asset.issuer",
			rw.id as "receiver_wallet.id",
			COALESCE(rw.stellar_address, '') as "receiver_wallet.stellar_address",
			COALESCE(rw.stellar_memo, '') as "receiver_wallet.stellar_memo",
			COALESCE(rw.stellar_memo_type, '') as "receiver_wallet.stellar_memo_type",
			rw.status as "receiver_wallet.status",
			rw.created_at as "receiver_wallet.created_at",
			rw.updated_at as "receiver_wallet.updated_at",
			rw.receiver_id as "receiver_wallet.receiver.id",
			w.id as "receiver_wallet.wallet.id",
			w.name as "receiver_wallet.wallet.name"
		FROM
			payments p
		JOIN disbursements d ON p.disbursement_id = d.id
		JOIN assets a ON p.asset_id = a.id
		JOIN receiver_wallets rw ON rw.receiver_id = p.receiver_id AND rw.wallet_id = d.wallet_id
		JOIN wallets w ON rw.wallet_id = w.id
		WHERE p.id = $1
		`

	err := sqlExec.GetContext(ctx, &payment, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		} else {
			return nil, fmt.Errorf("error querying payment ID: %w", err)
		}
	}

	return &payment, nil
}

// Count returns the number of payments matching the given query parameters.
func (p *PaymentModel) Count(ctx context.Context, queryParams *QueryParams, sqlExec db.SQLExecuter) (int, error) {
	var count int
	baseQuery := `
		SELECT
			count(*)
		FROM
			payments p
		JOIN disbursements d on p.disbursement_id = d.id
		JOIN assets a on p.asset_id = a.id
		JOIN wallets w on d.wallet_id = w.id			
		JOIN receiver_wallets rw on rw.receiver_id = p.receiver_id AND rw.wallet_id = w.id
		`

	query, params := newPaymentQuery(baseQuery, queryParams, false, sqlExec)

	err := sqlExec.GetContext(ctx, &count, query, params...)
	if err != nil {
		return 0, fmt.Errorf("error counting payments: %w", err)
	}
	return count, nil
}

// GetAll returns all PAYMENTS matching the given query parameters.
func (p *PaymentModel) GetAll(ctx context.Context, queryParams *QueryParams, sqlExec db.SQLExecuter) ([]Payment, error) {
	payments := []Payment{}

	query := `
		SELECT
			p.id,
			p.amount,
			COALESCE(p.stellar_transaction_id, '') as stellar_transaction_id,
			COALESCE(p.stellar_operation_id, '') as stellar_operation_id,
			p.status,
			p.status_history,
			p.created_at,
			p.updated_at,
			d.id as "disbursement.id",
			d.name as "disbursement.name",
			d.status as "disbursement.status",
			d.created_at as "disbursement.created_at",
			d.updated_at as "disbursement.updated_at",
			a.id as "asset.id",
			a.code as "asset.code",
			a.issuer as "asset.issuer",
			rw.id as "receiver_wallet.id",
			COALESCE(rw.stellar_address, '') as "receiver_wallet.stellar_address",
			COALESCE(rw.stellar_memo, '') as "receiver_wallet.stellar_memo",
			COALESCE(rw.stellar_memo_type, '') as "receiver_wallet.stellar_memo_type",
			rw.status as "receiver_wallet.status",
			rw.created_at as "receiver_wallet.created_at",
			rw.updated_at as "receiver_wallet.updated_at",
			rw.receiver_id as "receiver_wallet.receiver.id",
			w.id as "receiver_wallet.wallet.id",
			w.name as "receiver_wallet.wallet.name"
		FROM
			payments p
		JOIN disbursements d on p.disbursement_id = d.id
		JOIN assets a on p.asset_id = a.id
		JOIN wallets w on d.wallet_id = w.id
		JOIN receiver_wallets rw on rw.receiver_id = p.receiver_id AND rw.wallet_id = w.id
	`

	query, params := newPaymentQuery(query, queryParams, true, sqlExec)

	err := sqlExec.SelectContext(ctx, &payments, query, params...)
	if err != nil {
		return nil, fmt.Errorf("error querying payments: %w", err)
	}

	return payments, nil
}

// DeleteAllForDisbursement deletes all payments for a given disbursement.
func (p *PaymentModel) DeleteAllForDisbursement(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) error {
	query := `
		DELETE FROM payments
		WHERE disbursement_id = $1
		`

	result, err := sqlExec.ExecContext(ctx, query, disbursementID)
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
		    receiver_wallet_id
		) VALUES (
			$1,
			$2,
			$3,
			$4,
		    $5
		)
		`

	for _, payment := range inserts {
		_, err := sqlExec.ExecContext(ctx, query, payment.Amount, payment.AssetID, payment.ReceiverID, payment.DisbursementID, payment.ReceiverWalletID)
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

func (p *PaymentModel) GetBatchForUpdate(ctx context.Context, dbTx db.DBTransaction, batchSize int) ([]*Payment, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be greater than 0")
	}

	query := `
		SELECT
			p.id,
			p.amount,
			COALESCE(p.stellar_transaction_id, '') as "stellar_transaction_id",
			COALESCE(p.stellar_operation_id, '') as "stellar_operation_id",
			p.status,
			p.created_at,
			p.updated_at,
			d.id as "disbursement.id",
			d.status as "disbursement.status",
			a.id as "asset.id",
			a.code as "asset.code",
			a.issuer as "asset.issuer",
			rw.id as "receiver_wallet.id",
			rw.receiver_id as "receiver_wallet.receiver.id",
			COALESCE(rw.stellar_address, '') as "receiver_wallet.stellar_address",
			COALESCE(rw.stellar_memo, '') as "receiver_wallet.stellar_memo",
			COALESCE(rw.stellar_memo_type, '') as "receiver_wallet.stellar_memo_type",
			rw.status as "receiver_wallet.status"
		FROM
			payments p
				JOIN assets a on p.asset_id = a.id
				JOIN receiver_wallets rw on p.receiver_wallet_id = rw.id
				JOIN disbursements d on p.disbursement_id = d.id
		WHERE p.status = $1 -- 'READY'::payment_status
		AND rw.status = $2 -- 'REGISTERED'::receiver_wallet_status
		AND d.status = $3 -- 'STARTED'::disbursement_status
		ORDER BY p.disbursement_id ASC, p.updated_at ASC
		LIMIT $4
		FOR UPDATE SKIP LOCKED
		`

	var payments []*Payment
	err := dbTx.SelectContext(ctx, &payments, query, ReadyPaymentStatus, RegisteredReceiversWalletStatus, StartedDisbursementStatus, batchSize)
	if err != nil {
		return nil, fmt.Errorf("error getting ready payments: %w", err)
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

func (p *PaymentModel) RetryFailedPayments(ctx context.Context, email string, paymentIDs ...string) error {
	return db.RunInTransaction(ctx, p.dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
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

		res, err := dbTx.ExecContext(ctx, query, pq.Array(paymentIDs), statusMessage)
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

		return nil
	})
}

// GetByIDs returns a list of payments for the given IDs.
func (p *PaymentModel) GetByIDs(ctx context.Context, sqlExec db.SQLExecuter, paymentIDs []string) ([]*Payment, error) {
	payments := []*Payment{}

	if len(paymentIDs) == 0 {
		return payments, nil
	}

	query := `
		SELECT
			p.id,
			p.amount,
			COALESCE(p.stellar_transaction_id, '') as "stellar_transaction_id",
			COALESCE(p.stellar_operation_id, '') as "stellar_operation_id",
			p.status,
			p.created_at,
			p.updated_at,
			d.id as "disbursement.id",
			d.status as "disbursement.status",
			a.id as "asset.id",
			a.code as "asset.code",
			a.issuer as "asset.issuer",
			rw.id as "receiver_wallet.id",
			rw.receiver_id as "receiver_wallet.receiver.id",
			COALESCE(rw.stellar_address, '') as "receiver_wallet.stellar_address",
			COALESCE(rw.stellar_memo, '') as "receiver_wallet.stellar_memo",
			COALESCE(rw.stellar_memo_type, '') as "receiver_wallet.stellar_memo_type",
			rw.status as "receiver_wallet.status"
		FROM
			payments p
				JOIN assets a on p.asset_id = a.id
				JOIN receiver_wallets rw on p.receiver_wallet_id = rw.id
				JOIN disbursements d on p.disbursement_id = d.id
		WHERE p.id = ANY($1)
	`

	err := sqlExec.SelectContext(ctx, &payments, query, pq.Array(paymentIDs))
	if err != nil {
		return nil, fmt.Errorf("error getting payments: %w", err)
	}
	return payments, nil
}

// newPaymentQuery generates the full query and parameters for a payment search query
func newPaymentQuery(baseQuery string, queryParams *QueryParams, paginated bool, sqlExec db.SQLExecuter) (string, []interface{}) {
	qb := NewQueryBuilder(baseQuery)
	if queryParams.Filters[FilterKeyStatus] != nil {
		qb.AddCondition("p.status = ?", queryParams.Filters[FilterKeyStatus])
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
	if paginated {
		qb.AddSorting(queryParams.SortBy, queryParams.SortOrder, "p")
		qb.AddPagination(queryParams.Page, queryParams.PageLimit)
	}
	query, params := qb.Build()
	return sqlExec.Rebind(query), params
}
