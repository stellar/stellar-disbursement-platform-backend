package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/require"
)

func Test_NewChannelTransactionBundleModel(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name          string
		dbConnection  db.DBConnectionPool
		expectedError error
		expectedModel *ChannelTransactionBundleModel
	}{
		{
			name:          "returns an error if dbConnectionPool is nil",
			dbConnection:  nil,
			expectedError: fmt.Errorf("dbConnectionPool cannot be nil"),
			expectedModel: nil,
		},
		{
			name:          "🎉 successfully returns a model if dbConnectionPool is not nil",
			dbConnection:  dbConnectionPool,
			expectedError: nil,
			expectedModel: &ChannelTransactionBundleModel{
				dbConnectionPool:    dbConnectionPool,
				channelAccountModel: &ChannelAccountModel{DBConnectionPool: dbConnectionPool},
				transactionModel:    NewTransactionModel(dbConnectionPool),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualModel, actualError := NewChannelTransactionBundleModel(tc.dbConnection)
			require.Equal(t, tc.expectedError, actualError)
			require.Equal(t, tc.expectedModel, actualModel)
		})
	}
}

func Test_ChannelTransactionBundleModel_LoadAndLockTuples(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	const currentLedgerNumber = 100

	chAccTupleModel, err := NewChannelTransactionBundleModel(dbConnectionPool)
	require.NoError(t, err)
	txModel := NewTransactionModel(dbConnectionPool)
	chAccModel := ChannelAccountModel{dbConnectionPool}

	testCases := []struct {
		name                            string
		limit                           int
		lockToLedgerNumber              int
		numberOfChannelAccountsLocked   int
		numberOfChannelAccountsUnlocked int
		numberOfTransactionsLocked      int
		numberOfTransactionsUnlocked    int
		expectedError                   error
	}{
		{
			name:          "returns an error if limit<1",
			limit:         0,
			expectedError: fmt.Errorf("limit must be greater than 0"),
		},
		{
			name:               "returns an error if lockToLedgerNumber<=currentLedgerNumber",
			limit:              100,
			lockToLedgerNumber: currentLedgerNumber,
			expectedError:      fmt.Errorf("lockToLedgerNumber must be greater than currentLedgerNumber"),
		},
		{
			name:               "returns nil len(transactions) == 0",
			limit:              100,
			lockToLedgerNumber: currentLedgerNumber + 1,
		},
		{
			name:                       "returns nil if len(unlockedTransactions) == 0",
			limit:                      100,
			lockToLedgerNumber:         currentLedgerNumber + 1,
			numberOfTransactionsLocked: 10,
		},
		{
			name:                            "returns nil if len(unlockedTransactions) == 0 && len(unlockedChannelAccounts) > 0",
			limit:                           100,
			lockToLedgerNumber:              currentLedgerNumber + 1,
			numberOfTransactionsLocked:      10,
			numberOfChannelAccountsUnlocked: 10,
		},
		{
			name:                         "returns an error if len(unlockedTransactions) > 0 && len(unlockedChannelAccounts) == 0",
			limit:                        100,
			lockToLedgerNumber:           currentLedgerNumber + 1,
			numberOfTransactionsUnlocked: 10,
			expectedError:                fmt.Errorf("running atomic function in RunInTransactionWithResult: %w", ErrInsuficientChannelAccounts),
		},
		{
			name:                            "🎉 successfully returns chTxBundles if limit == len(unlockedTransactions) == len(unlockedChannelAccounts) > 0",
			limit:                           10,
			lockToLedgerNumber:              currentLedgerNumber + 10,
			numberOfTransactionsUnlocked:    10,
			numberOfChannelAccountsUnlocked: 10,
		},
		{
			name:                            "🎉 successfully returns chTxBundles if limit < len(unlockedTransactions) == len(unlockedChannelAccounts) > 0",
			limit:                           5,
			lockToLedgerNumber:              currentLedgerNumber + 10,
			numberOfTransactionsUnlocked:    10,
			numberOfChannelAccountsUnlocked: 10,
		},
		{
			name:                            "🎉 successfully returns chTxBundles if limit > len(unlockedTransactions) == len(unlockedChannelAccounts) > 0",
			limit:                           100,
			lockToLedgerNumber:              currentLedgerNumber + 10,
			numberOfTransactionsUnlocked:    10,
			numberOfChannelAccountsUnlocked: 10,
		},
		{
			name:                            "🎉 successfully returns chTxBundles if limit > len(unlockedTransactions) > len(unlockedChannelAccounts) > 0",
			limit:                           100,
			lockToLedgerNumber:              currentLedgerNumber + 10,
			numberOfTransactionsUnlocked:    20,
			numberOfChannelAccountsUnlocked: 10,
		},
		{
			name:                            "🎉 successfully returns chTxBundles if limit > len(unlockedChannelAccounts) > len(unlockedTransactions) > 0",
			limit:                           100,
			lockToLedgerNumber:              currentLedgerNumber + 10,
			numberOfTransactionsUnlocked:    10,
			numberOfChannelAccountsUnlocked: 20,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// ChannelAccounts(LOCKED)
			lockedChAccounts := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, tc.numberOfChannelAccountsLocked)
			for _, chAcc := range lockedChAccounts {
				_, err = chAccModel.Lock(ctx, dbConnectionPool, chAcc.PublicKey, int32(currentLedgerNumber*2), int32(tc.lockToLedgerNumber))
				require.NoError(t, err)
			}

			// ChannelAccounts(UNLOCKED)
			unlockedChAccounts := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, tc.numberOfChannelAccountsUnlocked)

			// Transactions(LOCKED)
			lockedTransactions := CreateTransactionFixtures(t, ctx, dbConnectionPool, tc.numberOfTransactionsLocked, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "", TransactionStatusPending, 1)
			for _, tx := range lockedTransactions {
				_, err = txModel.Lock(ctx, dbConnectionPool, tx.ID, int32(currentLedgerNumber*2), int32(tc.lockToLedgerNumber))
				require.NoError(t, err)
			}

			// Transactions(UNLOCKED)
			unlockedTransactions := CreateTransactionFixtures(t, ctx, dbConnectionPool, tc.numberOfTransactionsUnlocked, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "", TransactionStatusPending, 1)

			chTxBundles, err := chAccTupleModel.LoadAndLockTuples(ctx, currentLedgerNumber, tc.lockToLedgerNumber, tc.limit)
			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
				require.Empty(t, chTxBundles)
			} else {
				require.NoError(t, err)
				minLength := tc.limit
				if tc.numberOfChannelAccountsUnlocked < minLength {
					minLength = tc.numberOfChannelAccountsUnlocked
				}
				if tc.numberOfTransactionsUnlocked < minLength {
					minLength = tc.numberOfTransactionsUnlocked
				}
				require.Len(t, chTxBundles, minLength)

				if len(chTxBundles) == 0 {
					return
				}

				initiallyUnlockedChAccIDs := sdpUtils.MapSlice(unlockedChAccounts, func(chAcc *ChannelAccount) string { return chAcc.PublicKey })
				gotChAccIDs := sdpUtils.MapSlice(chTxBundles, func(chTxBundle *ChannelTransactionBundle) string { return chTxBundle.ChannelAccount.PublicKey })
				require.Subset(t, initiallyUnlockedChAccIDs, gotChAccIDs)

				initiallyUnlockedTxIDs := sdpUtils.MapSlice(unlockedTransactions, func(tx *Transaction) string { return tx.ID })
				gotTxIDs := sdpUtils.MapSlice(chTxBundles, func(chTxBundle *ChannelTransactionBundle) string { return chTxBundle.Transaction.ID })
				require.Subset(t, initiallyUnlockedTxIDs, gotTxIDs)

				// verify if the channel accounts are properly locked in the DB
				var count int
				q := fmt.Sprintf(`SELECT COUNT(*) FROM channel_accounts WHERE %s`, chAccModel.queryFilterForLockedState(true, currentLedgerNumber))
				err = dbConnectionPool.GetContext(ctx, &count, q)
				require.NoError(t, err)
				require.Equal(t, tc.numberOfChannelAccountsLocked+len(chTxBundles), count)

				// verify if the transactions are properly locked in the DB
				q = fmt.Sprintf(`SELECT COUNT(*) FROM submitter_transactions WHERE %s`, txModel.queryFilterForLockedState(true, currentLedgerNumber))
				err = dbConnectionPool.GetContext(ctx, &count, q)
				require.NoError(t, err)
				require.Equal(t, tc.numberOfTransactionsLocked+len(chTxBundles), count)
			}

			DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
		})
	}
}
