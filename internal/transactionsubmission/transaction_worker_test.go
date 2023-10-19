package transactionsubmission

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	engineMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/mocks"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	sdpUtlis "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// getTransactionWorkerInstance is used to create a valid instance of the class TransactionWorker, which is needed in
// many tests in this file.
func getTransactionWorkerInstance(t *testing.T, dbConnectionPool db.DBConnectionPool) TransactionWorker {
	t.Helper()

	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	wantSubmitterEngine, err := engine.NewSubmitterEngine(&horizonclient.Client{
		HorizonURL: "https://horizon-testnet.stellar.org",
		HTTP:       httpclient.DefaultClient(),
	})
	require.NoError(t, err)

	distributionKP := keypair.MustRandom()
	wantSigService, err := engine.NewDefaultSignatureService(
		network.TestNetworkPassphrase,
		dbConnectionPool,
		distributionKP.Seed(),
		chAccModel,
		&utils.PrivateKeyEncrypterMock{},
		distributionKP.Seed(),
	)
	require.NoError(t, err)

	wantMaxBaseFee := 100

	return TransactionWorker{
		dbConnectionPool:   dbConnectionPool,
		txModel:            txModel,
		chAccModel:         chAccModel,
		engine:             wantSubmitterEngine,
		sigService:         wantSigService,
		maxBaseFee:         wantMaxBaseFee,
		crashTrackerClient: &crashtracker.MockCrashTrackerClient{},
	}
}

// createTxJobFixture is used to create the resoureces needed for a txJob, and return a txJob with these resources. It
// can be customized according with the parameters passed.
func createTxJobFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, shouldLock bool, currentLedger, lockedToLedger int) TxJob {
	t.Helper()
	var err error

	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)

	// Create txJob:
	tx := store.CreateTransactionFixture(t, ctx, dbConnectionPool, uuid.NewString(), "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX", store.TransactionStatusProcessing, 1)
	chAcc := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 1)[0]

	if shouldLock {
		tx, err = txModel.Lock(ctx, dbConnectionPool, tx.ID, int32(currentLedger), int32(lockedToLedger))
		require.NoError(t, err)
		assert.True(t, tx.IsLocked(int32(currentLedger)))

		chAcc, err = chAccModel.Lock(ctx, dbConnectionPool, chAcc.PublicKey, int32(currentLedger), int32(lockedToLedger))
		require.NoError(t, err)
		assert.True(t, chAcc.IsLocked(int32(currentLedger)))
	}

	return TxJob{ChannelAccount: *chAcc, Transaction: *tx, LockedUntilLedgerNumber: lockedToLedger}
}

func Test_NewTransactionWorker(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	wantSubmitterEngine, err := engine.NewSubmitterEngine(&horizonclient.Client{
		HorizonURL: "https://horizon-testnet.stellar.org",
		HTTP:       httpclient.DefaultClient(),
	})
	require.NoError(t, err)

	distributionKP := keypair.MustRandom()
	wantSigService, err := engine.NewDefaultSignatureService(
		network.TestNetworkPassphrase,
		dbConnectionPool,
		distributionKP.Seed(),
		chAccModel,
		&utils.PrivateKeyEncrypterMock{},
		distributionKP.Seed(),
	)
	require.NoError(t, err)

	wantMaxBaseFee := 100
	wantTxProcessingLimiter := engine.NewTransactionProcessingLimiter(20)

	tssMonitorSvc := tssMonitor.TSSMonitorService{
		GitCommitHash: "gitCommitHash0x",
		Version:       "version123",
	}

	wantWorker := TransactionWorker{
		dbConnectionPool:    dbConnectionPool,
		txModel:             txModel,
		chAccModel:          chAccModel,
		engine:              wantSubmitterEngine,
		sigService:          wantSigService,
		maxBaseFee:          wantMaxBaseFee,
		crashTrackerClient:  &crashtracker.MockCrashTrackerClient{},
		txProcessingLimiter: wantTxProcessingLimiter,
		monitorSvc:          tssMonitorSvc,
	}

	testCases := []struct {
		name                string
		dbConnectionPool    db.DBConnectionPool
		txModel             *store.TransactionModel
		chAccModel          *store.ChannelAccountModel
		engine              *engine.SubmitterEngine
		sigService          engine.SignatureService
		maxBaseFee          int
		crashTrackerClient  crashtracker.CrashTrackerClient
		txProcessingLimiter *engine.TransactionProcessingLimiter
		monitorSvc          tssMonitor.TSSMonitorService
		wantError           error
	}{
		{
			name:      "validate dbConnectionPool",
			wantError: fmt.Errorf("dbConnectionPool cannot be nil"),
		},
		{
			name:             "validate txModel",
			dbConnectionPool: dbConnectionPool,
			wantError:        fmt.Errorf("txModel cannot be nil"),
		},
		{
			name:             "validate chAccModel",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			wantError:        fmt.Errorf("chAccModel cannot be nil"),
		},
		{
			name:             "validate engine",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			wantError:        fmt.Errorf("engine cannot be nil"),
		},
		{
			name:             "validate sigService",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine:           wantSubmitterEngine,
			wantError:        fmt.Errorf("sigService cannot be nil"),
		},
		{
			name:             "validate maxBaseFee",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine:           wantSubmitterEngine,
			sigService:       wantSigService,
			wantError:        fmt.Errorf("maxBaseFee must be greater than or equal to 100"),
		},
		{
			name:             "validate crashTrackerClient",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine:           wantSubmitterEngine,
			sigService:       wantSigService,
			maxBaseFee:       wantMaxBaseFee,
			wantError:        fmt.Errorf("crashTrackerClient cannot be nil"),
		},
		{
			name:               "validate txProcessingLimiter",
			dbConnectionPool:   dbConnectionPool,
			txModel:            txModel,
			chAccModel:         chAccModel,
			engine:             wantSubmitterEngine,
			sigService:         wantSigService,
			maxBaseFee:         wantMaxBaseFee,
			crashTrackerClient: &crashtracker.MockCrashTrackerClient{},
			wantError:          fmt.Errorf("txProcessingLimiter cannot be nil"),
		},
		{
			name:                "🎉 successfully returns a new transaction worker",
			dbConnectionPool:    dbConnectionPool,
			txModel:             txModel,
			chAccModel:          chAccModel,
			engine:              wantSubmitterEngine,
			sigService:          wantSigService,
			maxBaseFee:          wantMaxBaseFee,
			crashTrackerClient:  &crashtracker.MockCrashTrackerClient{},
			txProcessingLimiter: wantTxProcessingLimiter,
			monitorSvc:          tssMonitorSvc,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotWorker, err := NewTransactionWorker(
				tc.dbConnectionPool,
				tc.txModel,
				tc.chAccModel,
				tc.engine,
				tc.sigService,
				tc.maxBaseFee,
				tc.crashTrackerClient,
				tc.txProcessingLimiter,
				tc.monitorSvc,
			)

			if tc.wantError != nil {
				require.Error(t, err)
				require.Equal(t, tc.wantError, err)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, gotWorker)
				require.Equal(t, wantWorker, gotWorker)
			}
		})
	}
}

func Test_TransactionWorker_handleSuccessfulTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	currentLedger := 1
	lockedToLedger := 2

	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)

	t.Run("returns an error if UpdateStatusToSuccess fails", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger)
		require.NotEmpty(t, txJob)

		// mock UpdateStatusToSuccess FAIL
		errReturned := fmt.Errorf("updating transaction status to TransactionStatusSuccess: foo")
		mockTxStore := &storeMocks.MockTransactionStore{}
		mockTxStore.
			On("UpdateStatusToSuccess", ctx, mock.AnythingOfType("store.Transaction")).
			Return(nil, errReturned).
			Once()
		mockTxStore.
			On("UpdateStellarTransactionXDRReceived", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txJob.Transaction, nil).
			Once()
		transactionWorker.txModel = mockTxStore

		mMonitorService := monitor.MockMonitorService{}
		tssMonitorService := tssMonitor.TSSMonitorService{
			Version:       "0.01",
			GitCommitHash: "0xABC",
		}
		mMonitorService.On("MonitorCounters", mock.Anything, mock.Anything).Return(nil)

		transactionWorker.monitorSvc = tssMonitorService

		// Run test:
		err := transactionWorker.handleSuccessfulTransaction(ctx, &txJob, horizon.Transaction{Successful: true})
		require.Error(t, err)
		wantErr := utils.NewTransactionStatusUpdateError("SUCCESS", txJob.Transaction.ID, false, errReturned)
		require.Equal(t, wantErr, err)

		mockTxStore.AssertExpectations(t)
	})

	t.Run("returns an error if ChannelAccountModel.Unlock fails", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger)
		require.NotEmpty(t, txJob)

		// mock UpdateStatusToSuccess ✅
		mockTxStore := &storeMocks.MockTransactionStore{}
		mockTxStore.
			On("UpdateStatusToSuccess", ctx, mock.AnythingOfType("store.Transaction")).
			Return(&store.Transaction{}, nil).
			Once()
		mockTxStore.
			On("UpdateStellarTransactionXDRReceived", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txJob.Transaction, nil).
			Once()
		transactionWorker.txModel = mockTxStore

		// mock channelAccount Unlock (FAIL)
		errReturned := fmt.Errorf("something went wrong")
		mockChAccStore := &storeMocks.MockChannelAccountStore{}
		mockChAccStore.
			On("Unlock", ctx, dbConnectionPool, mock.AnythingOfType("string")).
			Return(nil, errReturned).
			Once()
		transactionWorker.chAccModel = mockChAccStore

		// Run test:
		err := transactionWorker.handleSuccessfulTransaction(ctx, &txJob, horizon.Transaction{Successful: true})
		require.Error(t, err)
		wantErr := fmt.Errorf("unlocking job: %w", fmt.Errorf("unlocking channel account: %w", errReturned))
		require.Equal(t, wantErr, err)

		mockTxStore.AssertExpectations(t)
	})

	t.Run("returns an error TransactionModel.Unlock fails", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger)
		require.NotEmpty(t, txJob)

		// mock UpdateStatusToSuccess ✅
		mockTxStore := &storeMocks.MockTransactionStore{}
		mockTxStore.
			On("UpdateStellarTransactionXDRReceived", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txJob.Transaction, nil).
			Once()
		mockTxStore.
			On("UpdateStatusToSuccess", ctx, mock.AnythingOfType("store.Transaction")).
			Return(&store.Transaction{}, nil).
			Once()

		// mock channelAccount Unlock ✅
		mockChAccStore := &storeMocks.MockChannelAccountStore{}
		mockChAccStore.
			On("Unlock", ctx, dbConnectionPool, mock.AnythingOfType("string")).
			Return(&store.ChannelAccount{}, nil).
			Once()
		transactionWorker.chAccModel = mockChAccStore

		// mock TransactionModel.Unlock (FAIL)
		errReturned := fmt.Errorf("something went wrong")
		mockTxStore.
			On("Unlock", ctx, dbConnectionPool, mock.AnythingOfType("string")).
			Return(nil, errReturned).
			Once()
		transactionWorker.txModel = mockTxStore

		// Run test:
		err := transactionWorker.handleSuccessfulTransaction(ctx, &txJob, horizon.Transaction{Successful: true})
		require.Error(t, err)
		wantErr := fmt.Errorf("unlocking job: %w", fmt.Errorf("unlocking transaction: %w", errReturned))
		require.Equal(t, wantErr, err)

		mockTxStore.AssertExpectations(t)
	})

	t.Run("🎉 successfully handles a transaction success", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		require.NotEmpty(t, transactionWorker)

		mMonitorService := monitor.MockMonitorService{}
		tssMonitorService := tssMonitor.TSSMonitorService{
			Version:       "0.01",
			GitCommitHash: "0xABC",
		}
		mMonitorService.On("MonitorCounters", mock.Anything, mock.Anything).Return(nil)

		transactionWorker.monitorSvc = tssMonitorService

		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger)
		require.NotEmpty(t, txJob)

		// Run test:
		const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="
		err := transactionWorker.handleSuccessfulTransaction(ctx, &txJob, horizon.Transaction{Successful: true, ResultXdr: resultXDR})
		require.NoError(t, err)

		// Assert the final state of the transaction in the DB:
		tx, err := txModel.Get(ctx, txJob.Transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, store.TransactionStatusSuccess, tx.Status)
		assert.Equal(t, resultXDR, tx.XDRReceived.String)
		assert.False(t, tx.IsLocked(int32(currentLedger)))

		// Assert the final state of the channel account in the DB:
		chAcc, err := chAccModel.Get(ctx, dbConnectionPool, txJob.ChannelAccount.PublicKey, 0)
		require.NoError(t, err)
		assert.False(t, chAcc.IsLocked(int32(currentLedger)))
	})

	t.Run("if a transaction with successful=false is passed, we save the xdr and leave it to be checked on reconciliation", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		require.NotEmpty(t, transactionWorker)

		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger)
		require.NotEmpty(t, txJob)

		// Run test:
		const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="
		err := transactionWorker.handleSuccessfulTransaction(ctx, &txJob, horizon.Transaction{Successful: false, ResultXdr: resultXDR})
		require.EqualError(t, err, "transaction was not successful for some reason")

		// Assert the final state of the transaction in the DB:
		tx, err := txModel.Get(ctx, txJob.Transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, store.TransactionStatusProcessing, tx.Status)
		assert.Equal(t, resultXDR, tx.XDRReceived.String)
		assert.True(t, tx.IsLocked(int32(currentLedger)))

		// Assert the final state of the channel account in the DB:
		chAcc, err := chAccModel.Get(ctx, dbConnectionPool, txJob.ChannelAccount.PublicKey, 0)
		require.NoError(t, err)
		assert.True(t, chAcc.IsLocked(int32(currentLedger)))
	})
}

func Test_TransactionWorker_reconcileSubmittedTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	const currentLedger = 1
	const lockedToLedger = 2
	const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="

	transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
	require.NotEmpty(t, transactionWorker)

	testCases := []struct {
		name                       string
		horizonTxResponse          horizon.Transaction
		horizonTxError             error
		wantErrContains            string
		shouldBeMarkedAsSuccessful bool
		shouldBePushedBackToQueue  bool
	}{
		{
			name:              "🎉 successfully verifies the tx went through and marks it as successful",
			horizonTxResponse: horizon.Transaction{Successful: true, ResultXdr: resultXDR},
		},
		{
			name:                      "🎉 successfully verifies the tx failed and mark it for resubmission",
			horizonTxResponse:         horizon.Transaction{Successful: false},
			shouldBePushedBackToQueue: true,
		},
		{
			name:                      "🎉 check the transaction returns a 404, so we mark it for resubmission",
			horizonTxError:            horizonclient.Error{Problem: problem.P{Status: http.StatusNotFound}},
			shouldBePushedBackToQueue: true,
		},
		{
			name:                      "un unexpected error is returned, so we wrap and send to the caller",
			horizonTxError:            horizonclient.Error{Problem: problem.P{Status: http.StatusTooManyRequests}},
			shouldBePushedBackToQueue: false,
			wantErrContains:           "unexpected error: horizon response error: StatusCode=429",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
			const txHash = "3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889"
			const envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"

			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger)
			tx, err := transactionWorker.txModel.UpdateStellarTransactionHashAndXDRSent(ctx, txJob.Transaction.ID, txHash, envelopeXDR)
			require.NoError(t, err)
			txJob.Transaction = *tx

			// mock LedgerNumberTracker
			mockLedgerNumberTracker := &engineMocks.MockLedgerNumberTracker{}
			mockLedgerNumberTracker.On("GetLedgerNumber").Return(currentLedger, nil).Once()
			transactionWorker.engine.LedgerNumberTracker = mockLedgerNumberTracker

			// mock TransactionDetail
			hMock := &horizonclient.MockClient{}
			hMock.On("TransactionDetail", txHash).Return(tc.horizonTxResponse, tc.horizonTxError).Once()
			transactionWorker.engine.HorizonClient = hMock

			mMonitorService := monitor.MockMonitorService{}
			tssMonitorService := tssMonitor.TSSMonitorService{
				Version:       "0.01",
				GitCommitHash: "0xABC",
			}
			mMonitorService.On("MonitorCounters", mock.Anything, mock.Anything).Return(nil)

			transactionWorker.monitorSvc = tssMonitorService

			// Run test:
			err = transactionWorker.reconcileSubmittedTransaction(ctx, &txJob)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.shouldBeMarkedAsSuccessful {
				// Assert the final state of the transaction in the DB:
				tx, err := transactionWorker.txModel.Get(ctx, txJob.Transaction.ID)
				require.NoError(t, err)
				assert.Equal(t, store.TransactionStatusSuccess, tx.Status)
				assert.False(t, tx.IsLocked(int32(currentLedger)))

				// Assert the final state of the channel account in the DB:
				chAcc, err := transactionWorker.chAccModel.Get(ctx, dbConnectionPool, txJob.ChannelAccount.PublicKey, 0)
				require.NoError(t, err)
				assert.False(t, chAcc.IsLocked(int32(currentLedger)))
			}

			if tc.shouldBePushedBackToQueue {
				// Assert the final state of the transaction in the DB:
				tx, err := transactionWorker.txModel.Get(ctx, txJob.Transaction.ID)
				require.NoError(t, err)
				assert.Equal(t, store.TransactionStatusProcessing, tx.Status)
				assert.False(t, tx.IsLocked(int32(currentLedger)))

				// Assert the final state of the channel account in the DB:
				chAcc, err := transactionWorker.chAccModel.Get(ctx, dbConnectionPool, txJob.ChannelAccount.PublicKey, 0)
				require.NoError(t, err)
				assert.False(t, chAcc.IsLocked(int32(currentLedger)))
			}

			mockLedgerNumberTracker.AssertExpectations(t)
			hMock.AssertExpectations(t)
		})
	}
}

func Test_TransactionWorker_validateJob(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	const currentLedger int32 = 1
	const lockedToLedger int32 = 2

	testCases := []struct {
		name                       string
		initialTransactionStatus   store.TransactionStatus
		wantHorizonErrorStatusCode int
		shouldLockTx               bool
		shouldLockChAcc            bool
		wantErrContains            string
	}{
		{
			name:                     "returns an error if the initial transaction status is SUCCESS",
			initialTransactionStatus: store.TransactionStatusSuccess,
			wantErrContains:          "invalid transaction status: SUCCESS",
		},
		{
			name:                     "returns an error if the initial transaction status is ERROR",
			initialTransactionStatus: store.TransactionStatusError,
			wantErrContains:          "invalid transaction status: ERROR",
		},
		{
			name:                       "returns an error if horizon returns an error",
			wantHorizonErrorStatusCode: http.StatusBadGateway,
			initialTransactionStatus:   store.TransactionStatusProcessing,
			wantErrContains:            "getting current ledger number: ",
		},
		{
			name:                       "returns an error if job's tx is not locked",
			wantHorizonErrorStatusCode: http.StatusOK,
			initialTransactionStatus:   store.TransactionStatusProcessing,
			wantErrContains:            "transaction should be locked",
		},
		{
			name:                       "returns an error if job's channel account is not locked",
			wantHorizonErrorStatusCode: http.StatusOK,
			initialTransactionStatus:   store.TransactionStatusProcessing,
			shouldLockTx:               true,
			wantErrContains:            "channel account should be locked",
		},
		{
			name:                       "🎉 successfully validate job when the resources are locked, horizon works and status is supported (PROCESSING)",
			wantHorizonErrorStatusCode: http.StatusOK,
			initialTransactionStatus:   store.TransactionStatusProcessing,
			shouldLockTx:               true,
			shouldLockChAcc:            true,
		},
		{
			name:                       "🎉 successfully validate job when the resources are locked, horizon works and status is supported (PENDING)",
			wantHorizonErrorStatusCode: http.StatusOK,
			initialTransactionStatus:   store.TransactionStatusProcessing,
			shouldLockTx:               true,
			shouldLockChAcc:            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			hMock := &horizonclient.MockClient{}
			if tc.wantHorizonErrorStatusCode == http.StatusOK {
				hMock.On("Root").Return(horizon.Root{HorizonSequence: int32(currentLedger)}, nil).Once()
			} else if tc.wantHorizonErrorStatusCode != 0 {
				hMock.On("Root").Return(horizon.Root{}, horizonclient.Error{Problem: problem.P{Status: http.StatusBadGateway}}).Once()
			}

			// Create a transaction worker:
			submitterEngine, err := engine.NewSubmitterEngine(hMock)
			require.NoError(t, err)
			transactionWorker := &TransactionWorker{
				engine:     submitterEngine,
				txModel:    store.NewTransactionModel(dbConnectionPool),
				chAccModel: store.NewChannelAccountModel(dbConnectionPool),
			}

			// create txJob:
			txJob := createTxJobFixture(t, ctx, dbConnectionPool, false, int(currentLedger), int(lockedToLedger))

			// Update status for txJob.Transaction
			var updatedTx store.Transaction
			q := `UPDATE submitter_transactions SET status = $1 WHERE id = $2 RETURNING *`
			err = dbConnectionPool.GetContext(ctx, &updatedTx, q, tc.initialTransactionStatus, txJob.Transaction.ID)
			require.NoError(t, err)
			txJob.Transaction = updatedTx

			// Lock txJob Channel account and transaction:
			if tc.shouldLockTx {
				lockedTx, innerErr := transactionWorker.txModel.Lock(ctx, dbConnectionPool, txJob.Transaction.ID, currentLedger, lockedToLedger)
				require.NoError(t, innerErr)
				txJob.Transaction = *lockedTx
			}
			if tc.shouldLockChAcc {
				lockedChAcc, innerErr := transactionWorker.chAccModel.Lock(ctx, dbConnectionPool, txJob.ChannelAccount.PublicKey, currentLedger, lockedToLedger)
				require.NoError(t, innerErr)
				txJob.ChannelAccount = *lockedChAcc
			}

			// Run test:
			err = transactionWorker.validateJob(&txJob)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			hMock.AssertExpectations(t)
		})
	}
}

func Test_TransactionWorker_buildAndSignTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	const currentLedger = 1
	const lockedToLedger = 2
	const accountSequence = 123

	distributionKP := keypair.MustRandom()
	sigService, err := engine.NewDefaultSignatureService(
		network.TestNetworkPassphrase,
		dbConnectionPool,
		distributionKP.Seed(),
		store.NewChannelAccountModel(dbConnectionPool),
		&utils.PrivateKeyEncrypterMock{},
		distributionKP.Seed(),
	)
	require.NoError(t, err)

	testCases := []struct {
		name                    string
		assetCode               string
		assetIssuer             string
		getAccountResponseObj   horizon.Account
		getAccountResponseError *horizonclient.Error
		wantErrorContains       string
	}{
		{
			name:              "returns an error if the asset code is empty",
			wantErrorContains: "asset code cannot be empty",
		},
		{
			name:              "returns an error if the asset code is not XLM and the issuer is not valid",
			assetCode:         "USDC",
			assetIssuer:       "FOOBAR",
			wantErrorContains: "invalid asset issuer: FOOBAR",
		},
		{
			name:                    "return an error if the AccountDetail call fails",
			assetCode:               "USDC",
			assetIssuer:             "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			getAccountResponseObj:   horizon.Account{},
			getAccountResponseError: &horizonclient.Error{Problem: problem.P{Status: http.StatusTooManyRequests}},
			wantErrorContains:       "horizon response error: ",
		},
		{
			name:                  "🎉 successfully build and sign a transaction",
			assetCode:             "USDC",
			assetIssuer:           "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
		},
		{
			name:                  "🎉 successfully build and sign a transaction with native asset",
			assetCode:             "XLM",
			assetIssuer:           "",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger)
			txJob.Transaction.AssetCode = tc.assetCode
			txJob.Transaction.AssetIssuer = tc.assetIssuer

			// mock horizon
			mockHorizon := &horizonclient.MockClient{}
			if !sdpUtlis.IsEmpty(tc.getAccountResponseObj) || !sdpUtlis.IsEmpty(tc.getAccountResponseError) {
				var hErr error
				if tc.getAccountResponseError != nil {
					hErr = tc.getAccountResponseError
				}
				mockHorizon.On("AccountDetail", horizonclient.AccountRequest{AccountID: txJob.ChannelAccount.PublicKey}).Return(tc.getAccountResponseObj, hErr).Once()
			}
			mockStore := &storeMocks.MockChannelAccountStore{}
			mockStore.On("Get", ctx, mock.Anything, txJob.ChannelAccount.PublicKey, 0).Return(txJob.ChannelAccount, nil)

			// Create a transaction worker:
			submitterEngine := &engine.SubmitterEngine{HorizonClient: mockHorizon}
			transactionWorker := &TransactionWorker{
				engine:     submitterEngine,
				txModel:    store.NewTransactionModel(dbConnectionPool),
				chAccModel: store.NewChannelAccountModel(dbConnectionPool),
				sigService: sigService,
				maxBaseFee: 100,
			}

			// Run test:
			gotFeeBumpTx, err := transactionWorker.buildAndSignTransaction(context.Background(), &txJob)
			if tc.wantErrorContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrorContains)
				assert.Nil(t, gotFeeBumpTx)
			} else {
				require.NoError(t, err)
				require.NotNil(t, gotFeeBumpTx)

				// Check that the transaction was built correctly:
				var wantAsset txnbuild.Asset = txnbuild.NativeAsset{}
				if strings.ToUpper(txJob.Transaction.AssetCode) != "XLM" {
					wantAsset = txnbuild.CreditAsset{
						Code:   txJob.Transaction.AssetCode,
						Issuer: txJob.Transaction.AssetIssuer,
					}
				}
				wantInnerTx, err := txnbuild.NewTransaction(
					txnbuild.TransactionParams{
						SourceAccount: &txnbuild.SimpleAccount{
							AccountID: txJob.ChannelAccount.PublicKey,
							Sequence:  accountSequence,
						},
						Operations: []txnbuild.Operation{
							&txnbuild.Payment{
								SourceAccount: distributionKP.Address(),
								Amount:        strconv.FormatFloat(txJob.Transaction.Amount, 'f', 6, 32), // TODO find a better way to do this
								Destination:   txJob.Transaction.Destination,
								Asset:         wantAsset,
							},
						},
						BaseFee: int64(transactionWorker.maxBaseFee),
						Preconditions: txnbuild.Preconditions{
							TimeBounds:   txnbuild.NewTimeout(300),
							LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)},
						},
						IncrementSequenceNum: true,
					},
				)
				require.NoError(t, err)
				wantInnerTx, err = sigService.SignStellarTransaction(ctx, wantInnerTx, distributionKP.Address(), txJob.ChannelAccount.PublicKey)
				require.NoError(t, err)

				wantFeeBumpTx, err := txnbuild.NewFeeBumpTransaction(
					txnbuild.FeeBumpTransactionParams{
						Inner:      wantInnerTx,
						FeeAccount: distributionKP.Address(),
						BaseFee:    int64(transactionWorker.maxBaseFee),
					},
				)
				require.NoError(t, err)
				wantFeeBumpTx, err = sigService.SignFeeBumpStellarTransaction(ctx, wantFeeBumpTx, distributionKP.Address())
				require.NoError(t, err)
				assert.Equal(t, wantFeeBumpTx, gotFeeBumpTx)
			}

			mockHorizon.AssertExpectations(t)
		})
	}
}

func Test_TransactionWorker_submit(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)
	const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="

	testCases := []struct {
		name                       string
		horizonResponse            horizon.Transaction
		horizonError               error
		wantFinalTransactionStatus store.TransactionStatus
		wantFinalResultXDR         string
		txMarkAsError              bool
	}{
		{
			name:            "unrecoverable horizon error is handled and tx status is marked as ERROR",
			horizonResponse: horizon.Transaction{},
			horizonError: horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_failed",
							"operations":  []string{"op_underfunded"}, // <--- this should make the transaction be marked as ERROR
						},
					},
				},
			},
			wantFinalTransactionStatus: store.TransactionStatusError,
			txMarkAsError:              true,
		},
		{
			name:                       "successful horizon error is handled and tx status is marked as SUCCESS",
			horizonResponse:            horizon.Transaction{Successful: true, ResultXdr: resultXDR},
			horizonError:               nil,
			wantFinalTransactionStatus: store.TransactionStatusSuccess,
			wantFinalResultXDR:         resultXDR,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, 1, 2)
			feeBumpTx := &txnbuild.FeeBumpTransaction{}

			mockHorizonClient := &horizonclient.MockClient{}
			mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}
			if tc.txMarkAsError {
				mockCrashTrackerClient.On("LogAndReportErrors", ctx, utils.NewHorizonErrorWrapper(tc.horizonError), "transaction error - cannot be retried").Once()
			}

			txProcessingLimiter := engine.NewTransactionProcessingLimiter(15)
			mockHorizonClient.
				On("SubmitFeeBumpTransactionWithOptions", feeBumpTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
				Return(tc.horizonResponse, tc.horizonError).
				Once()
			transactionWorker := TransactionWorker{
				dbConnectionPool: dbConnectionPool,
				txModel:          txModel,
				chAccModel:       chAccModel,
				engine: &engine.SubmitterEngine{
					HorizonClient: mockHorizonClient,
				},
				crashTrackerClient:  mockCrashTrackerClient,
				txProcessingLimiter: txProcessingLimiter,
			}

			mMonitorService := monitor.MockMonitorService{}
			tssMonitorService := tssMonitor.TSSMonitorService{
				Version:       "0.01",
				GitCommitHash: "0xABC",
			}
			mMonitorService.On("MonitorCounters", mock.Anything, mock.Anything).Return(nil)

			transactionWorker.monitorSvc = tssMonitorService

			// make sure the tx's initial status is PROCESSING:
			refreshedTx, err := txModel.Get(ctx, txJob.Transaction.ID)
			require.NoError(t, err)
			require.Equal(t, store.TransactionStatusProcessing, refreshedTx.Status)
			assert.Equal(t, *refreshedTx, txJob.Transaction)

			err = transactionWorker.submit(ctx, &txJob, feeBumpTx)
			require.NoError(t, err)

			// make sure the tx's status is the expected one:
			refreshedTx, err = txModel.Get(ctx, txJob.Transaction.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantFinalTransactionStatus, refreshedTx.Status)
			assert.Equal(t, tc.wantFinalResultXDR, refreshedTx.XDRReceived.String)

			// check if the channel account was unlocked:
			refreshedChAcc, err := chAccModel.Get(ctx, dbConnectionPool, txJob.ChannelAccount.PublicKey, 0)
			require.NoError(t, err)
			assert.False(t, refreshedChAcc.IsLocked(int32(txJob.LockedUntilLedgerNumber)))

			mockHorizonClient.AssertExpectations(t)
			mockCrashTrackerClient.AssertExpectations(t)
		})
	}
}
