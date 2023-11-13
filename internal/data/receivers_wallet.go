package data

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

const OTPExpirationTimeMinutes = 30

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
	ID              string                       `json:"id" db:"id"`
	Receiver        Receiver                     `json:"receiver" db:"receiver"`
	Wallet          Wallet                       `json:"wallet" db:"wallet"`
	StellarAddress  string                       `json:"stellar_address,omitempty" db:"stellar_address"`
	StellarMemo     string                       `json:"stellar_memo,omitempty" db:"stellar_memo"`
	StellarMemoType string                       `json:"stellar_memo_type,omitempty" db:"stellar_memo_type"`
	Status          ReceiversWalletStatus        `json:"status" db:"status"`
	StatusHistory   ReceiversWalletStatusHistory `json:"status_history,omitempty" db:"status_history"`
	CreatedAt       time.Time                    `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time                    `json:"updated_at" db:"updated_at"`
	OTP             string                       `json:"-" db:"otp"`
	OTPCreatedAt    *time.Time                   `json:"-" db:"otp_created_at"`
	OTPConfirmedAt  *time.Time                   `json:"otp_confirmed_at,omitempty" db:"otp_confirmed_at"`
	// AnchorPlatformAccountID is the ID of the SEP24 transaction initiated by the Anchor Platform where the receiver wallet was registered.
	AnchorPlatformTransactionID       string     `json:"anchor_platform_transaction_id,omitempty" db:"anchor_platform_transaction_id"`
	AnchorPlatformTransactionSyncedAt *time.Time `json:"anchor_platform_transaction_synced_at,omitempty" db:"anchor_platform_transaction_synced_at"`
	InvitedAt                         *time.Time `json:"invited_at,omitempty" db:"invited_at"`
	LastSmsSent                       *time.Time `json:"last_sms_sent,omitempty" db:"last_sms_sent"`
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
	// TotalInvitationSMSResentAttempts holds how many times were resent the Invitation SMS to the receiver
	// since the last invitation has been sent.
	TotalInvitationSMSResentAttempts int64 `json:"-" db:"total_invitation_sms_resent_attempts"`
}

type ReceiverWalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

type ReceiverWalletInsert struct {
	ReceiverID string
	WalletID   string
}

func (rw *ReceiverWalletModel) GetWithReceiverIds(ctx context.Context, sqlExec db.SQLExecuter, receiverIds ReceiverIDs) ([]ReceiverWallet, error) {
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
			MAX(m.created_at) as last_sms_sent
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
		COALESCE(rwc.stellar_memo_type, '') as stellar_memo_type,
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
		rwm.last_sms_sent as last_sms_sent
	FROM receiver_wallets_cte rwc
	LEFT JOIN receiver_wallets_stats_aggregate rws ON rws.receiver_wallet_id = rwc.id
	LEFT JOIN receiver_wallets_messages rwm ON rwm.receiver_wallet_id = rwc.id
	ORDER BY rwc.created_at
	`

	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.StringArray(receiverIds))
	if err != nil {
		return nil, fmt.Errorf("error querying receivers wallets: %w", err)
	}

	return receiverWallets, nil
}

// GetByReceiverIDsAndWalletID returns a list of receiver wallets by receiver IDs and wallet ID.
func (rw *ReceiverWalletModel) GetByReceiverIDsAndWalletID(ctx context.Context, sqlExec db.SQLExecuter, receiverIds []string, walletId string) ([]*ReceiverWallet, error) {
	receiverWallets := []*ReceiverWallet{}
	query := `
		SELECT
			rw.id,
			rw.receiver_id as "receiver.id",
			rw.wallet_id as "wallet.id",
			rw.status,
			rw.invitation_sent_at
		FROM receiver_wallets rw
		WHERE rw.receiver_id = ANY($1)
		AND rw.wallet_id = $2
	`
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.Array(receiverIds), walletId)
	if err != nil {
		return nil, fmt.Errorf("error querying receiver wallets: %w", err)
	}

	return receiverWallets, nil
}

func (rw *ReceiverWalletModel) GetAllPendingRegistration(ctx context.Context) ([]*ReceiverWallet, error) {
	const query = `
		SELECT
			rw.id,
			rw.invitation_sent_at,
			r.id AS "receiver.id",
			r.phone_number AS "receiver.phone_number",
			r.email AS "receiver.email",
			w.id AS "wallet.id",
			w.name AS "wallet.name"
		FROM
			receiver_wallets rw
			INNER JOIN receivers r ON r.id = rw.receiver_id
			INNER JOIN wallets w ON w.id = rw.wallet_id
		WHERE
			rw.status = 'READY'
	`

	receiverWallets := make([]*ReceiverWallet, 0)
	err := rw.dbConnectionPool.SelectContext(ctx, &receiverWallets, query)
	if err != nil {
		return nil, fmt.Errorf("error querying pending registration receiver wallets: %w", err)
	}

	return receiverWallets, nil
}

// UpdateOTPByReceiverPhoneNumberAndWalletDomain updates receiver wallet OTP if its not verified yet,
// and returns the number of updated rows.
func (rw *ReceiverWalletModel) UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx context.Context, receiverPhoneNumber, sep10ClientDomain, otp string) (numberOfUpdatedRows int, err error) {
	query := `
		WITH rw_cte AS (
			SELECT
				rw.id,
				rw.otp_confirmed_at
			FROM receiver_wallets rw
			INNER JOIN receivers r ON rw.receiver_id = r.id
			INNER JOIN wallets w ON rw.wallet_id = w.id
			WHERE r.phone_number = $1
			AND w.sep_10_client_domain = $2
			AND rw.otp_confirmed_at IS NULL
		)
		UPDATE
			receiver_wallets
		SET
			otp = $3,
			otp_created_at = NOW()
		FROM rw_cte
		WHERE
			receiver_wallets.id = rw_cte.id
	`

	rows, err := rw.dbConnectionPool.ExecContext(ctx, query, receiverPhoneNumber, sep10ClientDomain, otp)
	if err != nil {
		return 0, fmt.Errorf("error updating receiver wallets otp: %w", err)
	}

	updatedRowsAffected, err := rows.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("error getting updated rows of receiver wallets otp: %w", err)
	}

	return int(updatedRowsAffected), nil
}

// Insert inserts a new receiver wallet into the database.
func (rw *ReceiverWalletModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, insert ReceiverWalletInsert) (string, error) {
	var newId string
	query := `
		INSERT INTO receiver_wallets (receiver_id, wallet_id)
		VALUES ($1, $2)
		RETURNING id
	`

	err := sqlExec.GetContext(ctx, &newId, query, insert.ReceiverID, insert.WalletID)
	if err != nil {
		return "", fmt.Errorf("error inserting receiver wallet: %w", err)
	}
	return newId, nil
}

// GetByReceiverIDAndWalletDomain returns a receiver wallet that match the receiver ID and wallet domain.
func (rw *ReceiverWalletModel) GetByReceiverIDAndWalletDomain(ctx context.Context, receiverId string, walletDomain string, sqlExec db.SQLExecuter) (*ReceiverWallet, error) {
	var receiverWallet ReceiverWallet
	query := `
		SELECT
			rw.id,
			rw.receiver_id as "receiver.id",
			rw.status,
			COALESCE(rw.anchor_platform_transaction_id, '') as anchor_platform_transaction_id,
			COALESCE(rw.stellar_address, '') as stellar_address,
			COALESCE(rw.stellar_memo, '') as stellar_memo,
			COALESCE(rw.stellar_memo_type, '') as stellar_memo_type,
			COALESCE(rw.otp, '') as otp,
			rw.otp_created_at,
			rw.otp_confirmed_at,
			w.id as "wallet.id",
			w.name as "wallet.name",
			w.sep_10_client_domain as "wallet.sep_10_client_domain"
		FROM 
			receiver_wallets rw
		JOIN 
			wallets w ON rw.wallet_id = w.id
		WHERE
			rw.receiver_id = $1 
		AND	
			w.sep_10_client_domain = $2
	`

	err := sqlExec.GetContext(ctx, &receiverWallet, query, receiverId, walletDomain)
	if err != nil {
		return nil, fmt.Errorf("error querying receiver wallet: %w", err)
	}

	return &receiverWallet, nil
}

// UpdateReceiverWallet updates the status, address, OTP confirmation time, and anchor platform transaction ID of a
// receiver wallet.
func (rw *ReceiverWalletModel) UpdateReceiverWallet(ctx context.Context, receiverWallet ReceiverWallet, sqlExec db.SQLExecuter) error {
	query := `
		UPDATE 
			receiver_wallets rw
		SET 
			status = $1,
			anchor_platform_transaction_id = $2,
			stellar_address = $3,
			stellar_memo = $4,
			stellar_memo_type = $5,
			otp_confirmed_at = $6
		WHERE rw.id = $7
	`

	result, err := sqlExec.ExecContext(ctx, query,
		receiverWallet.Status,
		sql.NullString{String: receiverWallet.AnchorPlatformTransactionID, Valid: receiverWallet.AnchorPlatformTransactionID != ""},
		receiverWallet.StellarAddress,
		sql.NullString{String: receiverWallet.StellarMemo, Valid: receiverWallet.StellarMemo != ""},
		sql.NullString{String: receiverWallet.StellarMemoType, Valid: receiverWallet.StellarMemoType != ""},
		receiverWallet.OTPConfirmedAt,
		receiverWallet.ID)
	if err != nil {
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

// VerifyReceiverWalletOTP validates the receiver wallet OTP.
func (rw *ReceiverWalletModel) VerifyReceiverWalletOTP(ctx context.Context, networkPassphrase string, receiverWallet ReceiverWallet, otp string) error {
	if networkPassphrase == network.TestNetworkPassphrase {
		if otp == TestnetAlwaysValidOTP {
			log.Ctx(ctx).Warnf("OTP is being approved because TestnetAlwaysValidOTP (%s) was used", TestnetAlwaysValidOTP)
			return nil
		} else if otp == TestnetAlwaysInvalidOTP {
			log.Ctx(ctx).Errorf("OTP is being denied because TestnetAlwaysInvalidOTP (%s) was used", TestnetAlwaysInvalidOTP)
			return fmt.Errorf("otp does not match with value saved in the database")
		}
	}

	if receiverWallet.OTP != otp {
		return fmt.Errorf("otp does not match with value saved in the database")
	}

	if receiverWallet.OTPCreatedAt.IsZero() {
		return fmt.Errorf("otp does not have a valid created_at time")
	}

	otpExpirationTime := receiverWallet.OTPCreatedAt.Add(time.Minute * OTPExpirationTimeMinutes)
	if otpExpirationTime.Before(time.Now()) {
		return fmt.Errorf("otp is expired")
	}

	return nil
}

// UpdateStatusByDisbursementID updates the status of the receiver wallets associated with a disbursement.
func (rw *ReceiverWalletModel) UpdateStatusByDisbursementID(ctx context.Context, sqlExec db.SQLExecuter, disbursementID string, from, to ReceiversWalletStatus) error {
	if err := from.TransitionTo(to); err != nil {
		return fmt.Errorf("cannot transition from %s to %s for receiver wallets for disbursement %s: %w", from, to, disbursementID, err)
	}
	query := `
		UPDATE receiver_wallets
		SET status = $1,
			status_history = array_append(status_history, create_receiver_wallet_status_history(NOW(), $1))
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
func (rw *ReceiverWalletModel) GetByStellarAccountAndMemo(ctx context.Context, stellarAccount, stellarMemo, clientDomain string) (*ReceiverWallet, error) {
	// build query
	var receiverWallets ReceiverWallet
	query := `
		SELECT
			rw.id,
			rw.receiver_id as "receiver.id",
			rw.status,
			COALESCE(rw.anchor_platform_transaction_id, '') as anchor_platform_transaction_id,
			COALESCE(rw.stellar_address, '') as stellar_address,
			COALESCE(rw.stellar_memo, '') as stellar_memo,
			COALESCE(rw.stellar_memo_type, '') as stellar_memo_type,
			COALESCE(rw.otp, '') as otp,
			rw.otp_created_at,
			w.id as "wallet.id",
			w.name as "wallet.name",
			w.homepage as "wallet.homepage"
		FROM receiver_wallets rw
		JOIN wallets w ON rw.wallet_id = w.id
		WHERE rw.stellar_address = ?
	`

	// append memo to query if it is not empty
	args := []interface{}{stellarAccount}

	if clientDomain != "" {
		query += " AND w.sep_10_client_domain = ?"
		args = append(args, clientDomain)
	}

	if stellarMemo != "" {
		query += " AND rw.stellar_memo = ?"
		args = append(args, stellarMemo)
	} else {
		query += " AND (rw.stellar_memo IS NULL OR rw.stellar_memo = '')"
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

func (rw *ReceiverWalletModel) UpdateAnchorPlatformTransactionSyncedAt(ctx context.Context, receiverWalletID ...string) ([]ReceiverWallet, error) {
	const query = `
		UPDATE
			receiver_wallets
		SET
			anchor_platform_transaction_synced_at = NOW()
		WHERE
			id = ANY($1)
			AND anchor_platform_transaction_synced_at IS NULL
			AND status = $2 -- 'REGISTERED'::receiver_wallet_status
		RETURNING
			id, COALESCE(stellar_address, '') AS stellar_address, COALESCE(stellar_memo, '') AS stellar_memo,
			COALESCE(stellar_memo_type, '') AS stellar_memo_type, status, status_history,
			COALESCE(otp, '') AS otp, otp_confirmed_at, COALESCE(anchor_platform_transaction_id, '') AS anchor_platform_transaction_id,
			anchor_platform_transaction_synced_at
	`

	var receiverWallets []ReceiverWallet
	err := rw.dbConnectionPool.SelectContext(ctx, &receiverWallets, query, pq.Array(receiverWalletID), RegisteredReceiversWalletStatus)
	if err != nil {
		return nil, fmt.Errorf("updating anchor platform transaction synced at: %w", err)
	}

	return receiverWallets, nil
}

// RetryInvitationSMS sets null the invitation_sent_at of a receiver wallet.
func (rw *ReceiverWalletModel) RetryInvitationSMS(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletId string) (*ReceiverWallet, error) {
	var receiverWallet ReceiverWallet
	query := `
		UPDATE 
			receiver_wallets rw
		SET 
			invitation_sent_at = NULL
		WHERE rw.id = $1
		AND rw.status = 'READY'
		RETURNING 
			rw.id,
			rw.receiver_id as "receiver.id",
			rw.wallet_id as "wallet.id",
			rw.status,
			rw.invitation_sent_at,
			rw.created_at,
			rw.updated_at
	`

	err := sqlExec.GetContext(ctx, &receiverWallet, query, receiverWalletId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("updating receiver wallet: %w", err)
	}

	return &receiverWallet, nil
}

func (rw *ReceiverWalletModel) UpdateInvitationSentAt(ctx context.Context, sqlExec db.SQLExecuter, receiverWalletID ...string) ([]ReceiverWallet, error) {
	const query = `
		UPDATE
			receiver_wallets
		SET
			invitation_sent_at = NOW()
		WHERE
			id = ANY($1)
			AND status = $2 -- 'READY'::receiver_wallet_status
		RETURNING
			id, COALESCE(stellar_address, '') AS stellar_address, COALESCE(stellar_memo, '') AS stellar_memo,
			COALESCE(stellar_memo_type, '') AS stellar_memo_type, status, status_history,
			COALESCE(otp, '') AS otp, otp_confirmed_at, COALESCE(anchor_platform_transaction_id, '') AS anchor_platform_transaction_id,
			anchor_platform_transaction_synced_at, invitation_sent_at
	`

	var receiverWallets []ReceiverWallet
	err := sqlExec.SelectContext(ctx, &receiverWallets, query, pq.Array(receiverWalletID), ReadyReceiversWalletStatus)
	if err != nil {
		return nil, fmt.Errorf("updating invitation sent at: %w", err)
	}

	return receiverWallets, nil
}
