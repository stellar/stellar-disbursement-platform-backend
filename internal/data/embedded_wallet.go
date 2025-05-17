package data

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

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

func (status EmbeddedWalletStatus) Validate() error {
	switch EmbeddedWalletStatus(strings.ToUpper(string(status))) {
	case PendingWalletStatus, SuccessWalletStatus, ProcessingWalletStatus, FailedWalletStatus:
		return nil
	default:
		return fmt.Errorf("invalid embedded wallet status %q", status)
	}
}

type EmbeddedWallet struct {
	Token           string               `json:"token" db:"token"`
	TenantID        string               `json:"tenant_id" db:"tenant_id"`
	WasmHash        string               `json:"wasm_hash" db:"wasm_hash"`
	ContractAddress string               `json:"contract_address" db:"contract_address"`
	CreatedAt       *time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt       *time.Time           `json:"updated_at" db:"updated_at"`
	WalletStatus    EmbeddedWalletStatus `json:"wallet_status" db:"wallet_status"`
}

type EmbeddedWalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

func EmbeddWalletColumnNames(tableReference, resultAlias string) string {
	columns := SQLColumnConfig{
		TableReference: tableReference,
		ResultAlias:    resultAlias,
		RawColumns: []string{
			"token",
			"tenant_id",
			"created_at",
			"updated_at",
			"wallet_status",
		},
		CoalesceColumns: []string{
			"wasm_hash",
			"contract_address",
		},
	}.Build()

	return strings.Join(columns, ", ")
}

func (ew *EmbeddedWalletModel) GetByToken(ctx context.Context, sqlExec db.SQLExecuter, token string) (*EmbeddedWallet, error) {
	query := fmt.Sprintf(`
        SELECT
            %s  -- NO COMMA HERE
        FROM embedded_wallets ew
        WHERE
            ew.token = $1
        `, EmbeddWalletColumnNames("ew", ""))

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

type EmbeddedWalletUpdate struct {
	WasmHash        string
	ContractAddress string
	WalletStatus    EmbeddedWalletStatus
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

	return nil
}

func (ew *EmbeddedWalletModel) Update(ctx context.Context, sqlExec db.SQLExecuter, token string, update EmbeddedWalletUpdate) error {
	if err := update.Validate(); err != nil {
		return fmt.Errorf("validating embedded wallet update: %w", err)
	}

	fields := []string{}
	args := []any{}

	if update.WasmHash != "" {
		fields = append(fields, "wasm_hash = ?")
		args = append(args, update.WasmHash)
	}
	if update.ContractAddress != "" {
		fields = append(fields, "contract_address = ?")
		args = append(args, update.ContractAddress)
	}
	if update.WalletStatus != "" {
		fields = append(fields, "wallet_status = ?")
		args = append(args, update.WalletStatus)
	}

	args = append(args, token)
	query := fmt.Sprintf(`
				UPDATE embedded_wallets
				SET %s
				WHERE token = ?
		`, strings.Join(fields, ", "))

	query = sqlExec.Rebind(query)
	result, err := sqlExec.ExecContext(ctx, query, args...)
	if err != nil {
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
