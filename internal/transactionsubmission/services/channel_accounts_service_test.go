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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	engineMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
)

func Test_ChannelAccountsService_validate(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mSigService := engineMocks.NewMockSignatureService(t)

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
			name: "TSSDBConnectionPool cannot be nil",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
			},
			wantError: "signing service cannot be nil",
		},
		{
			name: "HorizonURL cannot be empty",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SigningService:      mSigService,
			},
			wantError: "horizon url cannot be empty",
		},
		{
			name: "maxBaseFee must be greater than or equal to 100",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SigningService:      mSigService,
				HorizonURL:          "https://horizon-testnet.stellar.org",
			},
			wantError: "maxBaseFee must be greater than or equal to 100",
		},
		{
			name: "advisory lock with ID was unavailable",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SigningService:      mSigService,
				HorizonURL:          "https://horizon-testnet.stellar.org",
				MaxBaseFee:          100,
			},
			isAdvisoryLockUnavailable: true,
			wantError:                 "failed getting db advisory lock: advisory lock is unavailable",
		},
		{
			name: "🎉 Successfully validate service",
			serviceOptions: ChannelAccountsService{
				TSSDBConnectionPool: dbConnectionPool,
				SigningService:      mSigService,
				HorizonURL:          "https://horizon-testnet.stellar.org",
				MaxBaseFee:          100,
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

func Test_ChannelAccountsService_GetHorizonClient(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	t.Run("GetHorizonClient() instantiates a new Horizon Client if the current horizonClient value is empty", func(t *testing.T) {
		chAccService := ChannelAccountsService{HorizonURL: "https://horizon-testnet.stellar.org"}
		wantHorizonCLient := &horizonclient.Client{
			HorizonURL: "https://horizon-testnet.stellar.org",
			HTTP:       httpclient.DefaultClient(),
		}
		horizonClient := chAccService.GetHorizonClient()
		require.Equal(t, wantHorizonCLient, horizonClient)
		require.Equal(t, wantHorizonCLient, chAccService.horizonClient)
		require.NotEqual(t, &wantHorizonCLient, &chAccService.horizonClient)
		require.Equal(t, &horizonClient, &chAccService.horizonClient)
	})

	t.Run("GetHorizonClient() returns the existing horizonClient if the current value is NOT empty", func(t *testing.T) {
		chAccService := ChannelAccountsService{HorizonURL: "https://horizon-testnet.stellar.org"}
		chAccService.horizonClient = &horizonclient.MockClient{}
		horizonClient := chAccService.GetHorizonClient()
		require.Equal(t, &horizonClient, &chAccService.horizonClient)
	})
}

func Test_ChannelAccountsService_GetLedgerNumberTracker(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	t.Run("GetLedgerNumberTracker() instantiates a new LedgerNumberTracker if the current one is empty", func(t *testing.T) {
		chAccService := ChannelAccountsService{HorizonURL: "https://horizon-testnet.stellar.org"}
		wantLedgerNumberTracker, err := engine.NewLedgerNumberTracker(chAccService.GetHorizonClient())
		require.NoError(t, err)

		ledgerNumberTracker, err := chAccService.GetLedgerNumberTracker()
		require.NoError(t, err)

		require.Equal(t, wantLedgerNumberTracker, ledgerNumberTracker)
		require.Equal(t, wantLedgerNumberTracker, chAccService.ledgerNumberTracker)
		require.NotEqual(t, &wantLedgerNumberTracker, &chAccService.ledgerNumberTracker)
		require.Equal(t, &ledgerNumberTracker, &chAccService.ledgerNumberTracker)
	})

	t.Run("GetLedgerNumberTracker() returns the existing LedgerNumberTracker if the current value is NOT empty", func(t *testing.T) {
		chAccService := ChannelAccountsService{HorizonURL: "https://horizon-testnet.stellar.org"}
		chAccService.ledgerNumberTracker = &engineMocks.MockLedgerNumberTracker{}
		ledgerNumberTracker, err := chAccService.GetLedgerNumberTracker()
		require.NoError(t, err)
		require.Equal(t, &ledgerNumberTracker, &chAccService.ledgerNumberTracker)
	})
}

func Test_ChannelAccounts_CreateAccount_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	rootAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currLedgerNumber := 100

	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, nil).
		Once()
	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNumber, nil).
		Once()
	mChannelAccountStore.
		On("Unlock", ctx, mock.Anything, mock.AnythingOfType("string")).
		Return(nil, nil).
		Twice()
	mSigService.
		On("DistributionAccount").
		Return(rootAccount.Address()).
		Twice().
		On("BatchInsert", ctx, mock.AnythingOfType("[]*keypair.Full"), true, currLedgerNumber).
		Return(nil).
		Once().
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once()

	err = cas.CreateChannelAccounts(ctx, 2)
	require.NoError(t, err)
}

func Test_ChannelAccounts_CreateAccount_CannotFindRootAccount_Failure(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	rootAccount := keypair.MustParseFull("SDL4E4RF6BHX77DBKE63QC4H4LQG7S7D2PB4TSF64LTHDIHP7UUJHH2V")
	currLedgerNumber := 100

	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{}, errors.New("some random error"))
	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNumber, nil).
		Once()
	mSigService.
		On("DistributionAccount").
		Return(rootAccount.Address()).
		Once()

	err = cas.CreateChannelAccounts(ctx, currLedgerNumber)
	require.ErrorContains(t, err, "creating channel accounts onchain: failed to retrieve root account: some random error")
}

func Test_ChannelAccounts_CreateAccount_Insert_Failure(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	rootAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currLedgerNumber := 100

	mLedgerNumberTracker.
		On("GetLedgerNumber").Return(currLedgerNumber, nil).
		Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil)
	mSigService.
		On("DistributionAccount").
		Return(rootAccount.Address()).
		Once().
		On("BatchInsert", ctx, mock.AnythingOfType("[]*keypair.Full"), true, 100).
		Return(errors.New("failure inserting account"))

	err = cas.CreateChannelAccounts(ctx, 2)
	require.EqualError(t, err, "creating channel accounts onchain: failed to insert channel accounts into signature service: failure inserting account")
}

func Test_ChannelAccounts_VerifyAccounts_Success(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
		PrivateKey: "YVeMG89DMl2Ku7IeGCumrvneDydfuW+2q4EKQoYhPRpKS/A1bKhNzAa7IjyLiA6UwTESsM6Hh8nactmuOfqUT38YVTx68CIgG6OuwCHPrmws57Tf",
	}

	rootAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currLedgerNum := 100

	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNum, nil)
	mChannelAccountStore.
		On("GetAndLock", ctx, channelAccount.PublicKey, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).
		Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: channelAccount.PublicKey}).
		Return(horizon.Account{}, nil).
		Once().
		On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).
		Once().
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, nil).
		Once()
	mSigService.
		On("DistributionAccount").
		Return(rootAccount.Address()).
		Twice().
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(&txnbuild.Transaction{}, nil).
		Once().
		On("Delete", ctx, channelAccount.PublicKey, currLedgerNum+engine.IncrementForMaxLedgerBounds).
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
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

	rootAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currLedgerNum := 1000

	mChannelAccountStore.
		On("Count", ctx).
		Return(len(channelAccounts), nil).
		Once()
	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Times(len(channelAccounts))
	for _, acc := range channelAccounts {
		mChannelAccountStore.
			On("GetAndLockAll", ctx, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds, 1).
			Return([]*store.ChannelAccount{acc}, nil).
			Once()
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: acc.PublicKey}).
			Return(horizon.Account{}, nil).
			Once().
			On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
			Return(horizon.Account{AccountID: rootAccount.Address()}, nil).
			Once().
			On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).
			Once()
		mSigService.
			On("DistributionAccount").
			Return(rootAccount.Address()).
			Twice().
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txnbuild.Transaction{}, nil).
			Once().
			On("Delete", ctx, acc.PublicKey, currLedgerNum+engine.IncrementForMaxLedgerBounds).
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	channelAccountID := "GDKMLSJSPHFWB26JV7ESWLJAKJ6KDTLQWYFT2T4ZVXFFHWBINUEJKASM"

	currLedgerNum := 1000

	mLedgerNumberTracker.On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Once()
	mChannelAccountStore.
		On("GetAndLock", ctx, channelAccountID, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
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
		On("GetAndLock", ctx, channelAccount.PublicKey, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
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
	mSigService.
		On("Delete", ctx, channelAccount.PublicKey, currLedgerNum+engine.IncrementForMaxLedgerBounds).
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	channelAccount := &store.ChannelAccount{
		PublicKey:  "GDXSRISWI6ZVFVVOUU2DNKVHUYEJQZ63A37P6C5NGKXBROW5WW5W6HW3",
		PrivateKey: "SDHGNWPVZJML64GMSQFVX7RAZBJXO3SWOMEGV77IPXUMKHHEOFD2LC75",
	}

	rootAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currLedgerNum := 1000

	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Once()
	mChannelAccountStore.
		On("GetAndLock", ctx, channelAccount.PublicKey, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds).
		Return(channelAccount, nil).
		Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: channelAccount.PublicKey}).
		Return(horizon.Account{}, nil).
		Once().
		On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).
		Once().
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, errors.New("foo bar")).
		Once()
	mSigService.
		On("DistributionAccount").
		Return(rootAccount.Address()).
		Twice().
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	desiredCount := 5
	rootAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
	currChannelAccountsCount := 2
	currLedgerNum := 100

	mChannelAccountStore.
		On("Count", ctx).
		Return(currChannelAccountsCount, nil).
		Once().
		On("Unlock", ctx, mock.Anything, mock.AnythingOfType("string")).
		Return(nil, nil).
		Times(desiredCount - currChannelAccountsCount)
	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Once()
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).
		Once().
		On("SubmitTransactionWithOptions", mock.Anything, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
		Return(horizon.Transaction{}, nil).
		Once()
	mSigService.
		On("DistributionAccount").
		Return(rootAccount.Address()).
		Twice().
		On("BatchInsert", ctx, mock.AnythingOfType("[]*keypair.Full"), true, currLedgerNum).
		Return(nil).
		Once().
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
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

	mChannelAccountStore := &storeMocks.MockChannelAccountStore{}
	defer mChannelAccountStore.AssertExpectations(t)
	mHorizonClient := &horizonclient.MockClient{}
	defer mHorizonClient.AssertExpectations(t)
	mLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
	defer mLedgerNumberTracker.AssertExpectations(t)
	mSigService := engineMocks.NewMockSignatureService(t)
	defer mSigService.AssertExpectations(t)

	cas := ChannelAccountsService{
		chAccStore:          mChannelAccountStore,
		horizonClient:       mHorizonClient,
		TSSDBConnectionPool: dbConnectionPool,
		ledgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100,
		SigningService:      mSigService,
		HorizonURL:          "https://horizon-testnet.stellar.org",
	}

	rootAccount := keypair.MustParseFull("SBMW2WDSVTGT2N2PCBF3PV7WBOIKVTGGIEBUUYMDX3CKTDD5HY3UIHV4")
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
	wantEnsureCount := 2

	mChannelAccountStore.
		On("Count", ctx).
		Return(currChannelAccountsCount, nil).
		Once()
	mLedgerNumberTracker.
		On("GetLedgerNumber").
		Return(currLedgerNum, nil).
		Times(currChannelAccountsCount - wantEnsureCount)
	mHorizonClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: rootAccount.Address()}).
		Return(horizon.Account{AccountID: rootAccount.Address()}, nil).
		Times(currChannelAccountsCount - wantEnsureCount)

	for _, acc := range channelAccounts {
		mChannelAccountStore.
			On("GetAndLockAll", ctx, currLedgerNum, currLedgerNum+engine.IncrementForMaxLedgerBounds, 1).
			Return([]*store.ChannelAccount{acc}, nil).
			Once()
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: acc.PublicKey}).
			Return(horizon.Account{}, nil).
			Once()
		mSigService.
			On("DistributionAccount").
			Return(rootAccount.Address()).
			Twice().
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txnbuild.Transaction{}, nil).
			Once().
			On("Delete", ctx, acc.PublicKey, currLedgerNum+engine.IncrementForMaxLedgerBounds).
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
