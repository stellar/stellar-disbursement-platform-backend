package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var (
	ErrCreateWalletInvalidStatus = fmt.Errorf("wallet status is not pending for token")
	ErrInvalidToken              = fmt.Errorf("token does not exist")
	ErrMissingToken              = fmt.Errorf("token is required")
	ErrMissingPublicKey          = fmt.Errorf("public key is required")
)

//go:generate mockery --name=EmbeddedWalletServiceInterface --case=underscore --structname=MockEmbeddedWalletService --filename=embedded_wallet_service.go
type EmbeddedWalletServiceInterface interface {
	// CreateInvitationToken creates a new embedded wallet invitation token
	CreateInvitationToken(ctx context.Context) (string, error)
	// CreateWallet creates a new embedded wallet using the provided token and public key
	CreateWallet(ctx context.Context, token, publicKey string) error
	// GetWallet retrieves an embedded wallet by token
	GetWallet(ctx context.Context, token string) (*data.EmbeddedWallet, error)
}

var _ EmbeddedWalletServiceInterface = (*EmbeddedWalletService)(nil)

// EmbeddedWalletService handles wallet creation and transaction sponsorship
type EmbeddedWalletService struct {
	sdpModels *data.Models
	tssModel  *store.TransactionModel
	wasmHash  string
}

func NewEmbeddedWalletService(sdpModels *data.Models, tssModel *store.TransactionModel, wasmHash string) (*EmbeddedWalletService, error) {
	if sdpModels == nil {
		return nil, fmt.Errorf("sdpModels cannot be nil")
	}
	if tssModel == nil {
		return nil, fmt.Errorf("tssModel cannot be nil")
	}
	if wasmHash == "" {
		return nil, fmt.Errorf("wasmHash cannot be empty")
	}

	return &EmbeddedWalletService{
		sdpModels: sdpModels,
		tssModel:  tssModel,
		wasmHash:  wasmHash,
	}, nil
}

type EmbeddedWalletServiceOptions struct {
	MTNDBConnectionPool db.DBConnectionPool
	TSSDBConnectionPool db.DBConnectionPool
	WasmHash            string
}

func (e *EmbeddedWalletService) CreateInvitationToken(ctx context.Context) (string, error) {
	token := uuid.New().String()

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (string, error) {
		insert := data.EmbeddedWalletInsert{
			Token:        token,
			WasmHash:     e.wasmHash,
			WalletStatus: data.PendingWalletStatus,
		}

		embeddedWallet, err := e.sdpModels.EmbeddedWallets.Insert(ctx, e.sdpModels.DBConnectionPool, insert)
		if err != nil {
			return "", fmt.Errorf("creating embedded wallet invitation token: %w", err)
		}

		return embeddedWallet.Token, nil
	})
}

func (e *EmbeddedWalletService) CreateWallet(ctx context.Context, token, publicKey string) error {
	if token == "" {
		return ErrMissingToken
	}
	if publicKey == "" {
		return ErrMissingPublicKey
	}

	currentTenant, err := tenant.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context: %w", err)
	}

	return db.RunInTransaction(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, e.tssModel.DBConnectionPool, nil, func(tssTx db.DBTransaction) error {
			embeddedWallet, err := e.sdpModels.EmbeddedWallets.GetByToken(ctx, dbTx, token)
			if err != nil {
				if errors.Is(err, data.ErrRecordNotFound) {
					return ErrInvalidToken
				}
				return fmt.Errorf("getting wallet by token %s: %w", token, err)
			}

			if embeddedWallet.WalletStatus != data.PendingWalletStatus {
				return ErrCreateWalletInvalidStatus
			}

			// Create the wallet transaction in TSS DB first
			tssTransaction := &store.Transaction{
				ExternalID:      embeddedWallet.Token,
				TransactionType: store.TransactionTypeWalletCreation,
				TenantID:        currentTenant.ID,
				WalletCreation: store.WalletCreation{
					PublicKey: publicKey,
					WasmHash:  e.wasmHash,
				},
			}

			_, err = e.tssModel.Insert(ctx, *tssTransaction)
			if err != nil {
				return fmt.Errorf("creating wallet transaction in TSS: %w", err)
			}

			embeddedWalletUpdate := data.EmbeddedWalletUpdate{
				WasmHash:     e.wasmHash,
				WalletStatus: data.ProcessingWalletStatus,
			}

			if err := e.sdpModels.EmbeddedWallets.Update(ctx, dbTx, embeddedWallet.Token, embeddedWalletUpdate); err != nil {
				return fmt.Errorf("updating embedded wallet %s: %w", embeddedWallet.Token, err)
			}

			return nil
		})
	})
}

func (e *EmbeddedWalletService) GetWallet(ctx context.Context, token string) (*data.EmbeddedWallet, error) {
	if token == "" {
		return nil, ErrMissingToken
	}

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.EmbeddedWallet, error) {
		embeddedWallet, err := e.sdpModels.EmbeddedWallets.GetByToken(ctx, dbTx, token)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, ErrInvalidToken
			}
			return nil, fmt.Errorf("getting wallet by token %s: %w", token, err)
		}
		return embeddedWallet, nil
	})
}
