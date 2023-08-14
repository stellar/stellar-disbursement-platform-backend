package store

import (
	"context"
	"fmt"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

var ErrInsuficientChannelAccounts = fmt.Errorf("there are no channel accounts available to process transactions")

// ChannelTransactionBundle is an abstraction that aggregates a bundle of a ChannelAccount and a Transaction. It is used
// to prepare the resources for the workers, locking both the Transaction (the job) and the ChannelAccount (the
// resource), and then updating the lock according with the parameters provided.
type ChannelTransactionBundle struct {
	// ChannelAccount is the resource needed to process the Transaction.
	ChannelAccount ChannelAccount `db:"channel_account"`
	// Transaction is the job that would be handled by the worker.
	Transaction Transaction `db:"transaction"`
	// LockedUntilLedgerNumber is the ledger number until which both the transaction and channel account are locked.
	LockedUntilLedgerNumber int `db:"locked_until_ledger_number"`
}

type ChannelTransactionBundleModel struct {
	dbConnectionPool    db.DBConnectionPool
	channelAccountModel *ChannelAccountModel
	transactionModel    *TransactionModel
}

func NewChannelTransactionBundleModel(dbConnectionPool db.DBConnectionPool) (*ChannelTransactionBundleModel, error) {
	if dbConnectionPool == nil {
		return nil, fmt.Errorf("dbConnectionPool cannot be nil")
	}

	return &ChannelTransactionBundleModel{
		dbConnectionPool:    dbConnectionPool,
		channelAccountModel: &ChannelAccountModel{DBConnectionPool: dbConnectionPool},
		transactionModel:    NewTransactionModel(dbConnectionPool),
	}, nil
}

// LoadAndLockTuples loads a slice of ChannelTransactionBundle from the database, and locks them until the given ledger
// number, up to the amount of transactions specified by the {limit} parameter. It returns the
// ErrInsuficientChannelAccounts error if there are transactions to process but no channel accounts available.
func (m *ChannelTransactionBundleModel) LoadAndLockTuples(ctx context.Context, currentLedgerNumber, lockToLedgerNumber, limit int) ([]*ChannelTransactionBundle, error) {
	if limit < 1 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}

	if lockToLedgerNumber <= currentLedgerNumber {
		return nil, fmt.Errorf("lockToLedgerNumber must be greater than currentLedgerNumber")
	}

	return db.RunInTransactionWithResult(ctx, m.dbConnectionPool, nil, func(dbTx db.DBTransaction) ([]*ChannelTransactionBundle, error) {
		// STEP 1: get transactions available to be processed:
		q := fmt.Sprintf(`
			SELECT
				*
			FROM
				submitter_transactions
			WHERE
				%s
				AND synced_at IS NULL
				AND status = ANY($1)
			ORDER BY
				updated_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		`, m.transactionModel.queryFilterForLockedState(false, int32(currentLedgerNumber)),
		)
		var unlockedTransactions []Transaction
		allowedTxStatuses := []TransactionStatus{TransactionStatusPending, TransactionStatusProcessing}
		err := dbTx.SelectContext(ctx, &unlockedTransactions, q, pq.Array(allowedTxStatuses), limit)
		if err != nil {
			return nil, fmt.Errorf("fetching unlocked transactions: %w", err)
		}
		if len(unlockedTransactions) == 0 {
			return nil, nil
		}

		// STEP 2: get channel accounts available to process the transactions:
		q = fmt.Sprintf(`
			SELECT
				*
			FROM
				channel_accounts
			WHERE
				%s
			ORDER BY
				updated_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
			`, m.channelAccountModel.queryFilterForLockedState(false, int32(currentLedgerNumber)),
		)
		var unlockedChannelAccounts []ChannelAccount
		err = dbTx.SelectContext(ctx, &unlockedChannelAccounts, q, len(unlockedTransactions))
		if err != nil {
			return nil, fmt.Errorf("calculating amount ov available channel accounts: %w", err)
		}
		if len(unlockedChannelAccounts) == 0 {
			return nil, ErrInsuficientChannelAccounts
		}

		// STEP 3: lock channel accounts and transactions, and build the bundle slice:
		bundleLen := len(unlockedChannelAccounts)
		bundles := make([]*ChannelTransactionBundle, bundleLen)
		for i := 0; i < bundleLen; i++ {
			chAcc := &unlockedChannelAccounts[i]
			var lockedChAcc *ChannelAccount
			lockedChAcc, err = m.channelAccountModel.Lock(ctx, dbTx, chAcc.PublicKey, int32(currentLedgerNumber), int32(lockToLedgerNumber))
			if err != nil {
				return nil, fmt.Errorf("locking channel account %q: %w", chAcc.PublicKey, err)
			}

			tx := &unlockedTransactions[i]
			var lockedTx *Transaction
			lockedTx, err = m.transactionModel.Lock(ctx, dbTx, tx.ID, int32(currentLedgerNumber), int32(lockToLedgerNumber))
			if err != nil {
				return nil, fmt.Errorf("locking transaction %q: %w", tx.ID, err)
			}

			bundles[i] = &ChannelTransactionBundle{
				ChannelAccount:          *lockedChAcc,
				Transaction:             *lockedTx,
				LockedUntilLedgerNumber: lockToLedgerNumber,
			}
		}

		return bundles, nil
	})
}
