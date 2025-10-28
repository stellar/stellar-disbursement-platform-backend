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
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type ReceiversWalletStatusHistoryEntry struct {
	Status    ReceiversWalletStatus `json:"status"`
	Timestamp time.Time             `json:"timestamp"`
}

type ReceiversWalletStatusHistory []ReceiversWalletStatusHistoryEntry

// Value implements the driver.Valuer interface.
func (rwsh ReceiversWalletStatusHistory) Value() (driver.Value, error) {
	var statusHistoryJSON []string
	for _, sh := range rwsh {
		shJSONBytes, err := json.Marshal(sh)
		if err != nil {
			return nil, fmt.Errorf("converting receiver status history to json for message: %w", err)
		}
		statusHistoryJSON = append(statusHistoryJSON, string(shJSONBytes))
	}

	return pq.Array(statusHistoryJSON).Value()
}

var _ driver.Valuer = (*ReceiversWalletStatusHistory)(nil)

// Scan implements the sql.Scanner interface.
func (rwsh *ReceiversWalletStatusHistory) Scan(src interface{}) error {
	var statusHistoryJSON []string
	if err := pq.Array(&statusHistoryJSON).Scan(src); err != nil {
		return fmt.Errorf("error scanning status history value: %w", err)
	}

	for _, sh := range statusHistoryJSON {
		var shEntry ReceiversWalletStatusHistoryEntry
		err := json.Unmarshal([]byte(sh), &shEntry)
		if err != nil {
			return fmt.Errorf("error unmarshaling status_history column: %w", err)
		}
		*rwsh = append(*rwsh, shEntry)
	}

	return nil
}

var _ sql.Scanner = (*ReceiversWalletStatusHistory)(nil)

type ReceiverWallet struct {
	ID               string                       `json:"id" db:"id"`
	Receiver         Receiver                     `json:"receiver" db:"receiver"`
	Wallet           Wallet                       `json:"wallet" db:"wallet"`
	StellarAddress   string                       `json:"stellar_address,omitempty" db:"stellar_address"`
	StellarMemo      string                       `json:"stellar_memo,omitempty" db:"stellar_memo"`
	StellarMemoType  schema.MemoType              `json:"stellar_memo_type,omitempty" db:"stellar_memo_type"`
	Status           ReceiversWalletStatus        `json:"status" db:"status"`
	StatusHistory    ReceiversWalletStatusHistory `json:"status_history,omitempty" db:"status_history"`
	CreatedAt        time.Time                    `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at" db:"updated_at"`
	OTP              string                       `json:"-" db:"otp"`
	OTPAttempts      int                          `json:"-" db:"otp_attempts"`
	OTPCreatedAt     *time.Time                   `json:"-" db:"otp_created_at"`
	OTPConfirmedAt   *time.Time                   `json:"otp_confirmed_at,omitempty" db:"otp_confirmed_at"`
	OTPConfirmedWith string                       `json:"otp_confirmed_with,omitempty" db:"otp_confirmed_with"`
	// AnchorPlatformAccountID is the ID of the SEP24 transaction initiated by the Anchor Platform where the receiver wallet was registered.
	AnchorPlatformTransactionID       string     `json:"anchor_platform_transaction_id,omitempty" db:"anchor_platform_transaction_id"`
	AnchorPlatformTransactionSyncedAt *time.Time `json:"anchor_platform_transaction_synced_at,omitempty" db:"anchor_platform_transaction_synced_at"`
	InvitedAt                         *time.Time `json:"invited_at,omitempty" db:"invited_at"`
	LastMessageSentAt                 *time.Time `json:"last_message_sent_at,omitempty" db:"last_message_sent_at"`
	InvitationSentAt                  *time.Time `json:"invitation_sent_at" db:"invitation_sent_at"`
	ReceiverWalletStats
}

type ReceiverWalletStats struct {
	TotalPayments     string          `json:"total_payments,omitempty" db:"total_payments"`
	PaymentsReceived  string          `json:"payments_received,omitempty" db:"payments_received"`
	FailedPayments    string          `json:"failed_payments,omitempty" db:"failed_payments"`
	CanceledPayments  string          `json:"canceled_payments,omitempty" db:"canceled_payments"`
	RemainingPayments string          `json:"remaining_payments,omitempty" db:"remaining_payments"`
	ReceivedAmounts   ReceivedAmounts `json:"received_amounts,omitempty" db:"received_amounts"`
	// TotalInvitationResentAttempts holds how many times were resent the Invitation SMS to the receiver
	// since the last invitation has been sent.
	TotalInvitationResentAttempts int64 `json:"-" db:"total_invitation_resent_attempts"`
}

type ReceiverWalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

type ReceiverWalletInsert struct {
	ReceiverID string
	WalletID   string
}

func (rw *ReceiverWalletModel) GetWithReceiverIDs(ctx context.Context, sqlExec db.SQLExecuter, receiverIDs ReceiverIDs) ([]ReceiverWallet, error) {
	receiverWallets := []ReceiverWallet{}
	query := `
	WITH receiver_wallets_cte AS (
		SELECT
			rw.id,
			rw.receiver_id,
			rw.anchor_platform_transaction_id,
			rw.stellar_address,
			rw.stellar_memo,
			rw.stellar_memo_type,
			rw.status,
			rw.created_at,
			rw.updated_at,
			w.id as wallet_id,
			w.name as wallet_name,
			w.homepage as wallet_homepage,
			w.sep_10_client_domain as wallet_sep_10_client_domain,
			w.enabled as wallet_enabled
		FROM receiver_wallets rw
		JOIN wallets w ON rw.wallet_id = w.id
		WHERE rw.receiver_id = ANY($1::varchar[])
	), receiver_wallets_stats AS (
		SELECT
			rwc.id as receiver_wallet_id,
			COUNT(p) as total_payments,
			COUNT(p) FILTER(WHERE p.status = 'SUCCESS') as payments_received,
			COUNT(p) FILTER(WHERE p.status = 'FAILED') as failed_payments,
			COUNT(p) FILTER(WHERE p.status = 'CANCELED') as canceled_payments,
			COUNT(p) FILTER(WHERE p.status IN ('DRAFT', 'READY', 'PENDING', 'PAUSED')) as remaining_payments,
			a.code as asset_code,
			a.issuer as asset_issuer,
			COALESCE(SUM(p.amount) FILTER(WHERE p.asset_id = a.id AND p.status = 'SUCCESS'), '0') as received_amount
		FROM receiver_wallets_cte rwc
		JOIN payments p ON rwc.receiver_id = p.receiver_id
		JOIN disbursements d ON p.disbursement_id = d.id AND rwc.wallet_id = d.wallet_id
		JOIN assets a ON a.id = p.asset_id
		GROUP BY (rwc.id, a.code, a.issuer)
	), receiver_wallets_stats_aggregate AS (
		SELECT
			rws.receiver_wallet_id as receiver_wallet_id,
			SUM(rws.total_payments) as total_payments,
			SUM(rws.payments_received) as payments_received,
			SUM(rws.failed_payments) as failed_payments,
			SUM(rws.canceled_payments) as canceled_payments,
			SUM(rws.remaining_payments) as remaining_payments,
			jsonb_agg(jsonb_build_object('asset_code', rws.asset_code, 'asset_issuer', rws.asset_issuer, 'received_amount', rws.received_amount::text)) as received_amounts
		FROM receiver_wallets_stats rws
		GROUP BY (rws.receiver_wallet_id)
	), receiver_wallets_messages AS (
		SELECT
			rwc.id as receiver_wallet_id,
			MIN(m.created_at) as invited_at,
			MAX(m.created_at) as last_message_sent_at
		FROM receiver_wallets_cte rwc
		LEFT JOIN messages m ON rwc.id = m.receiver_wallet_id
		WHERE m.status = 'SUCCESS'
		GROUP BY (rwc.id)
	)
	SELECT 
		rwc.id,
		rwc.receiver_id as "receiver.id",
		COALESCE(rwc.anchor_platform_transaction_id, '') as anchor_platform_transaction_id,
		COALESCE(rwc.stellar_address, '') as stellar_address,
		COALESCE(rwc.stellar_memo, '') as stellar_memo,
		COALESCE(rwc.stellar_memo_type::text, '') as stellar_memo_type,
		rwc.status,
		rwc.created_at,
		rwc.updated_at,
		rwc.wallet_id as "wallet.id",
		rwc.wallet_name as "wallet.name",
		rwc.wallet_homepage as "wallet.homepage",
		rwc.wallet_sep_10_client_domain as "wallet.sep_10_client_domain",
		rwc.wallet_enabled as "wallet.enabled",
		COALESCE(rws.total_payments, '0') as total_payments,
		COALESCE(rws.payments_received, '0') as payments_received,
		COALESCE(rws.failed_payments, '0') as failed_payments,
		COALESCE(rws.canceled_payments, '0') as canceled_payments,
		COALESCE(rws.remaining_payments, '0') as remaining_payments,
		rws.received_amounts,
		rwm.invited_at as invited_at,
		rwm.last_message_sent_at as last_message_sent_at
	FROM receiver_wallets_cte rwc
	LEFT JOIN receiver_wallets_stats_aggregate rws ON rws.receiver_wallet_id = rwc.id
	LEFT JOIN receiver_wallets_messages rwm ON rwm.receiver_wallet_id = rwc.id
	ORDER BY rwc.created_at
	`

	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.StringArray(receiverIDs))
	if err != nil {
		return nil, fmt.Errorf("error querying receivers wallets: %w", err)
	}

	return receiverWallets, nil
}

func ReceiverWalletColumnNames(tableReference, resultAlias string) string {
	columns := SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		RawColumns: []string{
			"id",
			`receiver_id AS "receiver.id"`,
			`wallet_id AS "wallet.id"`,
			"otp_attempts",
			"otp_created_at",
			"otp_confirmed_at",
			"status",
			"status_history",
			"created_at",
			"updated_at",
			"invitation_sent_at",
			"anchor_platform_transaction_synced_at",
		},
		CoalesceColumns: []string{
			"anchor_platform_transaction_id",
			"stellar_address",
			"stellar_memo",
			"stellar_memo_type::text AS stellar_memo_type",
			"otp",
			"otp_confirmed_with",
		},
	}.Build()

	return strings.Join(columns, ",\n")
}

// GetByID returns a receiver wallet by ID
func (rw *ReceiverWalletModel) GetByID(ctx context.Context, sqlExec db.SQLExecuter, id string) (*ReceiverWallet, error) {
	query := `
		SELECT
			` + ReceiverWalletColumnNames("rw", "") + `,
			` + WalletColumnNames("w", "wallet", false) + `
		FROM
			receiver_wallets rw
		JOIN
			wallets w ON rw.wallet_id = w.id
		WHERE
			rw.id = $1
	`

	var receiverWallet ReceiverWallet
	err := sqlExec.GetContext(ctx, &receiverWallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("querying receiver wallet: %w", err)
	}
	return &receiverWallet, nil
}

// GetByIDs returns a receiver wallet by IDs
func (rw *ReceiverWalletModel) GetByIDs(ctx context.Context, sqlExec db.SQLExecuter, ids ...string) ([]ReceiverWallet, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no receiver wallet IDs provided")
	}

	query := `
		SELECT
			` + ReceiverWalletColumnNames("rw", "") + `,
			` + WalletColumnNames("w", "wallet", false) + `
		FROM
			receiver_wallets rw
		JOIN
			wallets w ON rw.wallet_id = w.id
		WHERE
			rw.id = ANY($1)
	`

	receiverWallets := make([]ReceiverWallet, len(ids))
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("querying receiver wallet: %w", err)
	}
	return receiverWallets, nil
}

// GetByReceiverIDsAndWalletID returns a list of receiver wallets by receiver IDs and wallet ID.
func (rw *ReceiverWalletModel) GetByReceiverIDsAndWalletID(ctx context.Context, sqlExec db.SQLExecuter, receiverIds []string, walletID string) ([]*ReceiverWallet, error) {
	receiverWallets := []*ReceiverWallet{}
	query := `
		SELECT
			` + ReceiverWalletColumnNames("rw", "") + `
		FROM receiver_wallets rw
		WHERE rw.receiver_id = ANY($1)
		AND rw.wallet_id = $2
	`
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.Array(receiverIds), walletID)
	if err != nil {
		return nil, fmt.Errorf("error querying receiver wallets: %w", err)
	}

	return receiverWallets, nil
}

const getPendingRegistrationReceiverWalletsBaseQuery = `
	SELECT
		rw.id,
		rw.invitation_sent_at,
		r.id AS "receiver.id",
		COALESCE(r.phone_number, '') as "receiver.phone_number",
		COALESCE(r.email, '') as "receiver.email",
		w.id AS "wallet.id",
		w.name AS "wallet.name"
	FROM
		receiver_wallets rw
		INNER JOIN receivers r ON r.id = rw.receiver_id
		INNER JOIN wallets w ON w.id = rw.wallet_id
		INNER JOIN disbursements d ON w.id = d.wallet_id
		INNER JOIN payments p ON d.id = p.disbursement_id AND p.receiver_id = r.id
	WHERE
		rw.status = $1 -- 'READY'::receiver_wallet_status
		%s
	GROUP BY
		rw.id,
		r.id,
		w.id
`

var getPendingRegistrationReceiverWalletsBaseArgs = []any{ReadyReceiversWalletStatus}

func (rw *ReceiverWalletModel) GetAllPendingRegistrations(ctx context.Context, sqlExec db.SQLExecuter) ([]*ReceiverWallet, error) {
	query := fmt.Sprintf(getPendingRegistrationReceiverWalletsBaseQuery, "")

	receiverWallets := make([]*ReceiverWallet, 0)
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, getPendingRegistrationReceiverWalletsBaseArgs...)
	if err != nil {
		return nil, fmt.Errorf("error querying pending registration receiver wallets: %w", err)
	}

	return receiverWallets, nil
}

func (rw *ReceiverWalletModel) GetAllPendingRegistrationByReceiverWalletIDs(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletIDs []string) ([]*ReceiverWallet, error) {
	query := fmt.Sprintf(getPendingRegistrationReceiverWalletsBaseQuery, "AND rw.id = ANY($2)")

	receiverWallets := make([]*ReceiverWallet, 0)
	args := append(getPendingRegistrationReceiverWalletsBaseArgs, pq.Array(receiverWalletIDs))
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying pending registration receiver wallets: %w", err)
	}

	return receiverWallets, nil
}

func (rw *ReceiverWalletModel) GetAllPendingRegistrationByDisbursementID(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string) ([]*ReceiverWallet, error) {
	query := fmt.Sprintf(getPendingRegistrationReceiverWalletsBaseQuery, "AND d.id = $2")

	receiverWallets := make([]*ReceiverWallet, 0)
	args := append(getPendingRegistrationReceiverWalletsBaseArgs, disbursementID)
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying pending registration receiver wallets for disbursement ID %s: %w", disbursementID, err)
	}

	return receiverWallets, nil
}

// UpdateOTPByReceiverContactInfoAndWalletDomain updates receiver wallet OTP if its not verified yet, and returns the
// number of updated rows.
func (rw *ReceiverWalletModel) UpdateOTPByReceiverContactInfoAndWalletDomain(ctx context.Context, receiverContactInfo, sep10ClientDomain, otp string) (numberOfUpdatedRows int, err error) {
	query := `
		WITH rw_cte AS (
			SELECT
				rw.id,
				rw.otp_confirmed_at
			FROM
				receiver_wallets rw
				INNER JOIN receivers r ON rw.receiver_id = r.id
				INNER JOIN wallets w ON rw.wallet_id = w.id
			WHERE
				(r.phone_number = $1 OR r.email = $1)
				AND w.sep_10_client_domain = $2
				AND rw.otp_confirmed_at IS NULL
		)
		UPDATE
			receiver_wallets
		SET
			otp = $3,
			otp_created_at = NOW(),
			otp_attempts = 0
		FROM rw_cte
		WHERE
			receiver_wallets.id = rw_cte.id
	`

	rows, err := rw.dbConnectionPool.ExecContext(ctx, query, receiverContactInfo, sep10ClientDomain, otp)
	if err != nil {
		return 0, fmt.Errorf("updating receiver wallets otp: %w", err)
	}

	updatedRowsAffected, err := rows.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting updated rows of receiver wallets otp: %w", err)
	}

	return int(updatedRowsAffected), nil
}

// GetOrInsertReceiverWallet inserts a new receiver wallet into the database.
func (rw *ReceiverWalletModel) GetOrInsertReceiverWallet(ctx context.Context, sqlExec db.SQLExecuter, insert ReceiverWalletInsert) (string, error) {
	var newID string
	query := `
		INSERT INTO receiver_wallets (receiver_id, wallet_id)
		VALUES ($1, $2)
		RETURNING id
	`

	err := sqlExec.GetContext(ctx, &newID, query, insert.ReceiverID, insert.WalletID)
	if err != nil {
		return "", fmt.Errorf("error inserting receiver wallet: %w", err)
	}
	return newID, nil
}

// GetByReceiverIDAndWalletDomain returns a receiver wallet that match the receiver ID and wallet domain.
func (rw *ReceiverWalletModel) GetByReceiverIDAndWalletDomain(ctx context.Context, receiverID string, walletDomain string, sqlExec db.SQLExecuter) (*ReceiverWallet, error) {
	query := `
		SELECT
			` + ReceiverWalletColumnNames("rw", "") + `,
			` + WalletColumnNames("w", "wallet", false) + `
		FROM
			receiver_wallets rw
		JOIN
			wallets w ON rw.wallet_id = w.id
		WHERE
			rw.receiver_id = $1
			AND w.sep_10_client_domain = $2
	`

	var receiverWallet ReceiverWallet
	err := sqlExec.GetContext(ctx, &receiverWallet, query, receiverID, walletDomain)
	if err != nil {
		return nil, fmt.Errorf("error querying receiver wallet: %w", err)
	}

	return &receiverWallet, nil
}

// UpdateStatusByDisbursementID updates the status of the receiver wallets associated with a disbursement.
func (rw *ReceiverWalletModel) UpdateStatusByDisbursementID(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string, from, to ReceiversWalletStatus) error {
	if err := from.TransitionTo(to); err != nil {
		return fmt.Errorf("cannot transition from %s to %s for receiver wallets for disbursement %s: %w", from, to, disbursementID, err)
	}
	query := `
		UPDATE receiver_wallets
		SET status = $1,
			status_history = array_append(status_history, create_receiver_wallet_status_history(NOW(), $1, ''))
		WHERE id IN (
			SELECT rw.id
			FROM payments p
			JOIN receiver_wallets rw on p.receiver_wallet_id = rw.id
			WHERE p.disbursement_id = $2
				AND rw.status = $3
		)
	`

	result, err := sqlExec.ExecContext(ctx, query, to, disbursementID, from)
	if err != nil {
		return fmt.Errorf("error updating receiver_wallets for disbursement %s: %w", disbursementID, err)
	}
	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}

	log.Ctx(ctx).Infof("Set %d receiver_wallet from %s to %s for disbursement %s", numRowsAffected, from, to, disbursementID)
	return nil
}

// GetByStellarAccountAndMemo returns a receiver wallets that match the Stellar Account, memo and client domain.
func (rw *ReceiverWalletModel) GetByStellarAccountAndMemo(ctx context.Context, stellarAccount, clientDomain string, stellarMemo *string) (*ReceiverWallet, error) {
	// build query
	var receiverWallets ReceiverWallet
	query := `
		SELECT
			` + ReceiverWalletColumnNames("rw", "") + `,
			` + WalletColumnNames("w", "wallet", false) + `
		FROM
			receiver_wallets rw
		JOIN
			wallets w ON rw.wallet_id = w.id
		WHERE
			rw.stellar_address = ?
	`

	// append memo to query if it is not empty
	args := []interface{}{stellarAccount}

	if clientDomain != "" {
		query += " AND w.sep_10_client_domain = ?"
		args = append(args, clientDomain)
	}

	if stellarMemo != nil {
		if *stellarMemo != "" {
			query += " AND rw.stellar_memo = ?"
			args = append(args, *stellarMemo)
		} else {
			query += " AND (rw.stellar_memo IS NULL OR rw.stellar_memo = '')"
		}
	}

	// execute query
	query = rw.dbConnectionPool.Rebind(query)
	err := rw.dbConnectionPool.GetContext(ctx, &receiverWallets, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no receiver wallet could be found in GetByStellarAccountAndMemo: %w", ErrRecordNotFound)
		}
		return nil, fmt.Errorf("error querying receiver wallet: %w", err)
	}

	return &receiverWallets, nil
}

func (rw *ReceiverWalletModel) UpdateAnchorPlatformTransactionSyncedAt(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID ...string) ([]ReceiverWallet, error) {
	query := `
		UPDATE
			receiver_wallets
		SET
			anchor_platform_transaction_synced_at = NOW()
		WHERE
			id = ANY($1)
			AND anchor_platform_transaction_synced_at IS NULL
			AND status = $2 -- 'REGISTERED'::receiver_wallet_status
		RETURNING ` + ReceiverWalletColumnNames("", "")

	var receiverWallets []ReceiverWallet
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.Array(receiverWalletID), RegisteredReceiversWalletStatus)
	if err != nil {
		return nil, fmt.Errorf("updating anchor platform transaction synced at: %w", err)
	}

	return receiverWallets, nil
}

// RetryInvitationMessage sets null the invitation_sent_at of a receiver wallet.
func (rw *ReceiverWalletModel) RetryInvitationMessage(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID string) (*ReceiverWallet, error) {
	var receiverWallet ReceiverWallet
	query := `
		UPDATE
			receiver_wallets rw
		SET
			invitation_sent_at = NULL
		WHERE rw.id = $1
		AND rw.status = 'READY'
		RETURNING ` + ReceiverWalletColumnNames("", "")

	err := sqlExec.GetContext(ctx, &receiverWallet, query, receiverWalletID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("updating receiver wallet: %w", err)
	}

	return &receiverWallet, nil
}

func (rw *ReceiverWalletModel) UpdateInvitationSentAt(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID ...string) ([]ReceiverWallet, error) {
	query := `
		UPDATE
			receiver_wallets
		SET
			invitation_sent_at = NOW()
		WHERE
			id = ANY($1)
			AND status = $2 -- 'READY'::receiver_wallet_status
		RETURNING ` + ReceiverWalletColumnNames("", "")

	var receiverWallets []ReceiverWallet
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.Array(receiverWalletID), ReadyReceiversWalletStatus)
	if err != nil {
		return nil, fmt.Errorf("updating invitation sent at: %w", err)
	}

	return receiverWallets, nil
}

type ReceiverWalletUpdate struct {
	Status                      ReceiversWalletStatus `db:"status"`
	AnchorPlatformTransactionID string                `db:"anchor_platform_transaction_id"`
	StellarAddress              string                `db:"stellar_address"`
	StellarMemo                 *string               `db:"stellar_memo"`
	StellarMemoType             *schema.MemoType      `db:"stellar_memo_type"`
	OTPConfirmedAt              time.Time             `db:"otp_confirmed_at"`
	OTPConfirmedWith            string                `db:"otp_confirmed_with"`
	OTPAttempts                 *int                  `db:"otp_attempts"`
}

func (rwu ReceiverWalletUpdate) Validate() error {
	if utils.IsEmpty(rwu) {
		return fmt.Errorf("no values provided to update receiver wallet")
	}

	if rwu.Status != "" {
		if err := rwu.Status.Validate(); err != nil {
			return fmt.Errorf("validating status: %w", err)
		}
	}

	if rwu.StellarAddress != "" {
		if !strkey.IsValidEd25519PublicKey(rwu.StellarAddress) && !strkey.IsValidContractAddress(rwu.StellarAddress) {
			return fmt.Errorf("invalid stellar address")
		}
	}

	if !time.Time.IsZero(rwu.OTPConfirmedAt) && rwu.OTPConfirmedWith == "" {
		return fmt.Errorf("OTPConfirmedWith is required when OTPConfirmedAt is provided")
	}

	if rwu.OTPConfirmedWith != "" && time.Time.IsZero(rwu.OTPConfirmedAt) {
		return fmt.Errorf("OTPConfirmedAt is required when OTPConfirmedWith is provided")
	}

	return nil
}

func (rw *ReceiverWalletModel) Update(ctx context.Context, id string, update ReceiverWalletUpdate, sqlExec db.SQLExecuter) error {
	if err := update.Validate(); err != nil {
		return fmt.Errorf("validating receiver wallet update: %w", err)
	}

	if update.StellarMemo != nil || update.StellarMemoType != nil {
		stellarAddress := update.StellarAddress
		if stellarAddress == "" {
			existing, err := rw.GetByID(ctx, sqlExec, id)
			if err != nil {
				return fmt.Errorf("checking stored stellar address for memo validation: %w", err)
			}
			stellarAddress = existing.StellarAddress
		}

		if strkey.IsValidContractAddress(stellarAddress) {
			return ErrMemosNotSupportedForContractAddresses
		}
	}

	fields := []string{}
	args := []interface{}{}

	if update.Status != "" {
		fields = append(fields, "status = ?")
		args = append(args, update.Status)
		fields = append(fields, "status_history = array_prepend(create_receiver_wallet_status_history(NOW(), ?, ''), status_history)")
		args = append(args, update.Status)
	}
	if update.AnchorPlatformTransactionID != "" {
		fields = append(fields, "anchor_platform_transaction_id = ?")
		args = append(args, update.AnchorPlatformTransactionID)
	}
	if update.StellarAddress != "" {
		fields = append(fields, "stellar_address = ?")
		args = append(args, update.StellarAddress)
	}
	if update.StellarMemo != nil {
		fields = append(fields, "stellar_memo = ?")
		args = append(args, utils.SQLNullString(*update.StellarMemo))
	}
	if update.StellarMemoType != nil {
		fields = append(fields, "stellar_memo_type = ?")
		args = append(args, utils.SQLNullString(string(*update.StellarMemoType)))
	}
	if !time.Time.IsZero(update.OTPConfirmedAt) {
		fields = append(fields, "otp_confirmed_at = ?")
		args = append(args, update.OTPConfirmedAt)
	}
	if update.OTPConfirmedWith != "" {
		fields = append(fields, "otp_confirmed_with = ?")
		args = append(args, update.OTPConfirmedWith)
	}
	if update.OTPAttempts != nil {
		fields = append(fields, "otp_attempts = ?")
		args = append(args, *update.OTPAttempts)
	}

	args = append(args, id)
	query := fmt.Sprintf(`
        UPDATE receiver_wallets
        SET %s
        WHERE id = ?
    `, strings.Join(fields, ", "))

	query = sqlExec.Rebind(query)
	result, err := sqlExec.ExecContext(ctx, query, args...)
	if err != nil {
		var pqError *pq.Error
		if errors.As(err, &pqError) && pqError.Code == "P0001" && strings.Contains(pqError.Message, "already belongs to another receiver") {
			return ErrDuplicateWalletAddress
		}
		return fmt.Errorf("updating receiver wallet: %w", err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return fmt.Errorf("no receiver wallet could be found in UpdateReceiverWallet: %w", ErrRecordNotFound)
	}

	return nil
}

var (
	ErrWalletNotRegistered                   = errors.New("receiver wallet not registered")
	ErrPaymentsInProgressForWallet           = errors.New("receiver wallet has payments in progress")
	ErrUnregisterUserManagedWallet           = errors.New("user managed wallet cannot be unregistered")
	ErrDuplicateWalletAddress                = errors.New("wallet address already in use")
	ErrMemosNotSupportedForContractAddresses = errors.New("memos are not supported for contract addresses")
)

// UpdateStatusToReady updates the status of a receiver wallet to "READY" and clears the stellar address and memo.
func (rw *ReceiverWalletModel) UpdateStatusToReady(ctx context.Context, id string) error {
	return db.RunInTransaction(ctx, rw.dbConnectionPool, nil, func(tx db.DBTransaction) error {
		// 1. Check if the receiver-wallet is in "REGISTERED" status
		receiverWallet, err := rw.GetByID(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("getting receiver wallet with ID %q: %w", id, err)
		}

		if receiverWallet.Status != RegisteredReceiversWalletStatus {
			return ErrWalletNotRegistered
		}

		// 2. Check if the wallet is user managed
		if receiverWallet.Wallet.UserManaged {
			return ErrUnregisterUserManagedWallet
		}

		// 3. Check if there are payments in progress
		paymentsInProgress, err := rw.HasPaymentsInProgress(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("checking payments in progress for receiver wallet %s: %w", id, err)
		}
		if paymentsInProgress {
			return ErrPaymentsInProgressForWallet
		}

		// Record wallet id and memo in status message.
		statusMessage := fmt.Sprintf("unregistered stellar address %s, memo %s", receiverWallet.StellarAddress, receiverWallet.StellarMemo)
		const q = `
			UPDATE receiver_wallets
			SET status = 'READY',
				status_history = array_append(status_history, create_receiver_wallet_status_history(NOW(), 'READY', $1)),
					stellar_address = NULL,
					stellar_memo = NULL,
					stellar_memo_type = NULL,
					invitation_sent_at = NULL,
					otp = NULL,
					otp_confirmed_at = NULL,
					otp_confirmed_with = NULL,
					otp_created_at = NULL,
					anchor_platform_transaction_id = NULL,
					anchor_platform_transaction_synced_at = NULL
			WHERE id = $2
	`
		_, err = tx.ExecContext(ctx, q, statusMessage, id)
		if err != nil {
			return fmt.Errorf("unregistering receiver wallet: %w", err)
		}
		return nil
	})
}

// HasPaymentsInProgress checks if there are any payments in progress for the given receiver wallet ID.
func (rw *ReceiverWalletModel) HasPaymentsInProgress(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID string) (bool, error) {
	const q = `
        SELECT EXISTS (
            SELECT 1
              FROM payments
             WHERE receiver_wallet_id = $1
               AND status = ANY($2)
        )
    `

	var exists bool
	if err := sqlExec.GetContext(ctx, &exists, q, receiverWalletID, pq.Array(PaymentInProgressStatuses())); err != nil {
		return false, fmt.Errorf("checking payments in progress for receiver wallet: %w", err)
	}
	return exists, nil
}
