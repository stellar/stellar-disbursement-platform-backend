package services

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

//go:generate mockery --name=EmbeddedWalletServiceInterface --case=underscore --structname=MockEmbeddedWalletService --filename=embedded_wallet_service.go
type EmbeddedWalletServiceInterface interface {
	QueueAccountCreation(ctx context.Context, tenantID, externalID, credentialID, publicKey string) error
}

var _ EmbeddedWalletServiceInterface = (*EmbeddedWalletService)(nil)

type EmbeddedWalletServiceOptions struct {
	MTNDBConnectionPool db.DBConnectionPool
	TSSDBConnectionPool db.DBConnectionPool
}

type EmbeddedWalletService struct {
	sdpModels *data.Models // TODO: store the account model here and update the status
	tssModel  *store.TransactionModel
}

func NewEmbeddedWalletService(sdpModels *data.Models, tssModel *store.TransactionModel) *EmbeddedWalletService {
	return &EmbeddedWalletService{
		sdpModels: sdpModels,
		tssModel:  tssModel,
	}
}

func (s *EmbeddedWalletService) QueueAccountCreation(ctx context.Context, tenantID, externalID, credentialID, publicKey string) error {
	transaction := store.Transaction{
		ExternalID:      externalID,
		TransactionType: store.TransactionTypeWalletCreation,
		WalletCreation: store.WalletCreation{
			CredentialID: credentialID,
			PublicKey:    publicKey,
			WasmHash:     "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50",
		},
		TenantID: tenantID,
	}

	_, err := s.tssModel.Insert(ctx, transaction)
	if err != nil {
		return fmt.Errorf("inserting transaction: %w", err)
	}

	return nil
}
