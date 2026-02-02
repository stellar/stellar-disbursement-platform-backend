package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
)

var (
	ErrCreateWalletInvalidStatus = fmt.Errorf("wallet status is not pending for token")
	ErrInvalidToken              = fmt.Errorf("token does not exist")
	ErrMissingToken              = fmt.Errorf("token is required")
	ErrMissingPublicKey          = fmt.Errorf("public key is required")
	ErrMissingCredentialID       = fmt.Errorf("credential ID is required")
	ErrInvalidCredentialID       = fmt.Errorf("credential ID does not exist")
	ErrCredentialIDAlreadyExists = fmt.Errorf("credential ID already exists")
	ErrMissingContractAddress    = fmt.Errorf("contract address is required")

	// Sponsored transaction errors
	ErrMissingAccount      = fmt.Errorf("account is required")
	ErrMissingOperationXDR = fmt.Errorf("operation XDR is required")
)

//go:generate mockery --name=EmbeddedWalletServiceInterface --case=underscore --structname=MockEmbeddedWalletService --filename=embedded_wallet_service.go
type EmbeddedWalletServiceInterface interface {
	// CreateInvitationToken creates a new embedded wallet invitation token
	CreateInvitationToken(ctx context.Context) (string, error)
	// CreateWallet creates a new embedded wallet using the provided token, public key and credential ID
	CreateWallet(ctx context.Context, token, publicKey, credentialID string) error
	// GetWalletByCredentialID retrieves an embedded wallet by credential ID
	GetWalletByCredentialID(ctx context.Context, credentialID string) (*data.EmbeddedWallet, error)
	// GetPendingDisbursementAsset fetches the asset tied to a pending disbursement for the provided contract address
	GetPendingDisbursementAsset(ctx context.Context, contractAddress string) (*data.Asset, error)
	// IsVerificationPending returns true when the receiver wallet requires verification
	IsVerificationPending(ctx context.Context, contractAddress string) (bool, error)
	// GetReceiverContact retrieves the receiver contact info for an embedded wallet contract address
	GetReceiverContact(ctx context.Context, contractAddress string) (*data.Receiver, error)
	// SponsorTransaction sponsors a transaction on behalf of the embedded wallet
	SponsorTransaction(ctx context.Context, account, operationXDR string) (string, error)
	// GetTransactionStatus retrieves a sponsored transaction by ID for the provided account
	GetTransactionStatus(ctx context.Context, account, transactionID string) (*data.SponsoredTransaction, error)
}

var _ EmbeddedWalletServiceInterface = (*EmbeddedWalletService)(nil)

// EmbeddedWalletService handles wallet creation and transaction sponsorship
type EmbeddedWalletService struct {
	sdpModels *data.Models
	wasmHash  string
	rpcClient stellar.RPCClient
}

func NewEmbeddedWalletService(sdpModels *data.Models, wasmHash string, rpcClient stellar.RPCClient) (*EmbeddedWalletService, error) {
	if sdpModels == nil {
		return nil, fmt.Errorf("sdpModels cannot be nil")
	}
	if wasmHash == "" {
		return nil, fmt.Errorf("wasmHash cannot be empty")
	}
	if rpcClient == nil {
		return nil, fmt.Errorf("rpcClient cannot be nil")
	}

	return &EmbeddedWalletService{
		sdpModels: sdpModels,
		wasmHash:  wasmHash,
		rpcClient: rpcClient,
	}, nil
}

type EmbeddedWalletServiceOptions struct {
	MTNDBConnectionPool db.DBConnectionPool
	WasmHash            string
	RPCClient           stellar.RPCClient
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

	return db.RunInTransaction(ctx, e.sdpModels.DBConnectionPool, nil, func(sdpTx db.DBTransaction) error {
		embeddedWallet, err := e.sdpModels.EmbeddedWallets.GetByToken(ctx, sdpTx, token)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return ErrInvalidToken
			}
			return fmt.Errorf("getting wallet by token %s: %w", token, err)
		}

		if embeddedWallet.WalletStatus != data.PendingWalletStatus {
			return ErrCreateWalletInvalidStatus
		}

		credentialWallet, err := e.sdpModels.EmbeddedWallets.GetByCredentialID(ctx, sdpTx, credentialID)
		if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
			return fmt.Errorf("checking credential ID availability: %w", err)
		}
		if err == nil && credentialWallet != nil && credentialWallet.Token != embeddedWallet.Token {
			return ErrCredentialIDAlreadyExists
		}

		embeddedWalletUpdate := data.EmbeddedWalletUpdate{
			WasmHash:     e.wasmHash,
			CredentialID: credentialID,
			PublicKey:    publicKey,
		}

		if err := e.sdpModels.EmbeddedWallets.Update(ctx, sdpTx, embeddedWallet.Token, embeddedWalletUpdate); err != nil {
			if errors.Is(err, data.ErrEmbeddedWalletCredentialIDAlreadyExists) {
				return ErrCredentialIDAlreadyExists
			}
			return fmt.Errorf("updating embedded wallet %s: %w", embeddedWallet.Token, err)
		}

		return nil
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

func (e *EmbeddedWalletService) getReceiverWalletByContractAddress(ctx context.Context, contractAddress string) (*data.ReceiverWallet, error) {
	contractAddress = strings.TrimSpace(contractAddress)
	if contractAddress == "" {
		return nil, ErrMissingContractAddress
	}

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.ReceiverWallet, error) {
		receiverWallet, err := e.sdpModels.EmbeddedWallets.GetReceiverWallet(ctx, dbTx, contractAddress)
		if err != nil {
			return nil, fmt.Errorf("getting receiver wallet by contract address %s: %w", contractAddress, err)
		}
		return receiverWallet, nil
	})
}

func (e *EmbeddedWalletService) GetPendingDisbursementAsset(ctx context.Context, contractAddress string) (*data.Asset, error) {
	contractAddress = strings.TrimSpace(contractAddress)
	if contractAddress == "" {
		return nil, ErrMissingContractAddress
	}

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.Asset, error) {
		asset, err := e.sdpModels.EmbeddedWallets.GetPendingDisbursementAsset(ctx, dbTx, contractAddress)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("getting pending disbursement asset for %s: %w", contractAddress, err)
		}
		return asset, nil
	})
}

func (e *EmbeddedWalletService) IsVerificationPending(ctx context.Context, contractAddress string) (bool, error) {
	contractAddress = strings.TrimSpace(contractAddress)
	if contractAddress == "" {
		return false, ErrMissingContractAddress
	}

	receiverWallet, err := e.getReceiverWalletByContractAddress(ctx, contractAddress)
	if err != nil {
		return false, err
	}

	return receiverWallet.Status == data.ReadyReceiversWalletStatus, nil
}

func (e *EmbeddedWalletService) GetReceiverContact(ctx context.Context, contractAddress string) (*data.Receiver, error) {
	contractAddress = strings.TrimSpace(contractAddress)
	if contractAddress == "" {
		return nil, ErrMissingContractAddress
	}

	receiverWallet, err := e.getReceiverWalletByContractAddress(ctx, contractAddress)
	if err != nil {
		return nil, err
	}

	receiver, err := e.sdpModels.Receiver.Get(ctx, e.sdpModels.DBConnectionPool, receiverWallet.Receiver.ID)
	if err != nil {
		return nil, fmt.Errorf("getting receiver %s: %w", receiverWallet.Receiver.ID, err)
	}

	return receiver, nil
}

func (e *EmbeddedWalletService) SponsorTransaction(ctx context.Context, account, operationXDR string) (string, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return "", ErrMissingAccount
	}
	if operationXDR == "" {
		return "", ErrMissingOperationXDR
	}

	if err := e.simulateSponsoredTransaction(ctx, operationXDR); err != nil {
		return "", fmt.Errorf("simulating sponsored transaction: %w", err)
	}

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(sdpTx db.DBTransaction) (string, error) {
		insert := data.SponsoredTransactionInsert{
			ID:           uuid.New().String(),
			Account:      account,
			OperationXDR: operationXDR,
			Status:       data.PendingSponsoredTransactionStatus,
		}
		sponsoredTx, err := e.sdpModels.SponsoredTransactions.Insert(ctx, sdpTx, insert)
		if err != nil {
			return "", fmt.Errorf("creating sponsored transaction: %w", err)
		}

		return sponsoredTx.ID, nil
	})
}

func (e *EmbeddedWalletService) GetTransactionStatus(ctx context.Context, account, transactionID string) (*data.SponsoredTransaction, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, ErrMissingAccount
	}
	if transactionID == "" {
		return nil, fmt.Errorf("transaction ID is required")
	}

	return db.RunInTransactionWithResult(ctx, e.sdpModels.DBConnectionPool, nil, func(sdpTx db.DBTransaction) (*data.SponsoredTransaction, error) {
		sponsoredTx, err := e.sdpModels.SponsoredTransactions.GetByIDAndAccount(ctx, sdpTx, transactionID, account)
		if err != nil {
			return nil, fmt.Errorf("getting sponsored transaction by ID %s: %w", transactionID, err)
		}

		return sponsoredTx, nil
	})
}

func (e *EmbeddedWalletService) simulateSponsoredTransaction(ctx context.Context, operationXDR string) error {
	tenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting tenant from context: %w", err)
	}
	if tenant.DistributionAccountAddress == nil {
		return fmt.Errorf("distribution account address is missing for tenant")
	}
	distributionAccount := strings.TrimSpace(*tenant.DistributionAccountAddress)
	if distributionAccount == "" {
		return fmt.Errorf("distribution account address is missing for tenant")
	}
	if !strkey.IsValidEd25519PublicKey(distributionAccount) {
		return fmt.Errorf("distribution account address is not a valid ed25519 public key")
	}

	var operation xdr.InvokeHostFunctionOp
	err = xdr.SafeUnmarshalBase64(operationXDR, &operation)
	if err != nil {
		return fmt.Errorf("decoding operation XDR: %w", err)
	}

	if operation.HostFunction.Type != xdr.HostFunctionTypeHostFunctionTypeInvokeContract {
		return fmt.Errorf("operation is not an invoke contract host function")
	}
	if operation.HostFunction.InvokeContract == nil {
		return fmt.Errorf("invoke contract operation is missing contract details")
	}

	sponsoredOperation := &txnbuild.InvokeHostFunction{
		SourceAccount: distributionAccount,
		HostFunction:  operation.HostFunction,
		Auth:          operation.Auth,
	}

	txParams := txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: distributionAccount,
			Sequence:  0,
		},
		Operations: []txnbuild.Operation{sponsoredOperation},
		BaseFee:    txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimeout(300),
		},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return fmt.Errorf("building simulation transaction: %w", err)
	}

	txEnvelope, err := tx.Base64()
	if err != nil {
		return fmt.Errorf("encoding simulation transaction: %w", err)
	}

	if _, simErr := e.rpcClient.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{
		Transaction: txEnvelope,
		AuthMode:    protocol.AuthModeEnforce,
	}); simErr != nil {
		return simErr
	}

	return nil
}
