package services

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

var (
	ErrCreateWalletInvalidToken  = fmt.Errorf("invalid wallet token")
	ErrCreateWalletInvalidStatus = fmt.Errorf("wallet status is not pending for token")
	ErrGetWalletInvalidToken     = fmt.Errorf("invalid wallet token")
	ErrGetWalletMismatchedTenant = fmt.Errorf("tenant ID does not match the wallet's tenant ID")
	ErrMissingToken              = fmt.Errorf("token is required")
	ErrMissingPublicKey          = fmt.Errorf("public key is required")
)

//go:generate mockery --name=EmbeddedWalletServiceInterface --case=underscore --structname=MockEmbeddedWalletService --filename=embedded_wallet_service.go
type EmbeddedWalletServiceInterface interface {
	// CreateWallet creates a new embedded wallet using the provided token and public key
	CreateWallet(ctx context.Context, tenantID, token, publicKey string) error
	// GetWallet retrieves an embedded wallet by token
	GetWallet(ctx context.Context, tenantID, token string) (*data.EmbeddedWallet, error)
}

var _ EmbeddedWalletServiceInterface = (*EmbeddedWalletService)(nil)

// EmbeddedWalletService handles wallet creation and transaction sponsorship
type EmbeddedWalletService struct {
	sdpModels *data.Models
	tssModel  *store.TransactionModel
	wasmHash  string
}

func NewEmbeddedWalletService(sdpModels *data.Models, tssModel *store.TransactionModel, wasmHash string) *EmbeddedWalletService {
	return &EmbeddedWalletService{
		sdpModels: sdpModels,
		tssModel:  tssModel,
		wasmHash:  wasmHash,
	}
}

type EmbeddedWalletServiceOptions struct {
	MTNDBConnectionPool db.DBConnectionPool
	TSSDBConnectionPool db.DBConnectionPool
	WasmHash            string
}

func validateToken(token string) error {
	if token == "" {
		return ErrMissingToken
	}
	return nil
}

func (e *EmbeddedWalletService) CreateWallet(ctx context.Context, tenantID, token, publicKey string) error {
	// Validate inputs
	if err := validateToken(token); err != nil {
		return err
	}
	if publicKey == "" {
		return ErrMissingPublicKey
	}

	return db.RunInTransaction(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		embeddedWallet, err := e.sdpModels.EmbeddedWallets.GetByToken(ctx, dbTx, token)
		if err != nil {
			if err == data.ErrRecordNotFound {
				return ErrCreateWalletInvalidToken
			}
			return fmt.Errorf("getting wallet by token %s: %w", token, err)
		}

		if embeddedWallet.WalletStatus != data.PendingWalletStatus {
			return ErrCreateWalletInvalidStatus
		}

		embeddedWalletUpdate := data.EmbeddedWalletUpdate{
			WasmHash:     e.wasmHash,
			WalletStatus: data.SuccessWalletStatus,
		}

		if err := e.sdpModels.EmbeddedWallets.Update(ctx, dbTx, embeddedWallet.Token, embeddedWalletUpdate); err != nil {
			return fmt.Errorf("updating embedded wallet %s: %w", embeddedWallet.Token, err)
		}

		// TODO: Create the wallet transaction in TSS DB

		return nil
	})
}

func (e *EmbeddedWalletService) GetWallet(ctx context.Context, tenantID, token string) (*data.EmbeddedWallet, error) {
	if err := validateToken(token); err != nil {
		return nil, err
	}

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.EmbeddedWallet, error) {
		embeddedWallet, err := e.sdpModels.EmbeddedWallets.GetByToken(ctx, dbTx, token)
		if err != nil {
			if err == data.ErrRecordNotFound {
				return nil, ErrGetWalletInvalidToken
			}
			return nil, fmt.Errorf("getting wallet by token %s: %w", token, err)
		}
		if embeddedWallet.TenantID != tenantID {
			return nil, ErrGetWalletMismatchedTenant
		}
		return embeddedWallet, nil
	})
}
