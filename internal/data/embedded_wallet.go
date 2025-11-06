package data

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

var ErrEmbeddedWalletCredentialIDAlreadyExists = errors.New("an embedded wallet with this credential ID already exists")

type EmbeddedWalletStatus string

const (
	// The token has been created but the wallet has not been created yet.
	PendingWalletStatus EmbeddedWalletStatus = "PENDING"
	// The wallet creation is in progress.
	ProcessingWalletStatus EmbeddedWalletStatus = "PROCESSING"
	// The wallet has been created and is ready to use.
	SuccessWalletStatus EmbeddedWalletStatus = "SUCCESS"
	// The wallet creation failed.
	FailedWalletStatus EmbeddedWalletStatus = "FAILED"
)

const (
	// WebAuthn credential IDs can be up to 1023 bytes, and after URL encoding they can be longer.
	// We set this to 2048 to provide sufficient buffer for URL encoding and future compatibility.
	MaxCredentialIDLength = 2048
)

func (status EmbeddedWalletStatus) Validate() error {
	switch EmbeddedWalletStatus(strings.ToUpper(string(status))) {
	case PendingWalletStatus, SuccessWalletStatus, ProcessingWalletStatus, FailedWalletStatus:
		return nil
	default:
		return fmt.Errorf("invalid embedded wallet status %q", status)
	}
}

type EmbeddedWallet struct {
	Token            string               `json:"token" db:"token"`
	WasmHash         string               `json:"wasm_hash" db:"wasm_hash"`
	ContractAddress  string               `json:"contract_address" db:"contract_address"`
	CredentialID     string               `json:"credential_id" db:"credential_id"`
	PublicKey        string               `json:"public_key" db:"public_key"`
	ReceiverWalletID string               `json:"receiver_wallet_id" db:"receiver_wallet_id"`
	CreatedAt        *time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt        *time.Time           `json:"updated_at" db:"updated_at"`
	WalletStatus     EmbeddedWalletStatus `json:"wallet_status" db:"wallet_status"`
}

type EmbeddedWalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

func EmbeddedWalletColumnNames(tableReference, resultAlias string) string {
	columns := SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		RawColumns: []string{
			"token",
			"created_at",
			"updated_at",
			"wallet_status",
		},
		CoalesceStringColumns: []string{
			"wasm_hash",
			"contract_address",
			"credential_id",
			"public_key",
			"receiver_wallet_id",
		},
	}.Build()

	return strings.Join(columns, ", ")
}

func (ew *EmbeddedWalletModel) GetByToken(ctx context.Context, sqlExec db.SQLExecuter, token string) (*EmbeddedWallet, error) {
	query := fmt.Sprintf(`
        SELECT
            %s
        FROM embedded_wallets ew
        WHERE
            ew.token = $1
        `, EmbeddedWalletColumnNames("ew", ""))

	var wallet EmbeddedWallet
	err := sqlExec.GetContext(ctx, &wallet, query, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("querying embedded wallet: %w", err)
	}

	return &wallet, nil
}

func (ew *EmbeddedWalletModel) GetByCredentialID(ctx context.Context, sqlExec db.SQLExecuter, credentialID string) (*EmbeddedWallet, error) {
	query := fmt.Sprintf(`
        SELECT
            %s
        FROM embedded_wallets ew
        WHERE
            ew.credential_id = $1
        `, EmbeddedWalletColumnNames("ew", ""))

	var wallet EmbeddedWallet
	err := sqlExec.GetContext(ctx, &wallet, query, credentialID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("querying embedded wallet by credential ID: %w", err)
	}

	return &wallet, nil
}

type EmbeddedWalletInsert struct {
	Token        string               `db:"token"`
	WasmHash     string               `db:"wasm_hash"`
	WalletStatus EmbeddedWalletStatus `db:"wallet_status"`
}

func (ewi EmbeddedWalletInsert) Validate() error {
	if ewi.Token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	if ewi.WasmHash == "" {
		return fmt.Errorf("wasm hash cannot be empty")
	} else {
		_, err := hex.DecodeString(ewi.WasmHash)
		if err != nil {
			return fmt.Errorf("invalid wasm hash")
		}
	}

	if err := ewi.WalletStatus.Validate(); err != nil {
		return fmt.Errorf("validating wallet status: %w", err)
	}

	return nil
}

func (ew *EmbeddedWalletModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, insert EmbeddedWalletInsert) (*EmbeddedWallet, error) {
	if err := insert.Validate(); err != nil {
		return nil, fmt.Errorf("validating embedded wallet insert: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO embedded_wallets (
			token,
			wasm_hash,
			wallet_status
		) VALUES (
			$1, $2, $3
		) RETURNING %s`, EmbeddedWalletColumnNames("", ""))

	var wallet EmbeddedWallet
	err := sqlExec.GetContext(ctx, &wallet, query,
		insert.Token,
		insert.WasmHash,
		insert.WalletStatus)
	if err != nil {
		return nil, fmt.Errorf("inserting embedded wallet: %w", err)
	}

	return &wallet, nil
}

type EmbeddedWalletUpdate struct {
	WasmHash         string               `db:"wasm_hash"`
	ContractAddress  string               `db:"contract_address"`
	CredentialID     string               `db:"credential_id"`
	PublicKey        string               `db:"public_key"`
	WalletStatus     EmbeddedWalletStatus `db:"wallet_status"`
	ReceiverWalletID string               `db:"receiver_wallet_id"`
}

func (ewu EmbeddedWalletUpdate) Validate() error {
	if utils.IsEmpty(ewu) {
		return fmt.Errorf("no values provided to update embedded wallet")
	}

	if ewu.WasmHash != "" {
		_, err := hex.DecodeString(ewu.WasmHash)
		if err != nil {
			return fmt.Errorf("invalid wasm hash")
		}
	}

	if ewu.ContractAddress != "" {
		if !strkey.IsValidContractAddress(ewu.ContractAddress) {
			return fmt.Errorf("invalid contract address")
		}
	}

	if ewu.WalletStatus != "" {
		if err := ewu.WalletStatus.Validate(); err != nil {
			return fmt.Errorf("validating wallet status: %w", err)
		}
	}

	if ewu.CredentialID != "" {
		if len(ewu.CredentialID) > MaxCredentialIDLength {
			return fmt.Errorf("credential ID must be %d characters or less, got %d characters", MaxCredentialIDLength, len(ewu.CredentialID))
		}
	}

	return nil
}

func (ew *EmbeddedWalletModel) Update(ctx context.Context, sqlExec db.SQLExecuter, token string, update EmbeddedWalletUpdate) error {
	if err := update.Validate(); err != nil {
		return fmt.Errorf("validating embedded wallet update: %w", err)
	}

	setClause, params := BuildSetClause(update)
	if setClause == "" {
		return fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
				UPDATE embedded_wallets
				SET %s
				WHERE token = ?
		`, setClause)
	params = append(params, token)
	query = sqlExec.Rebind(query)

	result, err := sqlExec.ExecContext(ctx, query, params...)
	if err != nil {
		var pqError *pq.Error
		if errors.As(err, &pqError) {
			if pqError.Code == "23505" && pqError.Constraint == "embedded_wallets_credential_id_key" {
				return ErrEmbeddedWalletCredentialIDAlreadyExists
			}
		}
		return fmt.Errorf("updating embedded wallet: %w", err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return fmt.Errorf("no embedded wallets could be found in UpdateEmbeddedWallet: %w", ErrRecordNotFound)
	}

	return nil
}
