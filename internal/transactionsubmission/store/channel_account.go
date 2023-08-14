package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
)

type ChannelAccount struct {
	PublicKey  string       `db:"public_key"`
	PrivateKey string       `db:"private_key"` // TODO: remove this from the model, since we now rely on a Signer interface.
	UpdatedAt  *time.Time   `db:"updated_at"`
	CreatedAt  *time.Time   `db:"created_at"`
	LockedAt   sql.NullTime `db:"locked_at"`
	// LockedUntilLedgerNumber is the ledger number after which the lock expires. It should be synched with the
	// expiration ledger bound of the transaction submitted by this Stellar channel account.
	LockedUntilLedgerNumber sql.NullInt32 `db:"locked_until_ledger_number"`
}

func (ca *ChannelAccount) IsLocked(currentLedgerNumber int32) bool {
	return ca.LockedUntilLedgerNumber.Valid && currentLedgerNumber <= ca.LockedUntilLedgerNumber.Int32
}

type ChannelAccountModel struct {
	DBConnectionPool db.DBConnectionPool
}

func NewChannelAccountModel(dbConnectionPool db.DBConnectionPool) *ChannelAccountModel {
	return &ChannelAccountModel{DBConnectionPool: dbConnectionPool}
}

// Insert inserts a (publicKey, privateKey) pair to the database.
func (ca *ChannelAccountModel) Insert(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, privateKey string) error {
	err := ca.BatchInsert(ctx, sqlExec, []*ChannelAccount{{PublicKey: publicKey, PrivateKey: privateKey}})
	if err != nil {
		return fmt.Errorf("inserting channel account %q: %w", publicKey, err)
	}

	return nil
}

// BatchInsert inserts a a batch of (publicKey, privateKey) pairs into the database.
func (ca *ChannelAccountModel) BatchInsert(ctx context.Context, sqlExec db.SQLExecuter, channelAccounts []*ChannelAccount) error {
	if len(channelAccounts) == 0 {
		return nil
	}

	publicKeys := make([]string, len(channelAccounts))
	privateKeys := make([]string, len(channelAccounts))

	for i, chAcc := range channelAccounts {
		if chAcc.PublicKey == "" {
			return fmt.Errorf("public key cannot be empty")
		}
		if chAcc.PrivateKey == "" {
			return fmt.Errorf("private key cannot be empty")
		}

		publicKeys[i] = chAcc.PublicKey
		privateKeys[i] = chAcc.PrivateKey
	}

	const q = `
	INSERT INTO 
		channel_accounts (public_key, private_key)
	SELECT * 
		FROM UNNEST($1::text[], $2::text[])
	`

	_, err := sqlExec.ExecContext(ctx, q, pq.Array(publicKeys), pq.Array(privateKeys))
	if err != nil {
		return fmt.Errorf("inserting channel accounts: %w", err)
	}

	return nil
}

// InsertAndLock insert an account keypair into the database and locks it until some future ledger.
func (ca *ChannelAccountModel) InsertAndLock(ctx context.Context, publicKey string, privateKey string, currentLedger, nextLedgerLock int) error {
	return db.RunInTransaction(ctx, ca.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		err := ca.Insert(ctx, dbTx, publicKey, privateKey)
		if err != nil {
			return fmt.Errorf("cannot insert account %s: %w", publicKey, err)
		}

		_, err = ca.Lock(ctx, dbTx, publicKey, int32(currentLedger), int32(nextLedgerLock))
		if err != nil {
			return fmt.Errorf("cannot lock account %s: %w", publicKey, err)
		}

		return nil
	})
}

// BatchInsertAndLock inserts a batch of account keypairs into the database and locks them until some future ledger.
func (ca *ChannelAccountModel) BatchInsertAndLock(ctx context.Context, channelAccounts []*ChannelAccount, currentLedger, nextLedgerLock int) error {
	return db.RunInTransaction(ctx, ca.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		err := ca.BatchInsert(ctx, dbTx, channelAccounts)
		if err != nil {
			return fmt.Errorf("cannot insert batch insert %d accounts: %w", len(channelAccounts), err)
		}

		for _, account := range channelAccounts {
			_, err = ca.Lock(ctx, dbTx, account.PublicKey, int32(currentLedger), int32(nextLedgerLock))
			if err != nil {
				return fmt.Errorf("cannot lock account %s: %w", account.PublicKey, err)
			}
		}

		return nil
	})
}

// Get retrieves the channel account with the given public key from the database if account is not locked or `currentLedgerNumber` is
// ahead of the ledger number the account has been locked to.
func (ca *ChannelAccountModel) Get(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, currentLedgerNumber int) (*ChannelAccount, error) {
	query := `
		SELECT
			*
		FROM
			channel_accounts 
		WHERE
			public_key = $1%s
		FOR UPDATE SKIP LOCKED
		`

	if currentLedgerNumber > 0 {
		query = fmt.Sprintf(query, "\nAND "+ca.queryFilterForLockedState(false, int32(currentLedgerNumber)))
	} else if currentLedgerNumber == 0 {
		// bypass locked until ledger check for read-only purposes such as retrieving the keypair for signing
		query = fmt.Sprintf(query, "")
	} else {
		return nil, fmt.Errorf("invalid ledger number %d", currentLedgerNumber)
	}

	var channelAccount ChannelAccount
	err := sqlExec.GetContext(ctx, &channelAccount, query, publicKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("could not find channel account %q: %w", publicKey, ErrRecordNotFound)
		}
		return nil, fmt.Errorf("querying for channel account %q: %w", publicKey, err)
	}

	return &channelAccount, nil
}

// GetAndLock retrieves the channel account with the given public key from the database and locks the account until some future ledger.
func (ca *ChannelAccountModel) GetAndLock(ctx context.Context, publicKey string, currentLedger, nextLedgerLock int) (*ChannelAccount, error) {
	channelAccount, err := ca.Get(ctx, ca.DBConnectionPool, publicKey, currentLedger)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve account %s: %w", publicKey, err)
	}

	lockedAccount, err := ca.Lock(ctx, ca.DBConnectionPool, channelAccount.PublicKey, int32(currentLedger), int32(nextLedgerLock))
	if err != nil {
		return nil, fmt.Errorf("cannot lock account %s: %w", channelAccount.PublicKey, err)
	}

	return lockedAccount, nil
}

// Count retrieves the current count of channel accounts in the database.
func (ca *ChannelAccountModel) Count(ctx context.Context) (int, error) {
	query := `
		SELECT
			COUNT(*)
		FROM
			channel_accounts 
		`

	var count int
	err := ca.DBConnectionPool.GetContext(ctx, &count, query)
	if err != nil {
		return 0, fmt.Errorf("counting channel accounts: %w", err)
	}

	return count, nil
}

// GetAll all channel accounts from the database, respecting the limit provided for accounts that are not locked or `currentLedgerNumber` is
// ahead of the ledger number each account has been locked to.
func (ca *ChannelAccountModel) GetAll(ctx context.Context, sqlExec db.SQLExecuter, currentLedgerNumber, limit int) ([]*ChannelAccount, error) {
	baseQuery := `
		SELECT
			* 
		FROM 
			channel_accounts%s
		FOR UPDATE SKIP LOCKED
		`

	if currentLedgerNumber > 0 {
		baseQuery = fmt.Sprintf(baseQuery, "\nWHERE"+ca.queryFilterForLockedState(false, int32(currentLedgerNumber)))
	} else if currentLedgerNumber == 0 {
		// bypass locked until ledger check for read-only purposes such as retrieving the keypair for signing
		baseQuery = fmt.Sprintf(baseQuery, "")
	} else {
		return nil, fmt.Errorf("invalid ledger number %d", currentLedgerNumber)
	}

	query, params := ca.newLoadChannelAccountsLimitFromDatabase(baseQuery, limit)

	var accounts []*ChannelAccount
	err := sqlExec.SelectContext(ctx, &accounts, query, params...)
	if err != nil {
		return nil, fmt.Errorf("loading channel accounts from database: %w", err)
	}

	return accounts, nil
}

// GetAndLockAll retrieves all channel account that are not already locked from the database and locks them until some future ledger.
func (ca *ChannelAccountModel) GetAndLockAll(ctx context.Context, currentLedger, nextLedgerLock, limit int) ([]*ChannelAccount, error) {
	channelAccounts, err := ca.GetAll(ctx, ca.DBConnectionPool, currentLedger, limit)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve accounts for locking: %w", err)
	}
	if len(channelAccounts) == 0 {
		return nil, fmt.Errorf("no channel accounts available to retrieve")
	}

	var updatedChannelAccounts []*ChannelAccount
	for _, channelAccount := range channelAccounts {
		lockedAccount, err := ca.Lock(ctx, ca.DBConnectionPool, channelAccount.PublicKey, int32(currentLedger), int32(nextLedgerLock))
		if err != nil {
			return nil, fmt.Errorf("cannot lock account %s: %w", channelAccount.PublicKey, err)
		}

		updatedChannelAccounts = append(updatedChannelAccounts, lockedAccount)
	}

	return updatedChannelAccounts, nil
}

// newLoadChannelAccountsLimitFromDatabase returns a query that limits the number of channel accounts retrieved if limit>0,
// or retrieves all channel accounts if limit=0.
func (ca *ChannelAccountModel) newLoadChannelAccountsLimitFromDatabase(
	baseQuery string, limit int,
) (query string, params []interface{}) {
	qb := data.NewQueryBuilder(baseQuery)
	if limit > 0 {
		qb.AddPagination(1, limit)
	}
	query, params = qb.Build()
	return ca.DBConnectionPool.Rebind(query), params
}

// Delete deletes a channel account with the provided publicKey from the database.
func (ca *ChannelAccountModel) Delete(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) error {
	query := `
		DELETE
		FROM
			channel_accounts
		WHERE
			public_key = $1
		`

	res, err := sqlExec.ExecContext(ctx, query, publicKey)
	if err != nil {
		return fmt.Errorf("deleting channel account %q: %w", publicKey, err)
	}

	numRowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return fmt.Errorf("could not find nor delete account %q: %w", publicKey, ErrRecordNotFound)
	} else if numRowsAffected != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d when deleting channel account %s", numRowsAffected, publicKey)
	}

	return nil
}

// DeleteIfLockedUntil deletes a channel account with the provided publicKey from the database only if the provided
// `lockedUntilLedgerNumber` matches the value of the same field on the channel account. Also, if the account has not been
// locked previously, does not proceed with the deletion.
func (ca *ChannelAccountModel) DeleteIfLockedUntil(ctx context.Context, publicKey string, lockedUntilLedgerNumber int) error {
	return db.RunInTransaction(ctx, ca.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		account, err := ca.Get(ctx, dbTx, publicKey, 0)
		if err != nil {
			return fmt.Errorf("cannot retrieve account %s: %w", publicKey, err)
		}

		if !(account.LockedUntilLedgerNumber.Valid && account.LockedUntilLedgerNumber.Int32 == int32(lockedUntilLedgerNumber)) {
			return fmt.Errorf("cannot delete account due to locked until ledger number mismatch or field being null")
		}

		_, err = ca.Unlock(ctx, dbTx, account.PublicKey)
		if err != nil {
			return fmt.Errorf("cannot unlock account for deletion %s: %w", account.PublicKey, err)
		}

		err = ca.Delete(ctx, dbTx, account.PublicKey)
		if err != nil {
			return fmt.Errorf("cannot delete account %s: %w", account.PublicKey, err)
		}

		return nil
	})
}

// queryFilterForLockedState returns a SQL query filter that can be used to filter channel accounts based on their
// locked state.
func (ca *ChannelAccountModel) queryFilterForLockedState(locked bool, ledgerNumber int32) string {
	if locked {
		return fmt.Sprintf("(locked_until_ledger_number >= %d)", ledgerNumber)
	}
	return fmt.Sprintf("(locked_until_ledger_number IS NULL OR locked_until_ledger_number < %d)", ledgerNumber)
}

// Lock locks the channel account with the provided publicKey. It returns a ErrRecordNotFound error if you try to lock a
// channel account that is already locked.
func (ca *ChannelAccountModel) Lock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string, currentLedger, nextLedgerLock int32) (*ChannelAccount, error) {
	q := fmt.Sprintf(`
		UPDATE
			channel_accounts
		SET
			locked_at = NOW(),
			locked_until_ledger_number = $1
		WHERE
			public_key = $2
			AND %s
		RETURNING *
	`, ca.queryFilterForLockedState(false, currentLedger))
	var channelAccount ChannelAccount
	err := sqlExec.GetContext(ctx, &channelAccount, q, nextLedgerLock, publicKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("locking channel account %q: %w", publicKey, err)
	}

	return &channelAccount, nil
}

// Unlock lifts the lock from the channel account with the provided publicKey.
func (ca *ChannelAccountModel) Unlock(ctx context.Context, sqlExec db.SQLExecuter, publicKey string) (*ChannelAccount, error) {
	q := `
		UPDATE
			channel_accounts
		SET
			locked_at = NULL,
			locked_until_ledger_number = NULL
		WHERE
			public_key = $1
		RETURNING *
	`
	var channelAccount ChannelAccount
	err := sqlExec.GetContext(ctx, &channelAccount, q, publicKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("unlocking channel account %q: %w", publicKey, err)
	}

	return &channelAccount, nil
}

var _ ChannelAccountStore = &ChannelAccountModel{}
