package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var ErrRecordNotFound = errors.New("record not found")

type TransactionType string

const (
	TransactionTypePayment        TransactionType = "PAYMENT"
	TransactionTypeWalletCreation TransactionType = "WALLET_CREATION"
	TransactionTypeSponsored      TransactionType = "SPONSORED"
)

type Transaction struct {
	ID string `db:"id"`
	// ExternalID contains an external ID for the transaction. This is used for reconciliation.
	ExternalID string `db:"external_id"`
	// Status is the status of the transaction. Don't change it directly and use the internal methods of the model instead.
	Status        TransactionStatus        `db:"status"`
	StatusMessage sql.NullString           `db:"status_message"`
	StatusHistory TransactionStatusHistory `db:"status_history"`

	TransactionType TransactionType `db:"transaction_type"`
	Payment
	WalletCreation
	Sponsored

	TenantID            string         `db:"tenant_id"`
	DistributionAccount sql.NullString `db:"distribution_account"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
	// StartedAt is when the transaction was read from the queue into memory.
	StartedAt *time.Time `db:"started_at"`
	// SentAt is when the transaction was sent to the Stellar network.
	SentAt *time.Time `db:"sent_at"`
	// CompletedAt is when the transaction reached a terminal state, either SUCCESS or ERROR.
	CompletedAt *time.Time `db:"completed_at"`
	// SyncedAt is when the transaction was synced with SDP.
	SyncedAt *time.Time `db:"synced_at"`

	AttemptsCount          int            `db:"attempts_count"`
	StellarTransactionHash sql.NullString `db:"stellar_transaction_hash"`
	// XDRSent is the EnvelopeXDR submitted when creating a Stellar transaction in the network.
	XDRSent sql.NullString `db:"xdr_sent"`
	// XDRReceived is the ResultXDR received from the Stellar network when attempting to create a transaction.
	XDRReceived sql.NullString `db:"xdr_received"`
	LockedAt    *time.Time     `db:"locked_at"`
	// LockedUntilLedgerNumber is the ledger number after which the lock expires. It should be synched with the
	// expiration ledger bound set in the Stellar transaction submitted to the blockchain, and the same value in the
	// namesake column of the channel account model.
	LockedUntilLedgerNumber sql.NullInt32 `db:"locked_until_ledger_number"`
}

type Payment struct {
	AssetCode   string          `db:"asset_code"`
	AssetIssuer string          `db:"asset_issuer"`
	Amount      float64         `db:"amount"`
	Destination string          `db:"destination"`
	Memo        string          `db:"memo"`
	MemoType    schema.MemoType `db:"memo_type"`
}

type WalletCreation struct {
	PublicKey string `db:"public_key"`
	WasmHash  string `db:"wasm_hash"`
}

type Sponsored struct {
	SponsoredAccount      string `db:"sponsored_account"`
	SponsoredOperationXDR string `db:"sponsored_operation_xdr"`
}

func (tx *Transaction) BuildMemo() (txnbuild.Memo, error) {
	if tx.TransactionType != TransactionTypePayment {
		return nil, fmt.Errorf("transaction type %q does not support memo", tx.TransactionType)
	}

	memo, err := schema.NewMemo(tx.MemoType, tx.Memo)
	if err != nil {
		return nil, fmt.Errorf("building memo: %w", err)
	}
	return memo, nil
}

func (tx *Transaction) IsLocked(currentLedgerNumber int32) bool {
	return tx.LockedUntilLedgerNumber.Valid && currentLedgerNumber <= tx.LockedUntilLedgerNumber.Int32
}

// validate checks if the transaction fields are valid and can be added to the DB.
func (tx *Transaction) validate() error {
	if tx.ExternalID == "" {
		return fmt.Errorf("external ID is required")
	}

	if tx.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	switch tx.TransactionType {
	case TransactionTypePayment:
		if err := tx.Payment.validate(); err != nil {
			return fmt.Errorf("validating payment transaction: %w", err)
		}
	case TransactionTypeWalletCreation:
		if err := tx.WalletCreation.validate(); err != nil {
			return fmt.Errorf("validating wallet creation transaction: %w", err)
		}
	case TransactionTypeSponsored:
		if err := tx.Sponsored.validate(); err != nil {
			return fmt.Errorf("validating sponsored transaction: %w", err)
		}
	default:
		return fmt.Errorf("invalid transaction type %q", tx.TransactionType)
	}
	return nil
}

// validateWalletCreation checks if the transaction fields are valid and can be added to the DB.
func (wc *WalletCreation) validate() error {
	if wc.PublicKey == "" {
		return fmt.Errorf("public key is required")
	} else {
		_, err := hex.DecodeString(wc.PublicKey)
		if err != nil {
			return fmt.Errorf("public key %q is not a valid hex string: %w", wc.PublicKey, err)
		}
	}
	if wc.WasmHash == "" {
		return fmt.Errorf("wasm hash is required")
	} else {
		_, err := hex.DecodeString(wc.WasmHash)
		if err != nil {
			return fmt.Errorf("wasm hash %q is not a valid hex string: %w", wc.WasmHash, err)
		}
	}

	return nil
}

func (p *Payment) validate() error {
	if len(p.AssetCode) < 1 || len(p.AssetCode) > 12 {
		return fmt.Errorf("asset code must have between 1 and 12 characters")
	}
	if strings.ToLower(p.AssetCode) != "xlm" {
		if p.AssetIssuer == "" {
			return fmt.Errorf("asset issuer is required")
		}

		if !strkey.IsValidEd25519PublicKey(p.AssetIssuer) {
			return fmt.Errorf("asset issuer %q is not a valid ed25519 public key", p.AssetIssuer)
		}
	}
	if p.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if !strkey.IsValidEd25519PublicKey(p.Destination) && !strkey.IsValidContractAddress(p.Destination) {
		return fmt.Errorf("destination %q is not a valid ed25519 public key or contract address", p.Destination)
	}
	return nil
}

func (s *Sponsored) validate() error {
	if s.SponsoredAccount == "" {
		return fmt.Errorf("sponsored account is required")
	}
	if !strkey.IsValidContractAddress(s.SponsoredAccount) {
		return fmt.Errorf("sponsored account %q is not a valid contract address", s.SponsoredAccount)
	}

	if s.SponsoredOperationXDR == "" {
		return fmt.Errorf("sponsored operation XDR is required")
	}
	var operation xdr.InvokeHostFunctionOp
	if err := xdr.SafeUnmarshalBase64(s.SponsoredOperationXDR, &operation); err != nil {
		return fmt.Errorf("invalid sponsored operation XDR %q: %w", s.SponsoredOperationXDR, err)
	}
	return nil
}

type TransactionModel struct {
	DBConnectionPool db.DBConnectionPool
}

func NewTransactionModel(dbConnectionPool db.DBConnectionPool) *TransactionModel {
	return &TransactionModel{DBConnectionPool: dbConnectionPool}
}

func TransactionColumnNames(tableReference, resultAlias string) string {
	columns := data.SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		RawColumns: []string{
			"id",
			"external_id",
			"tenant_id",
			"transaction_type",
			"distribution_account",
			"status",
			"status_message",
			"status_history",
			"attempts_count",
			"stellar_transaction_hash",
			"xdr_sent",
			"xdr_received",
			"created_at",
			"updated_at",
			"started_at",
			"sent_at",
			"completed_at",
			"synced_at",
			"locked_at",
			"locked_until_ledger_number",
		},
		CoalesceStringColumns: []string{
			"asset_code",
			"asset_issuer",
			"destination",
			"public_key",
			"wasm_hash",
			"sponsored_account",
			"sponsored_operation_xdr",
			"memo",
			"memo_type::text",
		},
		CoalesceFloat64Columns: []string{
			"amount",
		},
	}.Build()

	return strings.Join(columns, ",\n")
}

// Insert adds a new Transaction to the database.
func (t *TransactionModel) Insert(ctx context.Context, tx Transaction) (*Transaction, error) {
	transactions, err := t.BulkInsert(ctx, t.DBConnectionPool, []Transaction{tx})
	if err != nil {
		return nil, fmt.Errorf("inserting single transaction: %w", err)
	}

	return &transactions[0], nil
}

// BulkInsert adds a batch of Transactions to the database and returns the inserted transactions.
func (t *TransactionModel) BulkInsert(ctx context.Context, sqlExec db.SQLExecuter, transactions []Transaction) ([]Transaction, error) {
	if len(transactions) == 0 {
		return nil, nil
	}

	var queryBuilder strings.Builder
	queryBuilder.WriteString("INSERT INTO submitter_transactions (transaction_type, external_id, asset_code, asset_issuer, amount, destination, public_key, wasm_hash, sponsored_account, sponsored_operation_xdr, tenant_id, memo, memo_type) VALUES ")
	valueStrings := make([]string, 0, len(transactions))
	valueArgs := make([]interface{}, 0, len(transactions)*13)

	for _, transaction := range transactions {
		if err := transaction.validate(); err != nil {
			return nil, fmt.Errorf("validating transaction for insertion: %w", err)
		}
		valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		valueArgs = append(valueArgs,
			transaction.TransactionType,
			transaction.ExternalID,
			sdpUtils.SQLNullString(transaction.AssetCode),
			sdpUtils.SQLNullString(transaction.AssetIssuer),
			sdpUtils.SQLNullFloat64(transaction.Amount),
			sdpUtils.SQLNullString(transaction.Destination),
			sdpUtils.SQLNullString(transaction.PublicKey),
			sdpUtils.SQLNullString(transaction.WasmHash),
			sdpUtils.SQLNullString(transaction.SponsoredAccount),
			sdpUtils.SQLNullString(transaction.SponsoredOperationXDR),
			transaction.TenantID,
			sdpUtils.SQLNullString(transaction.Memo),
			sdpUtils.SQLNullString(string(transaction.MemoType)),
		)
	}

	var insertedTransctions []Transaction
	queryBuilder.WriteString(strings.Join(valueStrings, ", "))
	queryBuilder.WriteString(" RETURNING " + TransactionColumnNames("", ""))
	query := sqlExec.Rebind(queryBuilder.String())
	err := sqlExec.SelectContext(ctx, &insertedTransctions, query, valueArgs...)
	if err != nil {
		return nil, fmt.Errorf("inserting transactions: %w", err)
	}

	return insertedTransctions, nil
}

// Get gets a Transaction from the database.
func (t *TransactionModel) Get(ctx context.Context, txID string) (*Transaction, error) {
	var transaction Transaction
	q := `
		SELECT
			` + TransactionColumnNames("", "") + `
		FROM
			submitter_transactions t
		WHERE
			t.id = $1
		`
	err := t.DBConnectionPool.GetContext(ctx, &transaction, q, txID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error querying transaction ID %s: %w", txID, err)
	}
	return &transaction, nil
}

func (t *TransactionModel) GetAllByExternalIDs(ctx context.Context, externalIDs []string) ([]*Transaction, error) {
	var transactions []*Transaction
	q := `
		SELECT
			` + TransactionColumnNames("", "") + `
		FROM
			submitter_transactions t
		WHERE
			t.external_id = ANY($1)
		`
	err := t.DBConnectionPool.SelectContext(ctx, &transactions, q, pq.Array(externalIDs))
	if err != nil {
		return nil, fmt.Errorf("error querying transactions: %w", err)
	}
	return transactions, nil
}

// UpdateStatusToSuccess updates a Transaction's status to SUCCESS. Only succeeds if the current status is PROCESSING.
func (t *TransactionModel) UpdateStatusToSuccess(ctx context.Context, tx Transaction) (*Transaction, error) {
	// verify if this state transition is valid:
	err := tx.Status.CanTransitionTo(TransactionStatusSuccess)
	if err != nil {
		return nil, fmt.Errorf("attempting to transition transaction status to TransactionStatusSuccess: %w", err)
	}

	var updatedTx Transaction
	query := `
			UPDATE
				submitter_transactions
			SET
				status = $1,
				completed_at = NOW(),
				status_history = array_append(status_history, create_submitter_transactions_status_history(NOW(), $1::transaction_status, NULL, stellar_transaction_hash, xdr_sent, xdr_received))
			WHERE
				id = $2
			RETURNING
				` + TransactionColumnNames("", "")
	err = t.DBConnectionPool.GetContext(ctx, &updatedTx, query, TransactionStatusSuccess, tx.ID)
	if err != nil {
		return nil, fmt.Errorf("updating transaction status to TransactionStatusSuccess: %w", err)
	}

	return &updatedTx, nil
}

// UpdateStatusToError updates a Transaction's status to ERROR. Only succeeds if the current status is PROCESSING.
func (t *TransactionModel) UpdateStatusToError(ctx context.Context, tx Transaction, message string) (*Transaction, error) {
	// verify if this state transition is valid:
	err := tx.Status.CanTransitionTo(TransactionStatusError)
	if err != nil {
		return nil, fmt.Errorf("attempting to transition transaction status to TransactionStatusError: %w", err)
	}

	var updatedTx Transaction
	query := `
			UPDATE
				submitter_transactions
			SET
				status = $1,
				completed_at = NOW(),
				status_message = $2,
				status_history = array_append(status_history, create_submitter_transactions_status_history(NOW(), $1::transaction_status, $2::text, stellar_transaction_hash, xdr_sent, xdr_received))
			WHERE
				id = $3
			RETURNING
				` + TransactionColumnNames("", "")
	err = t.DBConnectionPool.GetContext(ctx, &updatedTx, query, TransactionStatusError, message, tx.ID)
	if err != nil {
		return nil, fmt.Errorf("updating transaction status to TransactionStatusError: %w", err)
	}

	return &updatedTx, nil
}

func (t *TransactionModel) UpdateStellarTransactionHashXDRSentAndDistributionAccount(ctx context.Context, txID string, txHash, txXDRSent, distributionAccount string) (*Transaction, error) {
	if len(txHash) != 64 {
		return nil, fmt.Errorf("invalid transaction hash %q", txHash)
	}

	if !strkey.IsValidEd25519PublicKey(distributionAccount) {
		return nil, fmt.Errorf("distribution account %q is not a valid ed25519 public key", distributionAccount)
	}

	var txEnvelope xdr.TransactionEnvelope
	err := xdr.SafeUnmarshalBase64(txXDRSent, &txEnvelope)
	if err != nil {
		return nil, fmt.Errorf("invalid XDR envelope: %w", err)
	}

	query := `
		UPDATE
			submitter_transactions 
		SET 
			stellar_transaction_hash = $1::text,
			xdr_sent = $2,
			distribution_account = $3,
			sent_at = NOW(),
			status_history = array_append(status_history, create_submitter_transactions_status_history(NOW(), status, 'Updating Stellar Transaction Hash, XDR Sent and Distribution Account', $1::text, $2, xdr_received)),
			attempts_count = attempts_count + 1
		WHERE 
			id = $4
		RETURNING
			` + TransactionColumnNames("", "")
	var tx Transaction
	err = t.DBConnectionPool.GetContext(ctx, &tx, query, txHash, txXDRSent, distributionAccount, txID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error updating transaction hash: %w", err)
	}

	return &tx, nil
}

// UpdateStellarTransactionXDRReceived updates a Transaction's XDR received.
func (t *TransactionModel) UpdateStellarTransactionXDRReceived(ctx context.Context, txID string, xdrReceived string) (*Transaction, error) {
	var txResult xdr.TransactionResult
	err := xdr.SafeUnmarshalBase64(xdrReceived, &txResult)
	if err != nil {
		return nil, fmt.Errorf("invalid XDR result: %w", err)
	}

	query := `
		UPDATE
			submitter_transactions 
		SET 
			xdr_received = $1,
			status_history = array_append(status_history, create_submitter_transactions_status_history(NOW(), status, 'Updating XDR Received', stellar_transaction_hash, xdr_sent, $1::text))
		WHERE 
			id = $2
		RETURNING
			` + TransactionColumnNames("", "")
	var updatedTx Transaction
	err = t.DBConnectionPool.GetContext(ctx, &updatedTx, query, xdrReceived, txID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("error updating transaction hash: %w", err)
	}

	return &updatedTx, nil
}

// GetTransactionBatchForUpdate returns a batch of transactions that are ready to be synced. Locks the rows for update.
// Only returns transactions of the specified transaction type.
func (t *TransactionModel) GetTransactionBatchForUpdate(ctx context.Context, dbTx db.DBTransaction, batchSize int, tenantID string, transactionType TransactionType) ([]*Transaction, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be greater than 0")
	}
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	transactions := []*Transaction{}

	query := `
		SELECT 
		    ` + TransactionColumnNames("", "") + `
		FROM 
		    submitter_transactions
		WHERE 
		    status IN ('SUCCESS', 'ERROR')
		    AND synced_at IS NULL
		    AND tenant_id = $1
		    AND transaction_type = $2
		ORDER BY 
		    completed_at ASC
		LIMIT 
		    $3
		FOR UPDATE SKIP LOCKED
		`

	err := dbTx.SelectContext(ctx, &transactions, query, tenantID, transactionType, batchSize)
	if err != nil {
		return nil, fmt.Errorf("getting transactions: %w", err)
	}

	return transactions, nil
}

func (t *TransactionModel) GetTransactionPendingUpdateByID(ctx context.Context, dbTx db.SQLExecuter, txID string, expectedTransactionType TransactionType) (*Transaction, error) {
	query := `
		SELECT 
			` + TransactionColumnNames("", "") + `
		FROM 
			submitter_transactions
		WHERE
			id = $1
			AND status IN ('SUCCESS', 'ERROR')
			AND synced_at IS NULL
			AND transaction_type = $2
		FOR UPDATE SKIP LOCKED
	`

	var tx Transaction
	err := dbTx.GetContext(ctx, &tx, query, txID, expectedTransactionType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("getting transaction ID %s: %w", txID, err)
	}

	return &tx, nil
}

// UpdateSyncedTransactions updates the synced_at field for the given transaction IDs. Returns an error if the number of
// updated rows is not equal to the number of provided transaction IDs.
func (t *TransactionModel) UpdateSyncedTransactions(ctx context.Context, dbTx db.SQLExecuter, txIDs []string) error {
	if len(txIDs) == 0 {
		return fmt.Errorf("no transaction IDs provided")
	}

	query := `
		UPDATE 
		    submitter_transactions
		SET 
		    synced_at = NOW()
		WHERE 
		    id = ANY($1)
			AND status = ANY($2)
		`

	allowedStatuses := []TransactionStatus{TransactionStatusSuccess, TransactionStatusError}
	result, err := dbTx.ExecContext(ctx, query, pq.Array(txIDs), pq.Array(allowedStatuses))
	if err != nil {
		return fmt.Errorf("updating transactions: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected != int64(len(txIDs)) {
		return fmt.Errorf("expected %d rows to be affected, got %d", len(txIDs), rowsAffected)
	}

	return nil
}

// queryFilterForLockedState returns a SQL query filter that can be used to filter transactions based on their locked
// state.
func (t *TransactionModel) queryFilterForLockedState(locked bool, ledgerNumber int32) string {
	if locked {
		return fmt.Sprintf("(locked_until_ledger_number >= %d)", ledgerNumber)
	}
	return fmt.Sprintf("(locked_until_ledger_number IS NULL OR locked_until_ledger_number < %d)", ledgerNumber)
}

// Lock locks the transaction with the provided transactionID. It returns a ErrRecordNotFound error if you try to lock a
// transaction that is already locked.
func (t *TransactionModel) Lock(ctx context.Context, sqlExec db.SQLExecuter, transactionID string, currentLedger, nextLedgerLock int32) (*Transaction, error) {
	q := fmt.Sprintf(`
		UPDATE
			submitter_transactions
		SET
			locked_at = NOW(),
			locked_until_ledger_number = $1,
			status = $2
		WHERE
			id = $3
			AND %s
			AND synced_at IS NULL
			AND status = ANY($4)
		RETURNING
			`+TransactionColumnNames("", ""),
		t.queryFilterForLockedState(false, currentLedger),
	)
	var transaction Transaction
	allowedTxStatuses := []TransactionStatus{TransactionStatusPending, TransactionStatusProcessing}
	err := sqlExec.GetContext(ctx, &transaction, q, nextLedgerLock, TransactionStatusProcessing, transactionID, pq.Array(allowedTxStatuses))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("locking transaction %q: %w", transactionID, err)
	}

	return &transaction, nil
}

// Unlock lifts the lock from the transactionID with the provided publicKey.
func (t *TransactionModel) Unlock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) (*Transaction, error) {
	q := `
		UPDATE
			submitter_transactions
		SET
			locked_at = NULL,
			locked_until_ledger_number = NULL
		WHERE
			id = $1
		RETURNING
			` + TransactionColumnNames("", "")
	var transaction Transaction
	err := sqlExec.GetContext(ctx, &transaction, q, publicKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("unlocking transaction %q: %w", publicKey, err)
	}

	return &transaction, nil
}

// PrepareTransactionForReprocessing pushes the transaction with the provided transactionID back to the queue.
func (t *TransactionModel) PrepareTransactionForReprocessing(ctx context.Context, sqlExec db.SQLExecuter, transactionID string) (*Transaction, error) {
	q := `
		UPDATE
			submitter_transactions
		SET
			locked_at = NULL,
			locked_until_ledger_number = NULL,
			stellar_transaction_hash = NULL,
			xdr_sent = NULL,
			xdr_received = NULL
		WHERE
			id = $1
			AND synced_at IS NULL
			AND status = ANY($2)
		RETURNING
			` + TransactionColumnNames("", "")
	var transaction Transaction
	allowedTxStatuses := []TransactionStatus{TransactionStatusPending, TransactionStatusProcessing}
	err := sqlExec.GetContext(ctx, &transaction, q, transactionID, pq.Array(allowedTxStatuses))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("pushing transaction back to queue %q: %w", transactionID, err)
	}

	return &transaction, nil
}

var _ TransactionStore = &TransactionModel{}

type TransactionStatusHistoryEntry struct {
	Status                 string    `json:"status"`
	StatusMessage          string    `json:"status_message"`
	Timestamp              time.Time `json:"timestamp"`
	StellarTransactionHash string    `json:"stellar_transaction_hash"`
	XDRSent                string    `json:"xdr_sent"`
	XDRReceived            string    `json:"xdr_received"`
}

type TransactionStatusHistory []TransactionStatusHistoryEntry

// Value implements the driver.Valuer interface.
func (tsh TransactionStatusHistory) Value() (driver.Value, error) {
	var statusHistoryJSON []string
	for _, sh := range tsh {
		shJSONBytes, err := json.Marshal(sh)
		if err != nil {
			return nil, fmt.Errorf("error converting status history to json for transaction: %w", err)
		}
		statusHistoryJSON = append(statusHistoryJSON, string(shJSONBytes))
	}

	return pq.Array(statusHistoryJSON).Value()
}

// Scan implements the sql.Scanner interface.
func (tsh *TransactionStatusHistory) Scan(src interface{}) error {
	var statusHistoryJSON []string
	if err := pq.Array(&statusHistoryJSON).Scan(src); err != nil {
		return fmt.Errorf("error scanning status history value: %w", err)
	}

	for _, sh := range statusHistoryJSON {
		var shEntry TransactionStatusHistoryEntry
		err := json.Unmarshal([]byte(sh), &shEntry)
		if err != nil {
			return fmt.Errorf("error unmarshaling status_history column: %w", err)
		}
		*tsh = append(*tsh, shEntry)
	}

	return nil
}

var (
	_ sql.Scanner   = (*TransactionStatusHistory)(nil)
	_ driver.Valuer = (*TransactionStatusHistory)(nil)
)
