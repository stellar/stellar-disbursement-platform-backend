package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go/hash"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

//go:generate mockery --name=WalletCreationFromSubmitterServiceInterface --case=snake --structname=MockWalletCreationFromSubmitterService

type WalletCreationFromSubmitterServiceInterface interface {
	SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error
}

// WalletCreationFromSubmitterService is a service that monitors TSS wallet creation transactions that were complete and sync their completion
// state with the SDP embedded wallets.
type WalletCreationFromSubmitterService struct {
	sdpModels         *data.Models
	tssModel          *store.TransactionModel
	networkPassphrase string
}

const autoRegistrationIdentifier = "AUTO_REGISTRATION"

var _ WalletCreationFromSubmitterServiceInterface = (*WalletCreationFromSubmitterService)(nil)

// NewWalletCreationFromSubmitterService creates a new instance of WalletCreationFromSubmitterService.
func NewWalletCreationFromSubmitterService(
	models *data.Models,
	tssDBConnectionPool db.DBConnectionPool,
	networkPassphrase string,
) *WalletCreationFromSubmitterService {
	return &WalletCreationFromSubmitterService{
		sdpModels:         models,
		tssModel:          store.NewTransactionModel(tssDBConnectionPool),
		networkPassphrase: networkPassphrase,
	}
}

// calculateContractAddress calculates the contract address for a wallet creation transaction based on the distribution account and public key.
//
// Contract addresses can be deterministically derived from the deployer account and an optional salt. In this case, we set the salt to the
// wallet's public key hash, and the deployer account is the distribution account from the TSS transaction.
//
// Read more: https://developers.stellar.org/docs/build/smart-contracts/example-contracts/deployer#how-it-works
func (s *WalletCreationFromSubmitterService) calculateContractAddress(
	distributionAccount, publicKey string,
) (string, error) {
	publicKeyBytes, err := hex.DecodeString(publicKey)
	if err != nil {
		return "", fmt.Errorf("decoding public key: %w", err)
	}

	publicKeyHash := hash.Hash(publicKeyBytes)
	salt := xdr.Uint256(publicKeyHash)

	rawAddress, err := strkey.Decode(strkey.VersionByteAccountID, distributionAccount)
	if err != nil {
		return "", fmt.Errorf("decoding distribution account address: %w", err)
	}
	var uint256Val xdr.Uint256
	copy(uint256Val[:], rawAddress)
	distributionAccountID := xdr.AccountId{
		Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
		Ed25519: &uint256Val,
	}

	distributionSCAddress := xdr.ScAddress{
		Type:      xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &distributionAccountID,
	}

	contractIDPreimage := xdr.ContractIdPreimage{
		Type: xdr.ContractIdPreimageTypeContractIdPreimageFromAddress,
		FromAddress: &xdr.ContractIdPreimageFromAddress{
			Address: distributionSCAddress,
			Salt:    salt,
		},
	}

	networkHash := hash.Hash([]byte(s.networkPassphrase))
	hashIDPreimage := xdr.HashIdPreimage{
		Type: xdr.EnvelopeTypeEnvelopeTypeContractId,
		ContractId: &xdr.HashIdPreimageContractId{
			NetworkId:          xdr.Hash(networkHash),
			ContractIdPreimage: contractIDPreimage,
		},
	}

	preimageXDR, err := hashIDPreimage.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshaling preimage: %w", err)
	}

	contractIDHash := hash.Hash(preimageXDR)
	contractAddress, err := strkey.Encode(strkey.VersionByteContract, contractIDHash[:])
	if err != nil {
		return "", fmt.Errorf("encoding contract address: %w", err)
	}

	return contractAddress, nil
}

// SyncBatchTransactions syncs a batch of completed TSS wallet creation transactions with the embedded wallet table
func (s *WalletCreationFromSubmitterService) SyncBatchTransactions(ctx context.Context, batchSize int, tenantID string) error {
	err := db.RunInTransaction(ctx, s.sdpModels.DBConnectionPool, nil, func(sdpDBTx db.DBTransaction) error {
		return db.RunInTransaction(ctx, s.tssModel.DBConnectionPool, nil, func(tssDBTx db.DBTransaction) error {
			transactions, err := s.tssModel.GetTransactionBatchForUpdate(ctx, tssDBTx, batchSize, tenantID, store.TransactionTypeWalletCreation)
			if err != nil {
				return fmt.Errorf("getting wallet creation transactions for update: %w", err)
			}
			return s.syncTransactions(ctx, sdpDBTx, tssDBTx, transactions)
		})
	})
	if err != nil {
		return fmt.Errorf("synchronizing wallet creations from submitter: %w", err)
	}

	return nil
}

// syncTransactions synchronizes TSS wallet creation transactions with the embedded wallet table.
// It should be called within a DB transaction. This method processes multiple transactions efficiently.
func (s *WalletCreationFromSubmitterService) syncTransactions(ctx context.Context, sdpDBTx, tssDBTx db.DBTransaction, transactions []*store.Transaction) error {
	if s.sdpModels == nil || s.tssModel == nil {
		return fmt.Errorf("WalletCreationFromSubmitterService sdpModels and tssModel cannot be nil")
	}

	if len(transactions) == 0 {
		log.Ctx(ctx).Debug("No wallet creation transactions to sync from submitter to SDP")
		return nil
	}

	// 1. Validate all transactions
	for _, transaction := range transactions {
		if transaction.Status == store.TransactionStatusSuccess {
			if !transaction.StellarTransactionHash.Valid {
				return fmt.Errorf("expected successful transaction %s to have a stellar transaction hash", transaction.ID)
			}
			if !transaction.DistributionAccount.Valid {
				return fmt.Errorf("expected successful transaction %s to have a distribution account", transaction.ID)
			}
		}
		if transaction.WalletCreation.PublicKey == "" {
			return fmt.Errorf("expected transaction %s to have a public key in wallet creation", transaction.ID)
		}
		if transaction.Status != store.TransactionStatusSuccess && transaction.Status != store.TransactionStatusError {
			return fmt.Errorf("transaction id %s is in an unexpected status %s", transaction.ID, transaction.Status)
		}
	}

	// 2. Sync embedded wallets with transactions
	transactionIDs := make([]string, 0, len(transactions))
	for _, transaction := range transactions {
		err := s.syncEmbeddedWalletWithTransaction(ctx, sdpDBTx, transaction)
		if err != nil {
			return fmt.Errorf("syncing embedded wallet for transaction ID %s: %w", transaction.ID, err)
		}
		transactionIDs = append(transactionIDs, transaction.ID)
	}

	// 3. Set synced_at for all synced wallet creation transactions
	err := s.tssModel.UpdateSyncedTransactions(ctx, tssDBTx, transactionIDs)
	if err != nil {
		return fmt.Errorf("updating transactions as synced: %w", err)
	}
	log.Ctx(ctx).Infof("Updated %d wallet creation transactions as synced", len(transactions))

	return nil
}

// syncEmbeddedWalletWithTransaction updates the embedded wallet based on the transaction status.
func (s *WalletCreationFromSubmitterService) syncEmbeddedWalletWithTransaction(ctx context.Context, sdpDBTx db.DBTransaction, transaction *store.Transaction) error {
	embeddedWallet, err := s.sdpModels.EmbeddedWallets.GetByToken(ctx, sdpDBTx, transaction.ExternalID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return fmt.Errorf("embedded wallet with token %s not found", transaction.ExternalID)
		}
		return fmt.Errorf("getting embedded wallet with token %s: %w", transaction.ExternalID, err)
	}

	update := data.EmbeddedWalletUpdate{}

	switch transaction.Status {
	case store.TransactionStatusSuccess:
		if !transaction.DistributionAccount.Valid {
			return fmt.Errorf("distribution account is not set for transaction %s", transaction.ID)
		}

		contractAddress, calcErr := s.calculateContractAddress(
			transaction.DistributionAccount.String,
			transaction.WalletCreation.PublicKey,
		)
		if calcErr != nil {
			return fmt.Errorf("calculating contract address for transaction %s: %w", transaction.ID, calcErr)
		}

		update.ContractAddress = contractAddress
		embeddedWallet.ContractAddress = contractAddress
		update.WalletStatus = data.SuccessWalletStatus
	case store.TransactionStatusError:
		update.WalletStatus = data.FailedWalletStatus
	default:
		return fmt.Errorf("transaction %s is not in a terminal state (status: %s)", transaction.ID, transaction.Status)
	}

	err = s.sdpModels.EmbeddedWallets.Update(ctx, sdpDBTx, embeddedWallet.Token, update)
	if err != nil {
		return fmt.Errorf("updating embedded wallet with token %s: %w", embeddedWallet.Token, err)
	}

	if transaction.Status == store.TransactionStatusSuccess && embeddedWallet.ReceiverWalletID != "" {
		if embeddedWallet.VerificationField == "" {
			if err := s.autoRegisterEmbeddedWallet(ctx, sdpDBTx, embeddedWallet); err != nil {
				return fmt.Errorf("auto registering receiver wallet %s: %w", embeddedWallet.ReceiverWalletID, err)
			} else {
				// TODO: update receiver wallet to READY and start opt verification.
				log.Ctx(ctx).Debugf("embedded wallet %s requires manual verification. Receiver wallet %s remains READY", embeddedWallet.Token, embeddedWallet.ReceiverWalletID)
			}
		}
	}

	return nil
}

func (s *WalletCreationFromSubmitterService) autoRegisterEmbeddedWallet(ctx context.Context, sdpDBTx db.DBTransaction, embeddedWallet *data.EmbeddedWallet) error {
	receiverWallet, err := s.sdpModels.ReceiverWallet.GetByID(ctx, sdpDBTx, embeddedWallet.ReceiverWalletID)
	if err != nil {
		return fmt.Errorf("getting embedded wallet %s: %w", embeddedWallet.ReceiverWalletID, err)
	}

	if receiverWallet.Status == data.RegisteredReceiversWalletStatus {
		return nil
	}

	if strings.TrimSpace(embeddedWallet.ContractAddress) == "" {
		return fmt.Errorf("embedded wallet %s missing contract address", embeddedWallet.Token)
	}

	now := time.Now()
	walletUpdate := data.ReceiverWalletUpdate{
		Status:           data.RegisteredReceiversWalletStatus,
		StellarAddress:   embeddedWallet.ContractAddress,
		OTPConfirmedAt:   now,
		OTPConfirmedWith: autoRegistrationIdentifier,
	}
	if err = s.sdpModels.ReceiverWallet.Update(ctx, receiverWallet.ID, walletUpdate, sdpDBTx); err != nil {
		return fmt.Errorf("updating receiver wallet %s: %w", receiverWallet.ID, err)
	}

	return nil
}
