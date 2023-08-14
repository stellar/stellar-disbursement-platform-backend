package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ChannelAccount_IsLocked(t *testing.T) {
	const currentLedgerNumber = 10

	testCases := []struct {
		name                    string
		lockedUntilLedgerNumber sql.NullInt32
		wantResult              bool
	}{
		{
			name:                    "returns false if lockedUntilLedgerNumber is null",
			lockedUntilLedgerNumber: sql.NullInt32{},
			wantResult:              false,
		},
		{
			name:                    "returns false if lockedUntilLedgerNumber is lower than currentLedgerNumber",
			lockedUntilLedgerNumber: sql.NullInt32{Int32: currentLedgerNumber - 1, Valid: true},
			wantResult:              false,
		},
		{
			name:                    "returns true if lockedUntilLedgerNumber is equal to currentLedgerNumber",
			lockedUntilLedgerNumber: sql.NullInt32{Int32: currentLedgerNumber, Valid: true},
			wantResult:              true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ca := &ChannelAccount{LockedUntilLedgerNumber: tc.lockedUntilLedgerNumber}
			assert.Equal(t, tc.wantResult, ca.IsLocked(currentLedgerNumber))
		})
	}
}

func Test_ChannelAccountModel_BatchInsert_GetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	caModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 1
	const nextLedgerLock int32 = 11

	testCases := []struct {
		name            string
		chAccounts      []*ChannelAccount
		wantErrContains string
		queryAtLedger   int
		lockAccounts    bool
	}{
		{
			name: "empty accounts won't return error and won't create any records",
		},
		{
			name: "returns error if a public key is empty",
			chAccounts: []*ChannelAccount{
				{
					PublicKey:  "",
					PrivateKey: "SAIXHVEDXDEO37PUD7SAJU2BPZGRP43EI3FOPHP4L7AP3LICY6AMIR6T",
				},
			},
			wantErrContains: "public key cannot be empty",
		},
		{
			name: "returns error if a private key is empty",
			chAccounts: []*ChannelAccount{
				{
					PublicKey:  "GCFZGYGGXEMPJNL52QX2DXG2X5ZHJ3XTEWAUBWXQE2PXX7V532AI4ALT",
					PrivateKey: "",
				},
			},
			wantErrContains: "private key cannot be empty",
		},
		{
			name: "🎉 successfully insert one channel account",
			chAccounts: []*ChannelAccount{
				{
					PublicKey:  "GCFZGYGGXEMPJNL52QX2DXG2X5ZHJ3XTEWAUBWXQE2PXX7V532AI4ALT",
					PrivateKey: "SAIXHVEDXDEO37PUD7SAJU2BPZGRP43EI3FOPHP4L7AP3LICY6AMIR6T",
				},
			},
		},
		{
			name: "returns 0 channel accounts when querying at ledger number before accounts are unlocked",
			chAccounts: []*ChannelAccount{
				{
					PublicKey:  "GCFZGYGGXEMPJNL52QX2DXG2X5ZHJ3XTEWAUBWXQE2PXX7V532AI4ALT",
					PrivateKey: "SAIXHVEDXDEO37PUD7SAJU2BPZGRP43EI3FOPHP4L7AP3LICY6AMIR6T",
				},
				{
					PublicKey:  "GAL3MHT7SWJXV33JHK2BENHUVUZLENMJFYOLJU4CLI3723MDSRJL5AJM",
					PrivateKey: "SBHQLRTVR2HKLRE5UKKV2VIZIR7VHZQ6375KWOKU3E6H2AKE374VICXQ",
				},
			},
			queryAtLedger: 5,
			lockAccounts:  true,
		},
		{
			name: "🎉 successfully insert multiple channel accounts",
			chAccounts: []*ChannelAccount{
				{
					PublicKey:  "GCFZGYGGXEMPJNL52QX2DXG2X5ZHJ3XTEWAUBWXQE2PXX7V532AI4ALT",
					PrivateKey: "SAIXHVEDXDEO37PUD7SAJU2BPZGRP43EI3FOPHP4L7AP3LICY6AMIR6T",
				},
				{
					PublicKey:  "GAL3MHT7SWJXV33JHK2BENHUVUZLENMJFYOLJU4CLI3723MDSRJL5AJM",
					PrivateKey: "SBHQLRTVR2HKLRE5UKKV2VIZIR7VHZQ6375KWOKU3E6H2AKE374VICXQ",
				},
			},
		},
	}

	type comparableChAccount struct {
		PublicKey  string
		PrivateKey string
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			batchInsertErr := caModel.BatchInsert(ctx, caModel.DBConnectionPool, tc.chAccounts)

			if tc.lockAccounts {
				for _, ca := range tc.chAccounts {
					_, err = caModel.Lock(ctx, caModel.DBConnectionPool, ca.PublicKey, currentLedger, nextLedgerLock)
					require.NoError(t, err)
				}
			}

			allChAccounts, getAllErr := caModel.GetAll(ctx, caModel.DBConnectionPool, tc.queryAtLedger, 0)
			require.NoError(t, getAllErr)

			if tc.wantErrContains != "" {
				require.Error(t, batchInsertErr)
				assert.ErrorContains(t, batchInsertErr, tc.wantErrContains)
			} else if tc.lockAccounts {
				require.NoError(t, err)
				assert.Len(t, allChAccounts, 0)
			} else {
				require.NoError(t, batchInsertErr)
				assert.Equal(t, len(tc.chAccounts), len(allChAccounts))

				// compare the accounts
				var allChAccountsComparable []comparableChAccount
				for _, chAccount := range allChAccounts {
					allChAccountsComparable = append(allChAccountsComparable, comparableChAccount{
						PublicKey:  chAccount.PublicKey,
						PrivateKey: chAccount.PrivateKey,
					})
				}

				var tcChAccountsComparable []comparableChAccount
				for _, chAccount := range tc.chAccounts {
					tcChAccountsComparable = append(tcChAccountsComparable, comparableChAccount{
						PublicKey:  chAccount.PublicKey,
						PrivateKey: chAccount.PrivateKey,
					})
				}

				assert.ElementsMatch(t, tcChAccountsComparable, allChAccountsComparable)
			}

			DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_ChannelAccountModel_Insert_Get(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	caModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 1
	const nextLedgerLock int32 = 11

	testCases := []struct {
		name                string
		channelAccounts     []*ChannelAccount
		publicKeysToQuery   []string
		expectedErrorFormat string
		lockAccounts        bool
		queryAtLedger       int
		errorType           string
	}{
		{
			name: "can insert and get a channel account",
			channelAccounts: []*ChannelAccount{
				{
					PublicKey:  "GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
					PrivateKey: "SBXVHYY2VXHTXGHSQ4VXC7LSUUECQY633CZTY5Q6JCYRP5KQC4WRWU25",
				},
				{
					PublicKey:  "GCXLO7JS3X7H45ZQEJIA2NQPCAPGQW3TSYGPWUUSBXDXDGMZZFHEWZSU",
					PrivateKey: "SCYDT4TJF43OAO3TYQQAWKEPOGJSBXZGW3WVQZGTOXVVCSM5TFTCJQRZ",
				},
			},
			publicKeysToQuery: []string{
				"GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
				"GCXLO7JS3X7H45ZQEJIA2NQPCAPGQW3TSYGPWUUSBXDXDGMZZFHEWZSU",
			},
			expectedErrorFormat: "",
		},
		{
			name: "can get channel account at valid ledger number",
			channelAccounts: []*ChannelAccount{
				{
					PublicKey:  "GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
					PrivateKey: "SBXVHYY2VXHTXGHSQ4VXC7LSUUECQY633CZTY5Q6JCYRP5KQC4WRWU25",
				},
			},
			publicKeysToQuery: []string{
				"GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
			},
			queryAtLedger: 12,
			lockAccounts:  true,
		},
		{
			name:            "returns an error when trying to get a channel account that does not exist",
			channelAccounts: []*ChannelAccount{},
			publicKeysToQuery: []string{
				"GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
				"GCXLO7JS3X7H45ZQEJIA2NQPCAPGQW3TSYGPWUUSBXDXDGMZZFHEWZSU",
			},
			expectedErrorFormat: "could not find channel account %q: record not found",
			errorType:           "invalid acocunt",
		},
		{
			name: "returns an error when querying at invalid ledger number",
			channelAccounts: []*ChannelAccount{
				{
					PublicKey:  "GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
					PrivateKey: "SBXVHYY2VXHTXGHSQ4VXC7LSUUECQY633CZTY5Q6JCYRP5KQC4WRWU25",
				},
			},
			publicKeysToQuery: []string{
				"GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
			},
			queryAtLedger:       -1,
			expectedErrorFormat: "invalid ledger number %d",
			errorType:           "invalid ledger number",
		},
		{
			name: "returns an error when querying for locked channel account",
			channelAccounts: []*ChannelAccount{
				{
					PublicKey:  "GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
					PrivateKey: "SBXVHYY2VXHTXGHSQ4VXC7LSUUECQY633CZTY5Q6JCYRP5KQC4WRWU25",
				},
			},
			publicKeysToQuery: []string{
				"GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
			},
			queryAtLedger:       5,
			expectedErrorFormat: "could not find channel account %q: record not found",
			errorType:           "locked channel account",
			lockAccounts:        true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			for _, ca := range test.channelAccounts {
				err = caModel.Insert(ctx, caModel.DBConnectionPool, ca.PublicKey, ca.PrivateKey)
				require.NoError(t, err)
			}

			for _, pubKey := range test.publicKeysToQuery {
				// lock accounts if queryAtLedger specified
				if test.lockAccounts {
					_, err = caModel.Lock(ctx, caModel.DBConnectionPool, pubKey, currentLedger, nextLedgerLock)
					require.NoError(t, err)
				}

				cam, err := caModel.Get(ctx, caModel.DBConnectionPool, pubKey, test.queryAtLedger)
				if test.expectedErrorFormat != "" {
					require.Error(t, err)
					switch test.errorType {
					case "invalid account", "locked channel account":
						assert.EqualError(t, err, fmt.Sprintf(test.expectedErrorFormat, pubKey))
					case "invalid ledger number":
						assert.EqualError(t, err, fmt.Sprintf(test.expectedErrorFormat, test.queryAtLedger))
					}

				} else {
					require.NoError(t, err)
					assert.Equal(t, pubKey, cam.PublicKey)
				}
			}

			DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_ChannelAccountModel_Insert_Count(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	caModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	testCases := []struct {
		numChannelAccounts int
	}{
		{numChannelAccounts: 0},
		{numChannelAccounts: 1},
		{numChannelAccounts: 10},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d channel cccount(s)", tc.numChannelAccounts), func(t *testing.T) {
			for range make([]interface{}, tc.numChannelAccounts) {
				kp, err := keypair.Random()
				require.NoError(t, err)
				err = caModel.Insert(ctx, caModel.DBConnectionPool, kp.Address(), kp.Seed())
				require.NoError(t, err)
			}

			count, err := caModel.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.numChannelAccounts, count)

			DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_ChannelAccountModel_Insert_Delete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	caModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	ca := &ChannelAccount{
		PublicKey:  "GDLYOWHAC2U4I52OXDEWMEAVNR6WLML3LIG32QOOLKWPCC233OBSKVU5",
		PrivateKey: "SBXVHYY2VXHTXGHSQ4VXC7LSUUECQY633CZTY5Q6JCYRP5KQC4WRWU25",
	}

	testCases := []struct {
		name                   string
		channelAccountToAdd    *ChannelAccount
		channelAccountToDelete *ChannelAccount
		expectedErrorFormat    string
	}{
		{
			name:                   "add and delete channel account",
			channelAccountToAdd:    ca,
			channelAccountToDelete: ca,
		},
		{
			name:                "returns an error when trying to delete a channel account that does not exist",
			channelAccountToAdd: ca,
			channelAccountToDelete: &ChannelAccount{
				PublicKey:  "GCXLO7JS3X7H45ZQEJIA2NQPCAPGQW3TSYGPWUUSBXDXDGMZZFHEWZSU",
				PrivateKey: "SCYDT4TJF43OAO3TYQQAWKEPOGJSBXZGW3WVQZGTOXVVCSM5TFTCJQRZ",
			},
			expectedErrorFormat: "could not find nor delete account %q: record not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err = caModel.Insert(ctx, caModel.DBConnectionPool, tc.channelAccountToAdd.PublicKey, tc.channelAccountToAdd.PrivateKey)
			require.NoError(t, err)

			err = caModel.Delete(ctx, caModel.DBConnectionPool, tc.channelAccountToDelete.PublicKey)
			if tc.expectedErrorFormat != "" {
				require.Error(t, err)
				assert.EqualError(t, fmt.Errorf(tc.expectedErrorFormat, tc.channelAccountToDelete.PublicKey), err.Error())
			} else {
				require.NoError(t, err)
			}

			DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_ChannelAccountModel_queryFilterForLockedState(t *testing.T) {
	chAccModel := &ChannelAccountModel{}

	testCases := []struct {
		name         string
		locked       bool
		ledgerNumber int32
		wantFilter   string
	}{
		{
			name:         "locked to ledgerNumber=10",
			locked:       true,
			ledgerNumber: 10,
			wantFilter:   "(locked_until_ledger_number >= 10)",
		},
		{
			name:         "unlocked or expired on ledgerNumber=20",
			locked:       false,
			ledgerNumber: 20,
			wantFilter:   "(locked_until_ledger_number IS NULL OR locked_until_ledger_number < 20)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotFilter := chAccModel.queryFilterForLockedState(tc.locked, tc.ledgerNumber)
			assert.Equal(t, tc.wantFilter, gotFilter)
		})
	}
}

func Test_ChannelAccountModel_Lock(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 10
	const nextLedgerLock int32 = 20

	testCases := []struct {
		name                     string
		initialLockedAt          sql.NullTime
		initialLockedUntilLedger sql.NullInt32
		expectedErrContains      string
	}{
		{
			name: "🎉 successfully locks channel account without any previous lock",
		},
		{
			name:                     "🎉 successfully locks channel account with lock expired",
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger - 1, Valid: true},
		},
		{
			name:                     "🚧 cannot be locked again if still locked",
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger, Valid: true},
			expectedErrContains:      ErrRecordNotFound.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channelAccount := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)[0]
			q := `UPDATE channel_accounts SET locked_at = $1, locked_until_ledger_number = $2 WHERE public_key = $3`
			_, err := dbConnectionPool.ExecContext(ctx, q, tc.initialLockedAt, tc.initialLockedUntilLedger, channelAccount.PublicKey)
			require.NoError(t, err)

			channelAccount, err = chAccModel.Lock(ctx, dbConnectionPool, channelAccount.PublicKey, currentLedger, nextLedgerLock)

			if tc.expectedErrContains == "" {
				require.NoError(t, err)
				channelAccount, err = chAccModel.Get(ctx, chAccModel.DBConnectionPool, channelAccount.PublicKey, 0)
				require.NoError(t, err)
				assert.True(t, channelAccount.LockedAt.Valid)
				assert.True(t, channelAccount.LockedUntilLedgerNumber.Valid)
				assert.Equal(t, nextLedgerLock, channelAccount.LockedUntilLedgerNumber.Int32)

				var channelAccountRefreshed *ChannelAccount
				channelAccountRefreshed, err = chAccModel.Get(ctx, chAccModel.DBConnectionPool, channelAccount.PublicKey, 0)
				require.NoError(t, err)
				require.Equal(t, *channelAccountRefreshed, *channelAccount)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.expectedErrContains)
			}

			DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_ChannelAccountModel_Unlock(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 10

	testCases := []struct {
		name                     string
		initialLockedAt          sql.NullTime
		initialLockedUntilLedger sql.NullInt32
	}{
		{
			name: "🎉 successfully unlocks channel account that were not locked",
		},
		{
			name:                     "🎉 successfully unlocks channel account whose lock was expired",
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger - 1, Valid: true},
		},
		{
			name:                     "🎉 successfully unlocks locked channel account",
			initialLockedAt:          sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
			initialLockedUntilLedger: sql.NullInt32{Int32: currentLedger, Valid: true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channelAccount := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)[0]
			q := `UPDATE channel_accounts SET locked_at = $1, locked_until_ledger_number = $2 WHERE public_key = $3`
			_, err := dbConnectionPool.ExecContext(ctx, q, tc.initialLockedAt, tc.initialLockedUntilLedger, channelAccount.PublicKey)
			require.NoError(t, err)

			channelAccount, err = chAccModel.Unlock(ctx, dbConnectionPool, channelAccount.PublicKey)
			require.NoError(t, err)
			assert.False(t, channelAccount.LockedAt.Valid)
			assert.False(t, channelAccount.LockedUntilLedgerNumber.Valid)

			channelAccountRefreshed, err := chAccModel.Get(ctx, chAccModel.DBConnectionPool, channelAccount.PublicKey, 0)
			require.NoError(t, err)
			require.Equal(t, *channelAccountRefreshed, *channelAccount)

			DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}
}

func Test_ChannelAccountModel_Lock_Unlock(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	const currentLedger int32 = 10
	const nextLedgerLock int32 = 20

	// On creation, channel account is unlocked
	channelAccount := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)[0]
	assert.False(t, channelAccount.IsLocked(currentLedger))

	count := 3
	for range make(sql.RawBytes, count) {
		// Lock channel account
		channelAccount, err = chAccModel.Lock(ctx, dbConnectionPool, channelAccount.PublicKey, currentLedger, nextLedgerLock)
		require.NoError(t, err)
		assert.True(t, channelAccount.IsLocked(currentLedger))

		channelAccountRefreshed, err := chAccModel.Get(ctx, chAccModel.DBConnectionPool, channelAccount.PublicKey, 0)
		require.NoError(t, err)
		require.Equal(t, *channelAccountRefreshed, *channelAccount)

		// Unlock channel account
		channelAccount, err = chAccModel.Unlock(ctx, dbConnectionPool, channelAccount.PublicKey)
		require.NoError(t, err)
		assert.False(t, channelAccount.IsLocked(currentLedger))

		channelAccountRefreshed, err = chAccModel.Get(ctx, chAccModel.DBConnectionPool, channelAccount.PublicKey, 0)
		require.NoError(t, err)
		require.Equal(t, *channelAccountRefreshed, *channelAccount)

		count--
	}

	assert.Equal(t, 0, count)

	DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
}

func Test_ChannelAccountModel_DeleteIfLockedUntil(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	lockedToLedger := 100
	testCases := []struct {
		name                           string
		accountLockedUntilLedgerNumber int
		deleteAtLedgerNumber           int
		expectedErrContains            string
	}{
		{
			name:                           "returns error if delete at ledger number different from locked until ledger number",
			accountLockedUntilLedgerNumber: lockedToLedger,
			deleteAtLedgerNumber:           lockedToLedger + 1,
			expectedErrContains:            "cannot delete account due to locked until ledger number mismatch or field being null",
		},
		{
			name:                 "returns error if account not locked to ledger",
			deleteAtLedgerNumber: lockedToLedger,
			expectedErrContains:  "cannot delete account due to locked until ledger number mismatch or field being null",
		},
		{
			name:                           "successfully delete at ledger number",
			accountLockedUntilLedgerNumber: lockedToLedger,
			deleteAtLedgerNumber:           lockedToLedger,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channelAccount := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)[0]
			if tc.accountLockedUntilLedgerNumber != 0 {
				_, lockErr := chAccModel.Lock(
					ctx,
					chAccModel.DBConnectionPool,
					channelAccount.PublicKey,
					int32(tc.accountLockedUntilLedgerNumber),
					int32(tc.accountLockedUntilLedgerNumber),
				)
				require.NoError(t, lockErr)
			}

			err = chAccModel.DeleteIfLockedUntil(ctx, channelAccount.PublicKey, tc.deleteAtLedgerNumber)
			if tc.expectedErrContains != "" {
				require.ErrorContains(t, err, tc.expectedErrContains)
			} else {
				require.NoError(t, err)
			}

			DeleteAllFromChannelAccounts(t, ctx, chAccModel.DBConnectionPool)
		})
	}
}

func Test_ChannelAccountModel_GetAndLockAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	t.Run("try to lock all accounts that have already been locked", func(t *testing.T) {
		currLedgerNumber := int32(1)
		channelAccounts := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 3)
		for _, account := range channelAccounts {
			_, err := chAccModel.Lock(ctx, chAccModel.DBConnectionPool, account.PublicKey, currLedgerNumber, currLedgerNumber+10)
			require.NoError(t, err)
		}

		_, err := chAccModel.GetAndLockAll(ctx, int(currLedgerNumber), int(currLedgerNumber+5), 0)
		require.EqualError(t, err, "no channel accounts available to retrieve")

		DeleteAllFromChannelAccounts(t, ctx, chAccModel.DBConnectionPool)
	})

	t.Run("get and lock all available accounts", func(t *testing.T) {
		currLedgerNumber := int32(1)
		lockToLedgerNumber := currLedgerNumber + 10
		CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 3)

		updatedChannelAccounts, err := chAccModel.GetAndLockAll(ctx, int(currLedgerNumber), int(lockToLedgerNumber), 0)
		require.NoError(t, err)
		for _, account := range updatedChannelAccounts {
			assert.Equal(t, account.LockedUntilLedgerNumber.Int32, int32(lockToLedgerNumber))
		}

		DeleteAllFromChannelAccounts(t, ctx, chAccModel.DBConnectionPool)
	})
}

func Test_ChannelAccountModel_GetAndLock(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccModel := ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	t.Run("try to lock an account that has already been locked", func(t *testing.T) {
		currLedgerNumber := int32(1)
		channelAccount := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)[0]
		_, err := chAccModel.Lock(ctx, chAccModel.DBConnectionPool, channelAccount.PublicKey, currLedgerNumber, currLedgerNumber+10)
		require.NoError(t, err)

		_, err = chAccModel.GetAndLock(ctx, channelAccount.PublicKey, int(currLedgerNumber), int(currLedgerNumber+5))
		require.ErrorContains(t, err, fmt.Sprintf("cannot retrieve account %s", channelAccount.PublicKey))

		DeleteAllFromChannelAccounts(t, ctx, chAccModel.DBConnectionPool)
	})

	t.Run("get and lock an available account", func(t *testing.T) {
		currLedgerNumber := int32(1)
		lockToLedgerNumber := currLedgerNumber + 10
		channelAccount := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)[0]

		updatedAccount, err := chAccModel.GetAndLock(ctx, channelAccount.PublicKey, int(currLedgerNumber), int(lockToLedgerNumber))
		require.NoError(t, err)
		assert.Equal(t, updatedAccount.LockedUntilLedgerNumber.Int32, int32(lockToLedgerNumber))

		DeleteAllFromChannelAccounts(t, ctx, chAccModel.DBConnectionPool)
	})
}
