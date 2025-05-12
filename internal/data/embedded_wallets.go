package data

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

type CreationStatus string

const (
	PendingCreationStatus CreationStatus = "PENDING"
	SuccessCreationStatus CreationStatus = "SUCCESS"
	FailedCreationStatus  CreationStatus = "FAILED"
)

func (status CreationStatus) Validate() error {
	uppercaseStatus := CreationStatus(strings.TrimSpace(strings.ToUpper(string(status))))
	if slices.Contains(CreationStatuses(), uppercaseStatus) {
		return nil
	}

	return fmt.Errorf("invalid creation status: %s", status)
}

func CreationStatuses() []CreationStatus {
	return []CreationStatus{
		PendingCreationStatus,
		SuccessCreationStatus,
		FailedCreationStatus,
	}
}

type EmbeddedWalletModel struct {
	dbConnectionPool db.DBConnectionPool
}

type EmbeddedWallet struct {
	ID         string         `json:"id" db:"id"`
	Status     CreationStatus `json:"status" db:"status"`
	CreatedAt  time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at" db:"updated_at"`
	ContractID string         `json:"contract_id" db:"contract_id"`
	WasmHash   string         `json:"wasm_hash" db:"wasm_hash"`
}

type EmbeddedWalletInsert struct {
	ID string `db:"id"`
}

type EmbeddedWalletUpdate struct {
	Status     CreationStatus `db:"status"`
	ContractID string         `db:"contract_id"`
	WasmHash   string         `db:"wasm_hash"`
}

func (w *EmbeddedWalletInsert) Validate() error {
	if w.ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	return nil
}

func (w *EmbeddedWalletUpdate) Validate() error {
	if err := w.Status.Validate(); err != nil {
		return fmt.Errorf("status is invalid: %w", err)
	}
	if w.ContractID == "" {
		return fmt.Errorf("contract ID cannot be empty")
	}
	if w.WasmHash == "" {
		return fmt.Errorf("wasm hash cannot be empty")
	}
	return nil
}
