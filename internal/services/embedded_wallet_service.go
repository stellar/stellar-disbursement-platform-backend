package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

var (
	ErrCreateWalletInvalidStatus = fmt.Errorf("wallet status is not pending for token")
	ErrInvalidToken              = fmt.Errorf("token does not exist")
	ErrMissingToken              = fmt.Errorf("token is required")
	ErrMissingPublicKey          = fmt.Errorf("public key is required")
	ErrMissingCredentialID       = fmt.Errorf("credential ID is required")
	ErrInvalidCredentialID       = fmt.Errorf("credential ID does not exist")
	ErrCredentialIDAlreadyExists = fmt.Errorf("credential ID already exists")
)

//go:generate mockery --name=EmbeddedWalletServiceInterface --case=underscore --structname=MockEmbeddedWalletService --filename=embedded_wallet_service.go
type EmbeddedWalletServiceInterface interface {
	// CreateInvitationToken creates a new embedded wallet invitation token
	CreateInvitationToken(ctx context.Context) (string, error)
	// CreateWallet creates a new embedded wallet using the provided token, public key and credential ID
	CreateWallet(ctx context.Context, token, publicKey, credentialID string) error
	// GetWalletByCredentialID retrieves an embedded wallet by credential ID
	GetWalletByCredentialID(ctx context.Context, credentialID string) (*data.EmbeddedWallet, error)
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

func (e *EmbeddedWalletService) CreateWallet(ctx context.Context, token, publicKey, credentialID string) error {
	if token == "" {
		return ErrMissingToken
	}
	if publicKey == "" {
		return ErrMissingPublicKey
	}
	if credentialID == "" {
		return ErrMissingCredentialID
	}

	currentTenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context: %w", err)
	}

	_, err = e.sdpModels.EmbeddedWallets.GetByCredentialID(ctx, e.sdpModels.DBConnectionPool, credentialID)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return fmt.Errorf("checking credential ID availability: %w", err)
	}
	if err == nil {
		return ErrCredentialIDAlreadyExists
	}

	return db.RunInTransaction(ctx, e.tssModel.DBConnectionPool, nil, func(tssTx db.DBTransaction) error {
		embeddedWallet, err := e.sdpModels.EmbeddedWallets.GetByToken(ctx, e.sdpModels.DBConnectionPool, token)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return ErrInvalidToken
			}
			return fmt.Errorf("getting wallet by token %s: %w", token, err)
		}

		if embeddedWallet.WalletStatus != data.PendingWalletStatus {
			return ErrCreateWalletInvalidStatus
		}

		tssTransaction := &store.Transaction{
			ExternalID:      embeddedWallet.Token,
			TransactionType: store.TransactionTypeWalletCreation,
			TenantID:        currentTenant.ID,
			WalletCreation: store.WalletCreation{
				PublicKey: publicKey,
				WasmHash:  e.wasmHash,
			},
		}

		_, err = e.tssModel.BulkInsert(ctx, tssTx, []store.Transaction{*tssTransaction})
		if err != nil {
			return fmt.Errorf("creating wallet transaction in TSS: %w", err)
		}

		return db.RunInTransaction(ctx, e.sdpModels.DBConnectionPool, nil, func(sdpTx db.DBTransaction) error {
			embeddedWalletUpdate := data.EmbeddedWalletUpdate{
				WasmHash:     e.wasmHash,
				CredentialID: credentialID,
				PublicKey:    publicKey,
				WalletStatus: data.ProcessingWalletStatus,
			}

			if err := e.sdpModels.EmbeddedWallets.Update(ctx, sdpTx, embeddedWallet.Token, embeddedWalletUpdate); err != nil {
				if errors.Is(err, data.ErrEmbeddedWalletCredentialIDAlreadyExists) {
					return ErrCredentialIDAlreadyExists
				}
				return fmt.Errorf("updating embedded wallet %s: %w", embeddedWallet.Token, err)
			}

			return nil
		})
	})
}

func (e *EmbeddedWalletService) GetWalletByCredentialID(ctx context.Context, credentialID string) (*data.EmbeddedWallet, error) {
	if credentialID == "" {
		return nil, ErrMissingCredentialID
	}

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.EmbeddedWallet, error) {
		embeddedWallet, err := e.sdpModels.EmbeddedWallets.GetByCredentialID(ctx, dbTx, credentialID)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, ErrInvalidCredentialID
			}
			return nil, fmt.Errorf("getting wallet by credential ID %s: %w", credentialID, err)
		}
		return embeddedWallet, nil
	})
}
