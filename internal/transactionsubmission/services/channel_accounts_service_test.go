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
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
)

func Test_ChannelAccountsService_validate(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)

	testCases := []struct {
		name                      string
		serviceOptions            ChannelAccountsService
		isAdvisoryLockUnavailable bool
		wantError                 string
	}{
		{
			name:      "TSSDBConnectionPool cannot be nil",
			wantError: "tss db connection pool cannot be nil",
		},
		{
			name: "SubmitterEngine cannot be empty",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
			},
			wantError: "submitter engine cannot be empty",
		},
		{
			name: "Validating SubmitterEngine: horizon client cannot be nil",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					MaxBaseFee: 100,
				},
			},
			wantError: "validating submitter engine: horizon client cannot be nil",
		},
		{
			name: "Validating SubmitterEngine: ledger number tracker cannot be nil",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient: mHorizonClient,
				},
			},
			wantError: "validating submitter engine: ledger number tracker cannot be nil",
		},
		{
			name: "Validating SubmitterEngine: signature service cannot be nil",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
				},
			},
			wantError: "validating submitter engine: signature service cannot be empty",
		},
		{
			name: "Validating SubmitterEngine: max base fee must be greater than or equal to 100",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
					SignatureService:    sigService,
				},
			},
			wantError: "validating submitter engine: maxBaseFee must be greater than or equal to 100",
		},
		{
			name: "advisory lock with ID was unavailable",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
					SignatureService:    sigService,
					MaxBaseFee:          100,
				},
			},
			isAdvisoryLockUnavailable: true,
			wantError:                 "failed getting db advisory lock: advisory lock is unavailable",
		},
		{
			name: "🎉 Successfully validate service",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
					SignatureService:    sigService,
					MaxBaseFee:          100,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.isAdvisoryLockUnavailable {
				anotherDBConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
				require.NoError(t, err)
				defer anotherDBConnectionPool.Close()

				ctx := context.Background()
				err = acquireAdvisoryLockForCommand(ctx, anotherDBConnectionPool)
				require.NoError(t, err)
			}

			err := tc.serviceOptions.validate()
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.wantError)
			}
		})
	}
}

func Test_ChannelAccountsService_GetChannelAccountStore(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	t.Run("GetChannelAccountStore() instantiates a new ChannelAccountStore if the current chAccService value is empty", func(t *testing.T) {
		chAccService := ChannelAccountsService{TSSDBConnectionPool: dbConnectionPool}
		wantChAccStore := store.NewChannelAccountModel(dbConnectionPool)
		chAccStore := chAccService.GetChannelAccountStore()
		require.Equal(t, wantChAccStore, chAccStore)
		require.Equal(t, wantChAccStore, chAccService.chAccStore)
		require.NotEqual(t, &wantChAccStore, &chAccService.chAccStore)
		require.Equal(t, &chAccStore, &chAccService.chAccStore)
	})

	t.Run("GetChannelAccountStore() returns the existing chAccService if the current value is NOT empty", func(t *testing.T) {
		chAccService := ChannelAccountsService{TSSDBConnectionPool: dbConnectionPool}
		chAccService.chAccStore = &storeMocks.MockChannelAccountStore{}
		chAccStore := chAccService.GetChannelAccountStore()
		require.Equal(t, &chAccStore, &chAccService.chAccStore)
	})
}

func Test_ChannelAccounts_CreateAccount_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	chAccService := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	hostAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currLedgerNumber := 100
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNumber + preconditions.IncrementForMaxLedgerBounds),
	}

	publicKeys := []string{
		keypair.MustRandom().Address(),
		keypair.MustRandom().Address(),
	}

	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
		Return(horizon.Account{AccountID: hostAccount.Address()}, nil).
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, nil).
		Once()
	mLedgerNumberTracker.
		On("GetLedgerBounds").
		Return(ledgerBounds, nil).
		Once()
	mChannelAccountStore.
		On("Unlock", ctx, mock.Anything, mock.AnythingOfType("string")).
		Return(nil, nil).
		Twice()
	mDistAccResolver.
		On("HostDistributionAccount").
		Return(hostAccount.Address()).
		Twice()
	mHostAccSigClient.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()
	mChAccSigClient.
		On("BatchInsert", ctx, 2).
		Return(publicKeys, nil).
		Once().
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()

	err = chAccService.CreateChannelAccounts(ctx, 2)
	require.NoError(t, err)
}

func Test_ChannelAccounts_CreateAccount_CannotFindHostAccount_Failure(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, mDistAccResolver := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	hostAccount := keypair.MustParseFull("SDL4E4RF6BHX77DBKE63QC4H4LQG7S7D2PB4TSF64LTHDIHP7UUJHH2V")
	currLedgerNumber := 100

	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
		Return(horizon.Account{}, errors.New("some random error"))
	mDistAccResolver.
		On("HostDistributionAccount").
		Return(hostAccount.Address()).
		Once()

	err = cas.CreateChannelAccounts(ctx, currLedgerNumber)
	require.ErrorContains(t, err, "creating channel accounts onchain: failed to retrieve host account: horizon response error: some random error")
}

func Test_ChannelAccounts_CreateAccount_Insert_Failure(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, _, mDistAccResolver := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	hostAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")

	// current ledger number
	currLedgerNumber := 100
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNumber + preconditions.IncrementForMaxLedgerBounds),
	}

	defer mLedgerNumberTracker.AssertExpectations(t)

	mLedgerNumberTracker.
		On("GetLedgerBounds").Return(ledgerBounds, nil).Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
		Return(horizon.Account{AccountID: hostAccount.Address()}, nil)
	mDistAccResolver.
		On("HostDistributionAccount").
		Return(hostAccount.Address()).
		Once()
	mChAccSigClient.
		On("BatchInsert", ctx, 2).
		Return(nil, errors.New("failure inserting account"))

	err = cas.CreateChannelAccounts(ctx, 2)
	require.EqualError(t, err, "creating channel accounts onchain: failed to insert channel accounts into signature service: failure inserting account")
}

func Test_ChannelAccounts_VerifyAccounts_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	channelAccounts := []*store.ChannelAccount{
		{PublicKey: "GC3TKX2B6V7RSIU7UWNJ6MIA7PBTVBXGG7B43HYXRDLHB2DI6FVCYDE3"},
		{PublicKey: "GAV6VOD2JY6CYJ2XT7U4IH5HL5RJZXEDZFC7CQX5SR7SLLVOP3KPOFH2"},
	}

	ctx := context.Background()
	mChannelAccountStore.
		On("GetAll", ctx, dbConnectionPool, 0, 0).
		Return(channelAccounts, nil).
		Once()
	for _, acc := range channelAccounts {
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: acc.PublicKey}).
			Return(horizon.Account{AccountID: acc.PublicKey}, nil).
			Once()
	}

	deleteInvalidAcccounts := false
	err = cas.VerifyChannelAccounts(ctx, deleteInvalidAcccounts)
	require.NoError(t, err)
}

func Test_ChannelAccounts_VerifyAccounts_LoadChannelAccountsError_Failure(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	ctx := context.Background()
	mChannelAccountStore.
		On("GetAll", ctx, dbConnectionPool, 0, 0).
		Return(nil, errors.New("cannot load channel accounts from database")).
		Once()

	deleteInvalidAcccounts := false
	err = cas.VerifyChannelAccounts(ctx, deleteInvalidAcccounts)
	require.EqualError(
		t,
		err,
		"loading channel accounts from database in VerifyChannelAccounts: cannot load channel accounts from database",
	)
}

func Test_ChannelAccounts_VerifyAccounts_NotFound(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	channelAccounts := []*store.ChannelAccount{
		{PublicKey: "GC3TKX2B6V7RSIU7UWNJ6MIA7PBTVBXGG7B43HYXRDLHB2DI6FVCYDE3"},
		{PublicKey: "GAV6VOD2JY6CYJ2XT7U4IH5HL5RJZXEDZFC7CQX5SR7SLLVOP3KPOFH2"},
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
		mChannelAccountStore.
			On("Delete", ctx, dbConnectionPool, acc.PublicKey).
			Return(nil).
			Once()
	}

	getEntries := log.DefaultLogger.StartTest(log.WarnLevel)

	deleteInvalidAcccounts := true
	err = cas.VerifyChannelAccounts(ctx, deleteInvalidAcccounts)
	require.NoError(t, err)

	entries := getEntries()
	assert.Equal(t, len(entries), 2)
	for i, entry := range entries {
		assert.Equal(t, entry.Message, fmt.Sprintf("Account %s does not exist on the network", channelAccounts[i].PublicKey))
	}
}

func Test_ChannelAccounts_DeleteAccount_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
		PrivateKey: "YVeMG89DMl2Ku7IeGCumrvneDydfuW+2q4EKQoYhPRpKS/A1bKhNzAa7IjyLiA6UwTESsM6Hh8nactmuOfqUT38YVTx68CIgG6OuwCHPrmws57Tf",
	}

	hostAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")

	currLedgerNum := 100
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNum + preconditions.IncrementForMaxLedgerBounds),
	}

	mLedgerNumberTracker.
		On("GetLedgerNumber").Return(currLedgerNum, nil).Once().
		On("GetLedgerBounds").Return(ledgerBounds, nil).Once()
	mChannelAccountStore.
		On("GetAndLock", ctx, channelAccount.PublicKey, currLedgerNum, currLedgerNum+preconditions.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).
		Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: channelAccount.PublicKey}).
		Return(horizon.Account{}, nil).
		Once().
		On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
		Return(horizon.Account{AccountID: hostAccount.Address()}, nil).
		Once().
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, nil).
		Once()
	mDistAccResolver.
		On("HostDistributionAccount").
		Return(hostAccount.Address()).
		Twice()
	mHostAccSigClient.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()
	mChAccSigClient.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once().
		On("Delete", ctx, channelAccount.PublicKey).
		Return(nil).
		Once()

	err = cas.DeleteChannelAccount(ctx, DeleteChannelAccountsOptions{
		ChannelAccountID:  channelAccount.PublicKey,
		DeleteAllAccounts: false,
	})
	require.NoError(t, err)
}

func Test_ChannelAccounts_DeleteAccount_All_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
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

	hostAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")

	currLedgerNum := 1000
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNum + preconditions.IncrementForMaxLedgerBounds),
	}

	mChannelAccountStore.
		On("Count", ctx).
		Return(len(channelAccounts), nil).
		Once()
	mLedgerNumberTracker.
		On("GetLedgerNumber").Return(currLedgerNum, nil).Times(len(channelAccounts)).
		On("GetLedgerBounds").Return(ledgerBounds, nil).Times(len(channelAccounts))
	for _, acc := range channelAccounts {
		mChannelAccountStore.
			On("GetAndLockAll", ctx, currLedgerNum, currLedgerNum+preconditions.IncrementForMaxLedgerBounds, 1).
			Return([]*store.ChannelAccount{acc}, nil).
			Once()
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: acc.PublicKey}).
			Return(horizon.Account{}, nil).
			Once().
			On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
			Return(horizon.Account{AccountID: hostAccount.Address()}, nil).
			Once().
			On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Once()
		mDistAccResolver.
			On("HostDistributionAccount").
			Return(hostAccount.Address()).
			Twice()
		mHostAccSigClient.
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
			Return(&txnbuild.Transaction{}, nil).
			Once()
		mChAccSigClient.
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txnbuild.Transaction{}, nil).
			Once().
			On("Delete", ctx, acc.PublicKey).
			Return(nil).
			Once()
	}

	err = cas.DeleteChannelAccount(ctx, DeleteChannelAccountsOptions{DeleteAllAccounts: true})
	require.NoError(t, err)
}

func Test_ChannelAccounts_DeleteAccount_FindByPublicKey_Failure(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	channelAccountID := "GDKMLSJSPHFWB26JV7ESWLJAKJ6KDTLQWYFT2T4ZVXFFHWBINUEJKASM"

	currLedgerNum := 1000

	mLedgerNumberTracker.On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Once()
	mChannelAccountStore.
		On("GetAndLock", ctx, channelAccountID, currLedgerNum, currLedgerNum+preconditions.IncrementForMaxLedgerBounds).
		Return(nil, errors.New("db error")).
		Once()

	err = cas.DeleteChannelAccount(ctx, DeleteChannelAccountsOptions{ChannelAccountID: channelAccountID})
	require.ErrorContains(t,
		err,
		fmt.Sprintf("retrieving account %s from database in DeleteChannelAccount: db error", channelAccountID),
	)
}

func Test_ChannelAccounts_DeleteAccount_DeleteFromSigServiceError(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, _, _ := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GAMWDQPPO3MXDQHZWYQLCQMKMBVDDCV7WIRKLCALWJPI7MIQHYNERTXS",
		PrivateKey: "SBS2DJJSWZKKADWE4QEFN6CWXPM6KAFULKVJWO5VN7NIFDP6HFZXF6J7",
	}

	currLedgerNum := 1000

	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Once()
	mChannelAccountStore.
		On("GetAndLock", ctx, channelAccount.PublicKey, currLedgerNum, currLedgerNum+preconditions.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).
		Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: channelAccount.PublicKey}).
		Return(horizon.Account{}, horizonclient.Error{
			Problem: problem.P{
				Type: "https://stellar.org/horizon-errors/not_found",
			},
		}).
		Once()
	mChAccSigClient.
		On("Delete", ctx, channelAccount.PublicKey).
		Return(errors.New("sig service error")).
		Once()

	err = cas.DeleteChannelAccount(ctx, DeleteChannelAccountsOptions{ChannelAccountID: channelAccount.PublicKey})
	require.Error(t, err)
	require.ErrorContains(t, err, fmt.Sprintf(`deleting account %[1]s in DeleteChannelAccount: deleting %[1]s from signature service: sig service error`, channelAccount.PublicKey))
}

func Test_ChannelAccounts_DeleteAccount_SubmitTransaction_Failure(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
		PrivateKey: "SDHGNWPVZJML64GMSQFVX7RAZBJXO3SWOMEGV77IPXUMKHHEOFD2LC75",
	}

	hostAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")

	currLedgerNum := 1000
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNum + preconditions.IncrementForMaxLedgerBounds),
	}

	mLedgerNumberTracker.
		On("GetLedgerNumber").Return(currLedgerNum, nil).Once().
		On("GetLedgerBounds").Return(ledgerBounds, nil).Once()
	mChannelAccountStore.
		On("GetAndLock", ctx, channelAccount.PublicKey, currLedgerNum, currLedgerNum+preconditions.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).
		Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: channelAccount.PublicKey}).
		Return(horizon.Account{}, nil).
		Once().
		On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
		Return(horizon.Account{AccountID: hostAccount.Address()}, nil).
		Once().
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, errors.New("foo bar")).
		Once()
	mDistAccResolver.
		On("HostDistributionAccount").
		Return(hostAccount.Address()).
		Twice()
	mHostAccSigClient.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()
	mChAccSigClient.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()

	err = cas.DeleteChannelAccount(ctx, DeleteChannelAccountsOptions{ChannelAccountID: channelAccount.PublicKey})
	assert.ErrorContains(t, err, fmt.Sprintf(
		"deleting account %[1]s in DeleteChannelAccount: deleting account %[1]s onchain: submitting remove account transaction to the network for account %[1]s: horizon response error: foo bar",
		channelAccount.PublicKey,
	))
}

func Test_ChannelAccounts_EnsureChannelAccounts_Exact_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	ensureCount := 2

	mChannelAccountStore.
		On("Count", ctx).
		Return(ensureCount, nil).
		Once()
	getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

	err = cas.EnsureChannelAccountsCount(ctx, ensureCount)
	require.NoError(t, err)

	entries := getEntries()
	assert.Equal(t, entries[1].Message, fmt.Sprintf("✅ There are exactly %d managed channel accounts currently. Exiting...", ensureCount))

	mChannelAccountStore.AssertExpectations(t)
}

func Test_ChannelAccounts_EnsureChannelAccounts_Add_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	desiredCount := 5
	hostAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currChannelAccountsCount := 2

	currLedgerNum := 100
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNum + preconditions.IncrementForMaxLedgerBounds),
	}

	publicKeys := []string{
		keypair.MustRandom().Address(),
		keypair.MustRandom().Address(),
		keypair.MustRandom().Address(),
	}

	mChannelAccountStore.
		On("Count", ctx).
		Return(currChannelAccountsCount, nil).
		Once().
		On("Unlock", ctx, mock.Anything, mock.AnythingOfType("string")).
		Return(nil, nil).
		Times(desiredCount - currChannelAccountsCount)
	mLedgerNumberTracker.
		On("GetLedgerBounds").Return(ledgerBounds, nil).Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
		Return(horizon.Account{AccountID: hostAccount.Address()}, nil).
		Once().
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, nil).
		Once()
	mDistAccResolver.
		On("HostDistributionAccount").
		Return(hostAccount.Address()).
		Twice()
	mHostAccSigClient.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()
	mChAccSigClient.
		On("BatchInsert", ctx, desiredCount-currChannelAccountsCount).
		Return(publicKeys, nil).
		Once().
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()

	err = cas.EnsureChannelAccountsCount(ctx, desiredCount)
	require.NoError(t, err)
}

func Test_ChannelAccounts_EnsureChannelAccounts_Delete_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mChannelAccountStore := storeMocks.NewMockChannelAccountStore(t)
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, mChAccSigClient, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		TSSDBConnectionPool: dbConnectionPool,
		SubmitterEngine: engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			LedgerNumberTracker: mLedgerNumberTracker,
			MaxBaseFee:          100,
			SignatureService:    sigService,
		},
	}

	hostAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
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

	wantEnsureCount := 2
	currLedgerNum := 1000
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNum + preconditions.IncrementForMaxLedgerBounds),
	}

	mChannelAccountStore.
		On("Count", ctx).
		Return(currChannelAccountsCount, nil).
		Once()
	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Times(currChannelAccountsCount-wantEnsureCount).
		On("GetLedgerBounds").
		Return(ledgerBounds, nil).
		Times(currChannelAccountsCount - wantEnsureCount)
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address()}).
		Return(horizon.Account{AccountID: hostAccount.Address()}, nil).
		Times(currChannelAccountsCount - wantEnsureCount)

	for _, acc := range channelAccounts {
		mChannelAccountStore.
			On("GetAndLockAll", ctx, currLedgerNum, currLedgerNum+preconditions.IncrementForMaxLedgerBounds, 1).
			Return([]*store.ChannelAccount{acc}, nil).
			Once()
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: acc.PublicKey}).
			Return(horizon.Account{}, nil).
			Once()
		mDistAccResolver.
			On("HostDistributionAccount").
			Return(hostAccount.Address()).
			Twice()
		mHostAccSigClient.
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
			Return(&txnbuild.Transaction{}, nil).
			Once()
		mChAccSigClient.
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string")).
			Return(&txnbuild.Transaction{}, nil).
			Once().
			On("Delete", ctx, acc.PublicKey).
			Return(nil).
			Once()
	}
	mHorizonClient.
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, nil).
		Times(currChannelAccountsCount - wantEnsureCount)

	err = cas.EnsureChannelAccountsCount(ctx, wantEnsureCount)
	require.NoError(t, err)
}

func Test_ChannelAccounts_ViewChannelAccounts_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	channelAccounts := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 3)

	getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

	err = ViewChannelAccounts(ctx, dbConnectionPool)
	require.NoError(t, err)

	entries := getEntries()
	for i, entry := range entries[1:] {
		assert.Equal(t, entry.Message, fmt.Sprintf("Found account %s", channelAccounts[i].PublicKey))
	}
}

func Test_ChannelAccounts_ViewChannelAccounts_LoadChannelAccountsError_Failure(t *testing.T) {
	ctx := context.Background()

	err := ViewChannelAccounts(ctx, nil)
	require.Error(t, err)
	require.EqualError(t, err, "db connection pool cannot be nil")
}
