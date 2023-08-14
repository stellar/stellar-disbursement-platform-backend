package store

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

//go:generate mockery --name=ChannelAccountStore --case=underscore --structname=MockChannelAccountStore
type ChannelAccountStore interface {
	Delete(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) (err error)
	DeleteIfLockedUntil(ctx context.Context, publicKey string, lockedUntilLedgerNumber int) (err error)
	Get(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, currentLedgerNumber int) (ca *ChannelAccount, err error)
	GetAndLock(ctx context.Context, publicKey string, currentLedger, nextLedgerLock int) (*ChannelAccount, error)
	Count(ctx context.Context) (count int, err error)
	GetAll(ctx context.Context, sqlExec db.SQLExecuter, currentLedger, limit int) ([]*ChannelAccount, error)
	GetAndLockAll(ctx context.Context, currentLedger, nextLedgerLock, limit int) ([]*ChannelAccount, error)
	Insert(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, privateKey string) error
	InsertAndLock(ctx context.Context, publicKey string, privateKey string, currentLedger, nextLedgerLock int) error
	BatchInsert(ctx context.Context, sqlExec db.SQLExecuter, channelAccounts []*ChannelAccount) error
	BatchInsertAndLock(ctx context.Context, channelAccounts []*ChannelAccount, currentLedger, nextLedgerLock int) error
	// Lock management:
	Lock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, currentLedger, nextLedgerLock int32) (*ChannelAccount, error)
	Unlock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) (*ChannelAccount, error)
}

//go:generate mockery --name=TransactionStore --case=underscore --structname=MockTransactionStore
type TransactionStore interface {
	// CRUD:
	Insert(ctx context.Context, tx Transaction) (*Transaction, error)
	BulkInsert(ctx context.Context, sqlExec db.SQLExecuter, transactions []Transaction) ([]Transaction, error)
	Get(ctx context.Context, txID string) (tx *Transaction, err error)
	GetAllByPaymentIDs(ctx context.Context, paymentIDs []string) (transactions []*Transaction, err error)
	// Status & Lock management:
	UpdateStatusToSuccess(ctx context.Context, tx Transaction) (updatedTx *Transaction, err error)
	UpdateStatusToError(ctx context.Context, tx Transaction, message string) (updatedTx *Transaction, err error)
	UpdateStellarTransactionXDRReceived(ctx context.Context, txID string, xdrReceived string) (*Transaction, error)
	UpdateStellarTransactionHashAndXDRSent(ctx context.Context, txID string, txHash, txXDRSent string) (*Transaction, error)
	Lock(ctx context.Context, sqlExec db.SQLExecuter, transactionID string, currentLedger, nextLedgerLock int32) (*Transaction, error)
	Unlock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) (*Transaction, error)
	// Queue management:
	PrepareTransactionForReprocessing(ctx context.Context, sqlExec db.SQLExecuter, transactionID string) (*Transaction, error)
	GetTransactionBatchForUpdate(ctx context.Context, dbTx db.DBTransaction, batchSize int) (transactions []*Transaction, err error)
	UpdateSyncedTransactions(ctx context.Context, dbTx db.DBTransaction, txIDs []string) error
}
