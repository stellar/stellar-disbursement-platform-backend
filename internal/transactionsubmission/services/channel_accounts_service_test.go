package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/problem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	engineMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
)

func Test_ChannelAccounts_CreateAccount_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	opts := ChannelAccountServiceOptions{
		NumChannelAccounts: 2,
		MaxBaseFee:         100,
		NetworkPassphrase:  "Test SDF Network ; September 2015",
		RootSeed:           "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currLedgerNumber := 100

	ctx := context.Background()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil)
	mHorizonClient.On(
		"SubmitTransactionWithOptions",
		mock.Anything,
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	).Return(horizon.Transaction{}, nil).Once()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNumber, nil).Once()
	mChannelAccountStore.On(
		"BatchInsertAndLock",
		ctx,
		mock.AnythingOfType("[]*store.ChannelAccount"),
		currLedgerNumber,
		currLedgerNumber+engine.IncrementForMaxLedgerBounds,
	).Return(nil).Once()
	mChannelAccountStore.On(
		"Get", ctx, dbConnectionPool, mock.AnythingOfType("string"), 0,
	).Return(&store.ChannelAccount{PrivateKey: keypair.MustRandom().Seed()}, nil).Twice()
	mChannelAccountStore.On("Unlock", ctx, mock.Anything, mock.AnythingOfType("string")).Return(nil, nil).Twice()

	err = cas.CreateChannelAccountsOnChain(ctx, opts)
	require.NoError(t, err)
	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)

	store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
}

func Test_ChannelAccounts_CreateAccount_CannotFindRootAccount_Failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	opts := ChannelAccountServiceOptions{
		NumChannelAccounts: 2,
		MaxBaseFee:         100,
		NetworkPassphrase:  "Test SDF Network ; September 2015",
		RootSeed:           "SDL4E4RF6BHX77DBKE63QC4H4LQG7S7D2PB4TSF64LTHDIHP7UUJHH2V",
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currLedgerNumber := 100

	ctx := context.Background()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{}, errors.New("cannot find root account"))
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNumber, nil).Once()

	err = cas.CreateChannelAccountsOnChain(ctx, opts)
	require.ErrorContains(
		t,
		err,
		"creating channel accounts in batch in CreateChannelAccountsOnChain: failed to retrieve root account: cannot find root account",
	)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_CreateAccount_Insert_Failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	opts := ChannelAccountServiceOptions{
		NumChannelAccounts: 2,
		MaxBaseFee:         100,
		NetworkPassphrase:  "Test SDF Network ; September 2015",
		RootSeed:           "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currLedgerNumber := 100

	ctx := context.Background()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNumber, nil).Once()
	mChannelAccountStore.On(
		"BatchInsertAndLock",
		ctx,
		mock.AnythingOfType("[]*store.ChannelAccount"),
		currLedgerNumber,
		currLedgerNumber+engine.IncrementForMaxLedgerBounds,
	).Return(errors.New("failure inserting tx in DB"))
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil)

	err = cas.CreateChannelAccountsOnChain(ctx, opts)
	require.EqualError(
		t,
		err,
		"creating channel accounts in batch in CreateChannelAccountsOnChain: failed to insert channel accounts into signature service: batch inserting channel accounts: failure inserting tx in DB",
	)
	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_VerifyAccounts_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}

	cas := ChannelAccountsService{
		caStore:          mChannelAccountStore,
		horizonClient:    mHorizonClient,
		dbConnectionPool: dbConnectionPool,
	}

	opts := ChannelAccountServiceOptions{
		DeleteInvalidAcccounts: false,
	}

	channelAccounts := []*store.ChannelAccount{
		{
			PublicKey: "GC3TKX2B6V7RSIU7UWNJ6MIA7PBTVBXGG7B43HYXRDLHB2DI6FVCYDE3",
		},
		{
			PublicKey: "GAV6VOD2JY6CYJ2XT7U4IH5HL5RJZXEDZFC7CQX5SR7SLLVOP3KPOFH2",
		},
	}

	ctx := context.Background()
	mChannelAccountStore.On("GetAll", ctx, dbConnectionPool, 0, 0).Return(channelAccounts, nil).Once()
	for _, acc := range channelAccounts {
		mHorizonClient.On(
			"AccountDetail",
			horizonclient.AccountRequest{AccountID: acc.PublicKey},
		).Return(horizon.Account{AccountID: acc.PublicKey}, nil).Once()
	}

	err = cas.VerifyChannelAccounts(ctx, opts)
	require.NoError(t, err)
	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
}

func Test_ChannelAccounts_VerifyAccounts_LoadChannelAccountsError_Failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}

	cas := ChannelAccountsService{
		caStore:          mChannelAccountStore,
		horizonClient:    &horizonclient.MockClient{},
		dbConnectionPool: dbConnectionPool,
	}

	opts := ChannelAccountServiceOptions{
		DeleteInvalidAcccounts: false,
	}

	ctx := context.Background()
	mChannelAccountStore.
		On("GetAll", ctx, dbConnectionPool, 0, 0).
		Return(nil, errors.New("cannot load channel accounts from database")).
		Once()

	err = cas.VerifyChannelAccounts(ctx, opts)
	require.EqualError(
		t,
		err,
		"loading channel accounts from database in VerifyChannelAccounts: cannot load channel accounts from database",
	)
	mChannelAccountStore.AssertExpectations(t)
}

func Test_ChannelAccounts_VerifyAccounts_NotFound(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}

	cas := ChannelAccountsService{
		caStore:          mChannelAccountStore,
		horizonClient:    mHorizonClient,
		dbConnectionPool: dbConnectionPool,
	}

	opts := ChannelAccountServiceOptions{
		DeleteInvalidAcccounts: true,
	}

	channelAccounts := []*store.ChannelAccount{
		{
			PublicKey: "GC3TKX2B6V7RSIU7UWNJ6MIA7PBTVBXGG7B43HYXRDLHB2DI6FVCYDE3",
		},
		{
			PublicKey: "GAV6VOD2JY6CYJ2XT7U4IH5HL5RJZXEDZFC7CQX5SR7SLLVOP3KPOFH2",
		},
	}

	ctx := context.Background()
	mChannelAccountStore.On("GetAll", ctx, dbConnectionPool, 0, 0).Return(channelAccounts, nil).Once()
	for _, acc := range channelAccounts {
		mHorizonClient.On(
			"AccountDetail",
			horizonclient.AccountRequest{AccountID: acc.PublicKey},
		).Return(horizon.Account{}, horizonclient.Error{
			Problem: problem.P{
				Type: "https://stellar.org/horizon-errors/not_found",
			},
		}).Once()
		mChannelAccountStore.On("Delete", ctx, dbConnectionPool, acc.PublicKey).Return(nil).Once()
	}

	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

	err = cas.VerifyChannelAccounts(ctx, opts)
	require.NoError(t, err)

	entries := getEntries()
	assert.Equal(t, len(entries), 2)
	for i, entry := range entries {
		assert.Equal(
			t,
			entry.Message,
			fmt.Sprintf("Account %s does not exist on the network", channelAccounts[i].PublicKey),
		)
	}

	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
}

func Test_ChannelAccounts_DeleteAccount_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
		PrivateKey: "YVeMG89DMl2Ku7IeGCumrvneDydfuW+2q4EKQoYhPRpKS/A1bKhNzAa7IjyLiA6UwTESsM6Hh8nactmuOfqUT38YVTx68CIgG6OuwCHPrmws57Tf",
	}

	opts := ChannelAccountServiceOptions{
		ChannelAccountID:  channelAccount.PublicKey,
		MaxBaseFee:        100,
		NetworkPassphrase: "Test SDF Network ; September 2015",
		RootSeed:          "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
		DeleteAllAccounts: false,
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currLedgerNum := 100

	ctx := context.Background()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNum, nil).Once()
	mChannelAccountStore.On("GetAndLock", ctx, opts.ChannelAccountID, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).Once()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: opts.ChannelAccountID}).
		Return(horizon.Account{}, nil).Once()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).Once()
	mChannelAccountStore.On("Get", ctx, mock.Anything, opts.ChannelAccountID, 0).
		Return(channelAccount, nil).Once()
	mHorizonClient.On(
		"SubmitTransactionWithOptions",
		mock.Anything,
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	).Return(horizon.Transaction{}, nil).Once()
	mChannelAccountStore.On("DeleteIfLockedUntil", ctx, opts.ChannelAccountID, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(nil).Once()

	err = cas.DeleteChannelAccount(ctx, opts)
	require.NoError(t, err)

	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_DeleteAccount_All_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	channelAccounts := []*store.ChannelAccount{
		{
			PublicKey:  "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
			PrivateKey: "YVeMG89DMl2Ku7IeGCumrvneDydfuW+2q4EKQoYhPRpKS/A1bKhNzAa7IjyLiA6UwTESsM6Hh8nactmuOfqUT38YVTx68CIgG6OuwCHPrmws57Tf",
		},
		{
			PublicKey:  "GAORBNVUS7TZI6M47CE2XKJIYUZGWTQLPJTU3FEQCFR47H6LTLCTK25P",
			PrivateKey: "I9uPlXL/KvZOOK7kVHHjdFaSeJARV/lvv0YG7P2GCYclgz1MCmthiSZv0BF5HK13PmB4qgzMG9cebxShEZ8AjXDHZA4IOrt+4stE6GF8UR8jdWkG",
		},
	}

	opts := ChannelAccountServiceOptions{
		MaxBaseFee:        100,
		NetworkPassphrase: "Test SDF Network ; September 2015",
		RootSeed:          "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
		DeleteAllAccounts: true,
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currLedgerNum := 1000

	ctx := context.Background()
	mChannelAccountStore.On("Count", ctx).Return(len(channelAccounts), nil).Once()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNum, nil).Times(len(channelAccounts))
	for _, acc := range channelAccounts {
		mChannelAccountStore.
			On("GetAndLockAll", ctx, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds, 1).
			Return([]*store.ChannelAccount{acc}, nil).Once()
		mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: acc.PublicKey}).
			Return(horizon.Account{}, nil).Once()
		mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
			Return(horizon.Account{AccountID: rootAccount.Address()}, nil).Once()
		mHorizonClient.On(
			"SubmitTransactionWithOptions",
			mock.Anything,
			horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
		).Return(horizon.Transaction{}, nil).Once()
		mChannelAccountStore.On("Get", ctx, mock.Anything, acc.PublicKey, 0).
			Return(acc, nil).Once()
		mChannelAccountStore.On("DeleteIfLockedUntil", ctx, acc.PublicKey, currLedgerNum+engine.IncrementForMaxLedgerBounds).
			Return(nil).Once()
	}

	err = cas.DeleteChannelAccount(ctx, opts)
	require.NoError(t, err)

	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_DeleteAccount_FindByPublicKey_Failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	opts := ChannelAccountServiceOptions{
		ChannelAccountID:  "GDKMLSJSPHFWB26JV7ESWLJAKJ6KDTLQWYFT2T4ZVXFFHWBINUEJKASM",
		DeleteAllAccounts: false,
	}

	currLedgerNum := 1000

	ctx := context.Background()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNum, nil).Once()
	mChannelAccountStore.On("GetAndLock", ctx, opts.ChannelAccountID, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(nil, errors.New("db error")).Once()

	err = cas.DeleteChannelAccount(ctx, opts)
	require.ErrorContains(t,
		err,
		fmt.Sprintf("retrieving account %s from database in DeleteChannelAccount: db error", opts.ChannelAccountID),
	)

	mChannelAccountStore.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_DeleteAccount_DeleteFromDatabaseError(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GAMWDQPPO3MXDQHZWYQLCQMKMBVDDCV7WIRKLCALWJPI7MIQHYNERTXS",
		PrivateKey: "SBS2DJJSWZKKADWE4QEFN6CWXPM6KAFULKVJWO5VN7NIFDP6HFZXF6J7",
	}

	opts := ChannelAccountServiceOptions{
		ChannelAccountID:  channelAccount.PublicKey,
		NetworkPassphrase: "Test SDF Network ; September 2015",
		DeleteAllAccounts: false,
		RootSeed:          "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
	}

	currLedgerNum := 1000

	ctx := context.Background()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNum, nil).Once()
	mChannelAccountStore.On("GetAndLock", ctx, opts.ChannelAccountID, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).Once()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: opts.ChannelAccountID}).
		Return(horizon.Account{}, horizonclient.Error{
			Problem: problem.P{
				Type: "https://stellar.org/horizon-errors/not_found",
			},
		}).Once()
	mChannelAccountStore.
		On("DeleteIfLockedUntil", ctx, opts.ChannelAccountID, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(errors.New("db error")).
		Once()

	err = cas.DeleteChannelAccount(ctx, opts)
	require.Error(t, err)
	require.ErrorContains(
		t,
		err,
		fmt.Sprintf(
			`deleting account %[1]s in DeleteChannelAccount: deleting %[1]s from signature service: deleting channel account "%[1]s" from database: db error`,
			opts.ChannelAccountID,
		),
	)

	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_DeleteAccount_SubmitTransaction_Failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
		PrivateKey: "SDHGNWPVZJML64GMSQFVX7RAZBJXO3SWOMEGV77IPXUMKHHEOFD2LC75",
	}

	opts := ChannelAccountServiceOptions{
		ChannelAccountID:  channelAccount.PublicKey,
		MaxBaseFee:        100,
		NetworkPassphrase: "Test SDF Network ; September 2015",
		RootSeed:          "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currLedgerNum := 1000

	ctx := context.Background()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNum, nil).Once()
	mChannelAccountStore.On("GetAndLock", ctx, opts.ChannelAccountID, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).Once()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: opts.ChannelAccountID}).
		Return(horizon.Account{}, nil).Once()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).Once()
	mChannelAccountStore.On("Get", ctx, mock.Anything, opts.ChannelAccountID, 0).
		Return(channelAccount, nil).Once()
	mHorizonClient.On(
		"SubmitTransactionWithOptions",
		mock.Anything,
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	).Return(horizon.Transaction{}, errors.New("foo bar")).Once()

	err = cas.DeleteChannelAccount(ctx, opts)
	assert.ErrorContains(
		t,
		err,
		fmt.Sprintf(
			"deleting account %[1]s in DeleteChannelAccount: deleting account %[1]s onchain: submitting remove account transaction to the network for account %[1]s: horizon response error: foo bar",
			opts.ChannelAccountID,
		),
	)

	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_EnsureChannelAccounts_Exact_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}

	cas := ChannelAccountsService{
		caStore:          mChannelAccountStore,
		horizonClient:    mHorizonClient,
		dbConnectionPool: dbConnectionPool,
	}

	opts := ChannelAccountServiceOptions{NumChannelAccounts: 2}

	ctx := context.Background()
	mChannelAccountStore.On("Count", ctx).
		Return(opts.NumChannelAccounts, nil).Once()
	getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

	err = cas.EnsureChannelAccountsCount(ctx, opts)
	require.NoError(t, err)

	entries := getEntries()
	assert.Equal(t,
		entries[1].Message,
		fmt.Sprintf("There are exactly %d managed channel accounts currently. Exiting...", opts.NumChannelAccounts),
	)

	mChannelAccountStore.AssertExpectations(t)
}

func Test_ChannelAccounts_EnsureChannelAccounts_Add_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	desiredCount := 5
	opts := ChannelAccountServiceOptions{
		NumChannelAccounts: desiredCount,
		MaxBaseFee:         100,
		NetworkPassphrase:  "Test SDF Network ; September 2015",
		RootSeed:           "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currChannelAccountsCount := 2
	currLedgerNum := 100

	ctx := context.Background()
	mChannelAccountStore.On("Count", ctx).Return(currChannelAccountsCount, nil).Once()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNum, nil).Once()
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).Once()
	mHorizonClient.On(
		"SubmitTransactionWithOptions",
		mock.Anything,
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	).Return(horizon.Transaction{}, nil).Once()
	mChannelAccountStore.On("BatchInsertAndLock", ctx, mock.AnythingOfType("[]*store.ChannelAccount"), currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(nil).Once()
	mChannelAccountStore.
		On("Get", ctx, mock.Anything, mock.AnythingOfType("string"), 0).
		Return(&store.ChannelAccount{PrivateKey: keypair.MustRandom().Seed()}, nil).
		Times(desiredCount - currChannelAccountsCount)
	mChannelAccountStore.On("Unlock", ctx, mock.Anything, mock.AnythingOfType("string")).Return(nil, nil).
		Times(desiredCount - currChannelAccountsCount)

	err = cas.EnsureChannelAccountsCount(ctx, opts)
	require.NoError(t, err)

	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_EnsureChannelAccounts_Delete_Success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}

	cas := ChannelAccountsService{
		caStore:             mChannelAccountStore,
		horizonClient:       mHorizonClient,
		dbConnectionPool:    dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
	}

	opts := ChannelAccountServiceOptions{
		NumChannelAccounts: 2,
		MaxBaseFee:         100,
		NetworkPassphrase:  "Test SDF Network ; September 2015",
		RootSeed:           "SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4",
	}

	rootAccount := keypair.MustParseFull(opts.RootSeed)
	currChannelAccountsCount := 4

	channelAccounts := []*store.ChannelAccount{
		{
			PublicKey:  "GCCVRQS7R7V66QDPBZKHRVPOVPCG253BPUSYPWC4GZN54AVXIRHW4QYN",
			PrivateKey: "SCDC7JG53WIFEHFI72KIS6PMMVFDNZDT32VRQY45JVE4FEYNTQYXMWWJ",
		},
		{
			PublicKey:  "GDHVIPZMT6UWY2SNG7RBHK5P5NHXIIWMVINEARIO7QLBVNRJDYUNACDF",
			PrivateKey: "SDRLEKUEM5535VWJSRPICXLVPOWPVSTVWFNQSVIJ6M3TPHXBQBGHWNJ2",
		},
	}

	currLedgerNum := 1000

	ctx := context.Background()
	mChannelAccountStore.On("Count", ctx).Return(currChannelAccountsCount, nil).Once()
	mLedgerNumberTracker.On("GetLedgerNumber").Return(currLedgerNum, nil).Times(currChannelAccountsCount - opts.NumChannelAccounts)
	mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).Times(currChannelAccountsCount - opts.NumChannelAccounts)

	for _, acc := range channelAccounts {
		mChannelAccountStore.On("GetAndLockAll", ctx, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds, 1).
			Return([]*store.ChannelAccount{acc}, nil).Once()
		mHorizonClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: acc.PublicKey}).
			Return(horizon.Account{}, nil).Once()
		mChannelAccountStore.On("Get", ctx, mock.Anything, acc.PublicKey, 0).
			Return(acc, nil).Once()
		mChannelAccountStore.On("DeleteIfLockedUntil", ctx, acc.PublicKey, currLedgerNum+engine.IncrementForMaxLedgerBounds).
			Return(nil).Once()
	}

	mHorizonClient.On(
		"SubmitTransactionWithOptions",
		mock.Anything,
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	).Return(horizon.Transaction{}, nil).Times(currChannelAccountsCount - opts.NumChannelAccounts)

	err = cas.EnsureChannelAccountsCount(ctx, opts)
	require.NoError(t, err)

	mChannelAccountStore.AssertExpectations(t)
	mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker.AssertExpectations(t)
}

func Test_ChannelAccounts_ViewChannelAccounts_Success(t *testing.T) {
	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}

	cas := ChannelAccountsService{
		caStore:       mChannelAccountStore,
		horizonClient: &horizonclient.MockClient{},
	}

	channelAccounts := []*store.ChannelAccount{
		{
			PublicKey: "GDTQYQQSQ5AG6ZYERKU5VH3RBPEZ33U5HGYM6SPUY42QULOQIC2MRZ3N",
		},
		{
			PublicKey: "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
		},
		{
			PublicKey: "GAR7SZWK2GV23OGIQC2BBZUUDSVSMT3MUOY7NJLJ75W5OJ3KQUR7VAIV",
		},
	}

	ctx := context.Background()
	mChannelAccountStore.On("GetAll", ctx, mock.Anything, 0, 0).Return(channelAccounts, nil).Once()
	getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

	err := cas.ViewChannelAccounts(ctx)
	require.NoError(t, err)

	entries := getEntries()
	for i, entry := range entries[1:] {
		assert.Equal(
			t,
			entry.Message,
			fmt.Sprintf("Found account %s", channelAccounts[i].PublicKey),
		)
	}

	mChannelAccountStore.AssertExpectations(t)
}

func Test_ChannelAccounts_ViewChannelAccounts_LoadChannelAccountsError_Failure(t *testing.T) {
	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}

	cas := ChannelAccountsService{
		caStore:       mChannelAccountStore,
		horizonClient: &horizonclient.MockClient{},
	}
	ctx := context.Background()
	mChannelAccountStore.On("GetAll", ctx, mock.Anything, 0, 0).
		Return(nil, errors.New("db error")).Once()

	err := cas.ViewChannelAccounts(ctx)
	require.EqualError(t, err, "loading channel accounts from database in ViewChannelAccounts: db error")

	mChannelAccountStore.AssertExpectations(t)
}
