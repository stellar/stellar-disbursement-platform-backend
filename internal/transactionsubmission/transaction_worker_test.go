package transactionsubmission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	sdpMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	engineMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// getTransactionWorkerInstance is used to create a valid instance of the class TransactionWorker, which is needed in
// many tests in this file.
func getTransactionWorkerInstance(t *testing.T, dbConnectionPool db.DBConnectionPool) TransactionWorker {
	t.Helper()

	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	hClient := &horizonclient.Client{
		HorizonURL: "https://horizon-testnet.stellar.org",
		HTTP:       httpclient.DefaultClient(),
	}
	ledgerNumberTracker, err := preconditions.NewLedgerNumberTracker(hClient)
	require.NoError(t, err)

	distributionKP := keypair.MustRandom()
	distAccount := schema.NewStellarEnvTransactionAccount(distributionKP.Address())

	mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccResolver.
		On("DistributionAccount", mock.Anything, mock.AnythingOfType("string")).
		Return(distAccount, nil).
		Maybe()

	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	sigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		NetworkPassphrase:           network.TestNetworkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		DistributionPrivateKey:      distributionKP.Seed(),
		ChAccEncryptionPassphrase:   chAccEncryptionPassphrase,
		LedgerNumberTracker:         preconditionsMocks.NewMockLedgerNumberTracker(t),
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionAccountResolver: mDistAccResolver,
	})
	require.NoError(t, err)

	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       hClient,
		LedgerNumberTracker: ledgerNumberTracker,
		SignatureService:    sigService,
		MaxBaseFee:          100,
	}

	return TransactionWorker{
		dbConnectionPool:   dbConnectionPool,
		txModel:            txModel,
		chAccModel:         chAccModel,
		engine:             &submitterEngine,
		crashTrackerClient: &crashtracker.MockCrashTrackerClient{},
		eventProducer:      &events.MockProducer{},
	}
}

var (
	encrypter                 = &sdpUtils.DefaultPrivateKeyEncrypter{}
	chAccEncryptionPassphrase = keypair.MustRandom().Seed()
)

// createTxJobFixture is used to create the resoureces needed for a txJob, and return a txJob with these resources. It
// can be customized according with the parameters passed.
func createTxJobFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, shouldLock bool, currentLedger, lockedToLedger int, tenantID string) TxJob {
	t.Helper()
	var err error

	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)

	// Create txJob:
	tx := store.CreateTransactionFixtureNew(t, ctx, dbConnectionPool, store.TransactionFixture{
		ExternalID:         uuid.NewString(),
		AssetCode:          "USDC",
		AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		DestinationAddress: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		Status:             store.TransactionStatusProcessing,
		Amount:             1,
		TenantID:           tenantID,
	})
	chAcc := store.CreateChannelAccountFixturesEncrypted(t, ctx, dbConnectionPool, encrypter, chAccEncryptionPassphrase, 1)[0]

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
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := &store.ChannelAccountModel{DBConnectionPool: dbConnectionPool}

	distributionKP := keypair.MustRandom()

	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	wantSigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		NetworkPassphrase:         network.TestNetworkPassphrase,
		DBConnectionPool:          dbConnectionPool,
		DistributionPrivateKey:    distributionKP.Seed(),
		ChAccEncryptionPassphrase: chAccEncryptionPassphrase,
		LedgerNumberTracker:       preconditionsMocks.NewMockLedgerNumberTracker(t),

		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
		DistributionAccountResolver: sigMocks.NewMockDistributionAccountResolver(t),
	})
	require.NoError(t, err)

	hClient := &horizonclient.Client{
		HorizonURL: "https://horizon-testnet.stellar.org",
		HTTP:       httpclient.DefaultClient(),
	}
	ledgerNumberTracker, err := preconditions.NewLedgerNumberTracker(hClient)
	require.NoError(t, err)
	wantSubmitterEngine := engine.SubmitterEngine{
		HorizonClient:       hClient,
		LedgerNumberTracker: ledgerNumberTracker,
		SignatureService:    wantSigService,
		MaxBaseFee:          100,
	}
	require.NoError(t, err)

	wantTxProcessingLimiter := engine.NewTransactionProcessingLimiter(20)

	tssMonitorSvc := tssMonitor.TSSMonitorService{
		GitCommitHash: "gitCommitHash0x",
		Version:       "version123",
	}

	wantWorker := TransactionWorker{
		dbConnectionPool:    dbConnectionPool,
		txModel:             txModel,
		chAccModel:          chAccModel,
		engine:              &wantSubmitterEngine,
		crashTrackerClient:  &crashtracker.MockCrashTrackerClient{},
		txProcessingLimiter: wantTxProcessingLimiter,
		monitorSvc:          tssMonitorSvc,
		eventProducer:       &events.MockProducer{},
	}

	testCases := []struct {
		name                string
		dbConnectionPool    db.DBConnectionPool
		txModel             *store.TransactionModel
		chAccModel          *store.ChannelAccountModel
		engine              *engine.SubmitterEngine
		sigService          signing.SignatureService
		maxBaseFee          int
		crashTrackerClient  crashtracker.CrashTrackerClient
		txProcessingLimiter engine.TransactionProcessingLimiter
		monitorSvc          tssMonitor.TSSMonitorService
		eventProducer       events.Producer
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
			name:             "validate engine: horizon client cannot be nil",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine:           &engine.SubmitterEngine{},
			wantError:        fmt.Errorf("validating engine: horizon client cannot be nil"),
		},
		{
			name:             "validate engine: ledger number tracker cannot be nil",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine: &engine.SubmitterEngine{
				HorizonClient: hClient,
			},
			wantError: fmt.Errorf("validating engine: ledger number tracker cannot be nil"),
		},
		{
			name:             "validate engine: sig service cannot be nil",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine: &engine.SubmitterEngine{
				HorizonClient:       hClient,
				LedgerNumberTracker: ledgerNumberTracker,
			},
			wantError: fmt.Errorf("validating engine: signature service cannot be empty"),
		},
		{
			name:             "validate engine: max base fee",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine: &engine.SubmitterEngine{
				HorizonClient:       hClient,
				LedgerNumberTracker: ledgerNumberTracker,
				SignatureService:    wantSigService,
			},
			wantError: fmt.Errorf("validating engine: maxBaseFee must be greater than or equal to 100"),
		},
		{
			name:             "validate crashTrackerClient",
			dbConnectionPool: dbConnectionPool,
			txModel:          txModel,
			chAccModel:       chAccModel,
			engine:           &wantSubmitterEngine,
			sigService:       wantSigService,
			wantError:        fmt.Errorf("crashTrackerClient cannot be nil"),
		},
		{
			name:               "validate txProcessingLimiter",
			dbConnectionPool:   dbConnectionPool,
			txModel:            txModel,
			chAccModel:         chAccModel,
			engine:             &wantSubmitterEngine,
			sigService:         wantSigService,
			crashTrackerClient: &crashtracker.MockCrashTrackerClient{},
			wantError:          fmt.Errorf("txProcessingLimiter cannot be nil"),
		},
		{
			name:                "🎉 successfully returns a new transaction worker",
			dbConnectionPool:    dbConnectionPool,
			txModel:             txModel,
			chAccModel:          chAccModel,
			engine:              &wantSubmitterEngine,
			sigService:          wantSigService,
			crashTrackerClient:  &crashtracker.MockCrashTrackerClient{},
			txProcessingLimiter: wantTxProcessingLimiter,
			monitorSvc:          tssMonitorSvc,
			eventProducer:       &events.MockProducer{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotWorker, err := NewTransactionWorker(
				tc.dbConnectionPool,
				tc.txModel,
				tc.chAccModel,
				tc.engine,
				tc.crashTrackerClient,
				tc.txProcessingLimiter,
				tc.monitorSvc,
				tc.eventProducer,
			)

			if tc.wantError != nil {
				require.Error(t, err)
				require.Equal(t, tc.wantError.Error(), err.Error())
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, gotWorker)
				require.NotEmpty(t, gotWorker.jobUUID)
				wantWorker.jobUUID = gotWorker.jobUUID
				require.Equal(t, wantWorker, gotWorker)
			}
		})
	}
}

func Test_TransactionWorker_updateContextLogger(t *testing.T) {
	testCases := []struct {
		name                   string
		preexistingTxHash      string
		preexistingXDRReceived string
		preexistingXDRSent     string
		additionalLogrusFields logrus.Fields
	}{
		{
			name: "without preexisting tx_hash nor XDR data",
		},
		{
			name:              "with preexisting tx_hash but no XDR data",
			preexistingTxHash: "tx_hash_123",
			additionalLogrusFields: logrus.Fields{
				"tx_hash": "tx_hash_123",
			},
		},
		{
			name:               "with preexisting tx_hash and XDRSent",
			preexistingTxHash:  "tx_hash_123",
			preexistingXDRSent: "xdr_sent_123",
			additionalLogrusFields: logrus.Fields{
				"tx_hash":  "tx_hash_123",
				"xdr_sent": "xdr_sent_123",
			},
		},
		{
			name:                   "with preexisting tx_hash and XDR data (sent and received)",
			preexistingTxHash:      "tx_hash_123",
			preexistingXDRSent:     "xdr_sent_123",
			preexistingXDRReceived: "xdr_received_123",
			additionalLogrusFields: logrus.Fields{
				"tx_hash":      "tx_hash_123",
				"xdr_sent":     "xdr_sent_123",
				"xdr_received": "xdr_received_123",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbt := dbtest.OpenWithTSSMigrationsOnly(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
			transactionWorker.monitorSvc = tssMonitor.TSSMonitorService{
				GitCommitHash: "gitCommitHash0x",
				Version:       "version123",
			}

			// Prepare the context
			ctx := context.Background()
			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, 1, 2, uuid.NewString())
			require.NotEmpty(t, txJob)
			if tc.preexistingTxHash != "" {
				txJob.Transaction.StellarTransactionHash = sql.NullString{Valid: true, String: tc.preexistingTxHash}
			}
			if tc.preexistingXDRSent != "" {
				txJob.Transaction.XDRSent = sql.NullString{Valid: true, String: tc.preexistingXDRSent}
			}
			if tc.preexistingXDRReceived != "" {
				txJob.Transaction.XDRReceived = sql.NullString{Valid: true, String: tc.preexistingXDRReceived}
			}

			updatedCtx := transactionWorker.updateContextLogger(ctx, &txJob)

			// Call the logger
			getEntries := log.Ctx(updatedCtx).StartTest(log.DebugLevel)
			log.Ctx(updatedCtx).Debug("FOO BAR")
			entries := getEntries()

			// Assert length of entries:
			require.Len(t, entries, 1)
			logText := entries[0].Message

			// Assert log text:
			assert.Contains(t, logText, "FOO BAR", "Main message text is missing")

			// Assert log data:
			wantLogData := logrus.Fields{
				"app_version":         "version123",
				"asset":               txJob.Transaction.AssetCode,
				"channel_account":     txJob.ChannelAccount.PublicKey,
				"created_at":          txJob.Transaction.CreatedAt.String(),
				"destination_account": txJob.Transaction.Destination,
				"event_id":            transactionWorker.jobUUID,
				"git_commit_hash":     "gitCommitHash0x",
				"tenant_id":           txJob.Transaction.TenantID,
				"tx_id":               txJob.Transaction.ID,
				"updated_at":          txJob.Transaction.UpdatedAt.String(),
			}
			for k, v := range tc.additionalLogrusFields {
				wantLogData[k] = v
			}
			logData := entries[0].Data
			wantLogData["pid"] = logData["pid"]
			assert.Equal(t, wantLogData, logData, "Missing key-value pair")
		})
	}
}

func Test_TransactionWorker_handleFailedTransaction_nonHorizonErrors(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	nonHorizonError := errors.New("non-horizon error")

	testCases := []struct {
		name         string
		hTxRespFn    func(txJob *TxJob) horizon.Transaction
		hErr         *utils.HorizonErrorWrapper
		setupMocksFn func(t *testing.T, tw *TransactionWorker, txJob *TxJob)
		errContains  []string
	}{
		{
			name: "saveResponseXDRIfPresent fails",
			hTxRespFn: func(txJob *TxJob) horizon.Transaction {
				return horizon.Transaction{
					ID:         "tx_id_123",
					ResultXdr:  "result_xdr",
					Successful: false,
					Account:    txJob.ChannelAccount.PublicKey,
				}
			},
			hErr: utils.NewHorizonErrorWrapper(nonHorizonError),
			setupMocksFn: func(t *testing.T, tw *TransactionWorker, txJob *TxJob) {
				// PART 1: mock UpdateStellarTransactionXDRReceived that'll be called in saveResponseXDRIfPresent
				mockTxStore := storeMocks.NewMockTransactionStore(t)
				mockTxStore.
					On("UpdateStellarTransactionXDRReceived", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
					Return(nil, errors.New("txModel error in UpdateStellarTransactionXDRReceived")).
					Once()
				tw.txModel = mockTxStore

				// PART 2: mock deferred LogAndMonitorTransaction
				mMonitorClient := monitorMocks.NewMockMonitorClient(t)
				mMonitorClient.
					On("MonitorCounters", sdpMonitor.PaymentErrorTag, mock.Anything).
					Return(nil).
					Once()
				tssMonitorService := tssMonitor.TSSMonitorService{
					Version:       "0.01",
					GitCommitHash: "0xABC",
					Client:        mMonitorClient,
				}
				tw.monitorSvc = tssMonitorService
			},
			errContains: []string{"saving response XDR", "updating XDRReceived", "txModel error in UpdateStellarTransactionXDRReceived"},
		},
		{
			name: "it's not a horizon error, and unlockJob fails",
			hTxRespFn: func(txJob *TxJob) horizon.Transaction {
				return horizon.Transaction{
					ID:         "tx_id_123",
					ResultXdr:  "result_xdr",
					Successful: false,
					Account:    txJob.ChannelAccount.PublicKey,
				}
			},
			hErr: utils.NewHorizonErrorWrapper(nonHorizonError),
			setupMocksFn: func(t *testing.T, tw *TransactionWorker, txJob *TxJob) {
				// PART 1: mock UpdateStellarTransactionXDRReceived that'll be called in saveResponseXDRIfPresent
				mockTxStore := storeMocks.NewMockTransactionStore(t)
				txJob.Transaction.XDRReceived = sql.NullString{Valid: true, String: "xdr_received_123"}
				mockTxStore.
					On("UpdateStellarTransactionXDRReceived", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
					Return(&txJob.Transaction, nil).
					Once()
				tw.txModel = mockTxStore

				// PART 2: mock Unlock that'll be called in unlockJob
				mockChAccStore := storeMocks.NewMockChannelAccountStore(t)
				mockChAccStore.
					On("Unlock", mock.Anything, mock.Anything, txJob.ChannelAccount.PublicKey).
					Return(nil, errors.New("chAccModel error in Unlock")).
					Once()
				tw.chAccModel = mockChAccStore

				// PART 3: mock deferred LogAndMonitorTransaction
				mMonitorClient := monitorMocks.NewMockMonitorClient(t)
				mMonitorClient.
					On("MonitorCounters", sdpMonitor.PaymentErrorTag, mock.Anything).
					Return(nil).
					Once()
				tssMonitorService := tssMonitor.TSSMonitorService{
					Version:       "0.01",
					GitCommitHash: "0xABC",
					Client:        mMonitorClient,
				}
				tw.monitorSvc = tssMonitorService
			},
			errContains: []string{"unlocking job", "unlocking channel account", "chAccModel error in Unlock"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
			transactionWorker.jobUUID = uuid.NewString()
			txJob := createTxJobFixture(t, context.Background(), dbConnectionPool, true, 1, 2, uuid.NewString())
			require.NotEmpty(t, txJob)

			// Setup mocks:
			tc.setupMocksFn(t, &transactionWorker, &txJob)

			// Run test:
			err := transactionWorker.handleFailedTransaction(context.Background(), &txJob, tc.hTxRespFn(&txJob), tc.hErr)

			// Assert:
			if tc.errContains != nil {
				require.Error(t, err)
				for i, wantErr := range tc.errContains {
					assert.Containsf(t, err.Error(), wantErr, "error with index %d and text %q was not found in %q", i, wantErr, err.Error())
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_TransactionWorker_handleFailedTransaction_errorsThatTriggerJitter(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name        string
		statusCode  int
		resultCodes map[string]interface{}
	}{
		{
			name:       "504 - timeout",
			statusCode: http.StatusGatewayTimeout,
		},
		{
			name:       "429 - Too Many Requests",
			statusCode: http.StatusTooManyRequests,
		},
		{
			name:       "400 (tx_insufficient_fee) - Bad Request",
			statusCode: http.StatusBadRequest,
			resultCodes: map[string]interface{}{
				"transaction": "tx_insufficient_fee",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			tw := getTransactionWorkerInstance(t, dbConnectionPool)
			tw.jobUUID = uuid.NewString()
			txJob := createTxJobFixture(t, context.Background(), dbConnectionPool, true, 1, 2, uuid.NewString())
			require.NotEmpty(t, txJob)

			// declare horizon error
			horizonError := horizonclient.Error{
				Problem: problem.P{
					Status: tc.statusCode,
					Extras: map[string]interface{}{
						"result_codes": tc.resultCodes,
					},
				},
			}
			hErr := utils.NewHorizonErrorWrapper(horizonError)

			// PART 1: mock UpdateStellarTransactionXDRReceived that will be called from saveResponseXDRIfPresent
			mockTxStore := storeMocks.NewMockTransactionStore(t)
			txJob.Transaction.XDRReceived = sql.NullString{Valid: true, String: "xdr_received_123"}
			mockTxStore.
				On("UpdateStellarTransactionXDRReceived", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
				Return(&txJob.Transaction, nil).
				Once()
			tw.txModel = mockTxStore
			// PART 2: mock Unlock(s), that will be called from unlockJob
			mockChAccStore := storeMocks.NewMockChannelAccountStore(t)
			mockChAccStore.
				On("Unlock", mock.Anything, mock.Anything, txJob.ChannelAccount.PublicKey).
				Return(nil, nil).
				Once()
			tw.chAccModel = mockChAccStore
			mockTxStore.
				On("Unlock", mock.Anything, mock.Anything, txJob.Transaction.ID).
				Return(nil, nil).
				Once()
			// PART 3: setup the jitter to be one error away from taking action
			txProcessingLimiter := engine.NewTransactionProcessingLimiter(100)
			txProcessingLimiter.IndeterminateResponsesCounter = engine.IndeterminateResponsesToleranceLimit - 1
			assert.Equal(t, 100, txProcessingLimiter.LimitValue())
			tw.txProcessingLimiter = txProcessingLimiter
			// PART 4: mock deferred LogAndMonitorTransaction
			mMonitorClient := monitorMocks.NewMockMonitorClient(t)
			mMonitorClient.
				On("MonitorCounters", sdpMonitor.PaymentErrorTag, mock.Anything).
				Return(nil).
				Once()
			tssMonitorService := tssMonitor.TSSMonitorService{
				Version:       "0.01",
				GitCommitHash: "0xABC",
				Client:        mMonitorClient,
			}
			tw.monitorSvc = tssMonitorService

			// Run test:
			hTransaction := horizon.Transaction{
				ID:         "tx_id_123",
				ResultXdr:  "result_xdr",
				Successful: false,
				Account:    txJob.ChannelAccount.PublicKey,
			}
			err := tw.handleFailedTransaction(context.Background(), &txJob, hTransaction, hErr)
			require.NoError(t, err)

			// Assert that the jitter took action
			var ok bool
			txProcessingLimiter, ok = tw.txProcessingLimiter.(*engine.TransactionProcessingLimiterImpl)
			require.True(t, ok)
			assert.Equal(t, engine.DefaultBundlesSelectionLimit, txProcessingLimiter.LimitValue())
		})
	}
}

func Test_TransactionWorker_handleFailedTransaction_markedAsDefinitiveError(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	crashTrackerMessage := "transaction error - cannot be retried"

	testCases := []struct {
		name            string
		resultCodes     map[string]interface{}
		crashTrackerMsg string
	}{
		// - 400: with any of the transaction error codes [tx_bad_auth, tx_bad_auth_extra, tx_insufficient_balance]
		{
			name:            "400 (tx_bad_auth) - Bad Request",
			resultCodes:     map[string]interface{}{"transaction": "tx_bad_auth"},
			crashTrackerMsg: crashTrackerMessage,
		},
		{
			name:            "400 (tx_bad_auth_extra) - Bad Request",
			resultCodes:     map[string]interface{}{"transaction": "tx_bad_auth_extra"},
			crashTrackerMsg: crashTrackerMessage,
		},
		{
			name:            "400 (tx_insufficient_balance) - Bad Request",
			resultCodes:     map[string]interface{}{"transaction": "tx_insufficient_balance"},
			crashTrackerMsg: crashTrackerMessage,
		},
		// - 400: with any of the operation error codes [op_bad_auth, op_underfunded, op_src_not_authorized, op_no_destination, op_no_trust, op_line_full, op_not_authorized, op_no_issuer]
		{
			name:            "400 (op_bad_auth) - Bad Request",
			resultCodes:     map[string]interface{}{"operations": []string{"op_bad_auth"}},
			crashTrackerMsg: crashTrackerMessage,
		},
		{
			name:            "400 (op_underfunded) - Bad Request",
			resultCodes:     map[string]interface{}{"operations": []string{"op_underfunded"}},
			crashTrackerMsg: crashTrackerMessage,
		},
		{
			name:            "400 (op_src_not_authorized) - Bad Request",
			resultCodes:     map[string]interface{}{"operations": []string{"op_src_not_authorized"}},
			crashTrackerMsg: crashTrackerMessage,
		},
		{
			name:        "400 (op_no_destination) - Bad Request",
			resultCodes: map[string]interface{}{"operations": []string{"op_no_destination"}},
		},
		{
			name:        "400 (op_no_trust) - Bad Request",
			resultCodes: map[string]interface{}{"operations": []string{"op_no_trust"}},
		},
		{
			name:        "400 (op_line_full) - Bad Request",
			resultCodes: map[string]interface{}{"operations": []string{"op_line_full"}},
		},
		{
			name:        "400 (op_not_authorized) - Bad Request",
			resultCodes: map[string]interface{}{"operations": []string{"op_not_authorized"}},
		},
		{
			name:            "400 (op_no_issuer) - Bad Request",
			resultCodes:     map[string]interface{}{"operations": []string{"op_no_issuer"}},
			crashTrackerMsg: crashTrackerMessage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			tw := getTransactionWorkerInstance(t, dbConnectionPool)
			tw.jobUUID = uuid.NewString()

			txJob := createTxJobFixture(t, context.Background(), dbConnectionPool, true, 1, 2, uuid.NewString())
			const (
				resultXDR   = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="
				txHash      = "3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889"
				envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"
			)
			tx, err := tw.txModel.UpdateStellarTransactionHashAndXDRSent(ctx, txJob.Transaction.ID, txHash, envelopeXDR)
			require.NoError(t, err)
			txJob.Transaction = *tx

			// declare horizon error
			horizonError := horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{"result_codes": tc.resultCodes},
				},
			}
			hErr := utils.NewHorizonErrorWrapper(horizonError)

			// PART 1: mock call to jitter (TransactionProcessingLimiter)
			mockTxProcessingLimiter := engineMocks.NewMockTransactionProcessingLimiter(t)
			mockTxProcessingLimiter.On("AdjustLimitIfNeeded", hErr).Return().Once()
			tw.txProcessingLimiter = mockTxProcessingLimiter

			// PART 2: mock producer that'll be called in producePaymentCompletedEvent -> WriteMessages
			mockEventProducer := events.NewMockProducer(t)
			mockEventProducer.
				On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
				Run(func(args mock.Arguments) {
					messages, ok := args.Get(1).([]events.Message)
					require.True(t, ok)
					require.Len(t, messages, 1)

					msg := messages[0]

					assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
					assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
					assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
					assert.Equal(t, events.PaymentCompletedErrorType, msg.Type)

					msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
					require.True(t, ok)
					assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
					assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
					assert.Equal(t, string(data.FailedPaymentStatus), msgData.PaymentStatus)
					assert.Equal(t, hErr.Error(), msgData.PaymentStatusMessage)
					assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*200)
					assert.Equal(t, txHash, msgData.StellarTransactionID)
				}).
				Return(nil).
				Once()
			tw.eventProducer = mockEventProducer

			// PART 3: mock LogAndReportErrors
			if tc.crashTrackerMsg != "" {
				mockCrashTrackerClient := crashtracker.NewMockCrashTrackerClient(t)
				mockCrashTrackerClient.
					On("LogAndReportErrors", mock.Anything, hErr, tc.crashTrackerMsg).
					Return().
					Once()
				tw.crashTrackerClient = mockCrashTrackerClient
			}

			// PART 4: mock deferred LogAndMonitorTransaction
			mMonitorClient := monitorMocks.NewMockMonitorClient(t)
			mMonitorClient.
				On("MonitorCounters", sdpMonitor.PaymentErrorTag, mock.Anything).
				Return(nil).
				Once()
			tssMonitorService := tssMonitor.TSSMonitorService{
				Version:       "0.01",
				GitCommitHash: "0xABC",
				Client:        mMonitorClient,
			}
			tw.monitorSvc = tssMonitorService

			// Run test:
			hTransaction := horizon.Transaction{
				ID:          txHash,
				ResultXdr:   resultXDR,
				EnvelopeXdr: envelopeXDR,
				Successful:  false,
				Account:     txJob.ChannelAccount.PublicKey,
			}
			err = tw.handleFailedTransaction(context.Background(), &txJob, hTransaction, hErr)
			require.NoError(t, err)

			// Assert transaction status
			updatedTx, err := tw.txModel.Get(ctx, txJob.Transaction.ID)
			require.NoError(t, err)
			assert.Equal(t, store.TransactionStatusError, updatedTx.Status)
		})
	}
}

func Test_TransactionWorker_handleFailedTransaction_notDefinitiveErrorButTriggersCrashTracker(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
	defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

	tw := getTransactionWorkerInstance(t, dbConnectionPool)
	tw.jobUUID = uuid.NewString()

	txJob := createTxJobFixture(t, context.Background(), dbConnectionPool, true, 1, 2, uuid.NewString())
	const (
		resultXDR   = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="
		txHash      = "3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889"
		envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"
	)
	tx, err := tw.txModel.UpdateStellarTransactionHashAndXDRSent(ctx, txJob.Transaction.ID, txHash, envelopeXDR)
	require.NoError(t, err)
	txJob.Transaction = *tx
	// declare horizon error
	horizonError := horizonclient.Error{
		Problem: problem.P{
			Status: http.StatusBadRequest,
			Extras: map[string]interface{}{"result_codes": map[string]interface{}{
				"transaction": "tx_bad_seq",
			}},
		},
	}
	hErr := utils.NewHorizonErrorWrapper(horizonError)

	// PART 1: mock call to jitter (TransactionProcessingLimiter)
	mockTxProcessingLimiter := engineMocks.NewMockTransactionProcessingLimiter(t)
	mockTxProcessingLimiter.On("AdjustLimitIfNeeded", hErr).Return().Once()
	tw.txProcessingLimiter = mockTxProcessingLimiter

	// PART 2: mock LogAndReportErrors
	mockCrashTrackerClient := crashtracker.NewMockCrashTrackerClient(t)
	mockCrashTrackerClient.
		On("LogAndReportErrors", mock.Anything, hErr, "tx_bad_seq detected!").
		Return().
		Once()
	tw.crashTrackerClient = mockCrashTrackerClient

	// PART 3: mock deferred LogAndMonitorTransaction
	mMonitorClient := monitorMocks.NewMockMonitorClient(t)
	mMonitorClient.
		On("MonitorCounters", sdpMonitor.PaymentErrorTag, mock.Anything).
		Return(nil).
		Once()
	tssMonitorService := tssMonitor.TSSMonitorService{
		Version:       "0.01",
		GitCommitHash: "0xABC",
		Client:        mMonitorClient,
	}
	tw.monitorSvc = tssMonitorService

	// Run test:
	hTransaction := horizon.Transaction{
		ID:          txHash,
		ResultXdr:   resultXDR,
		EnvelopeXdr: envelopeXDR,
		Successful:  false,
		Account:     txJob.ChannelAccount.PublicKey,
	}
	err = tw.handleFailedTransaction(context.Background(), &txJob, hTransaction, hErr)
	require.NoError(t, err)

	// Assert transaction status
	updatedTx, err := tw.txModel.Get(ctx, txJob.Transaction.ID)
	require.NoError(t, err)
	assert.Equal(t, store.TransactionStatusProcessing, updatedTx.Status)
}

func Test_TransactionWorker_handleFailedTransaction_retryableErrorThatDoesntTriggerJitter(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name string
		hErr *utils.HorizonErrorWrapper
	}{
		// - 400 - tx_too_late
		{
			name: "400 (tx_too_late) - Bad Request",
			hErr: utils.NewHorizonErrorWrapper(horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadRequest,
					Extras: map[string]interface{}{
						"result_codes": map[string]interface{}{
							"transaction": "tx_too_late",
						},
					},
				},
			}),
		},
		// - 502 - unable to connect to horizon
		{
			name: "502 - Bad Gateway",
			hErr: utils.NewHorizonErrorWrapper(horizonclient.Error{
				Problem: problem.P{
					Status: http.StatusBadGateway,
				},
			}),
		},
		// unexpected error
		{
			name: "502 - Bad Gateway",
			hErr: utils.NewHorizonErrorWrapper(errors.New("foo bar error")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			tw := getTransactionWorkerInstance(t, dbConnectionPool)
			tw.jobUUID = uuid.NewString()

			txJob := createTxJobFixture(t, context.Background(), dbConnectionPool, true, 1, 2, uuid.NewString())
			const (
				resultXDR   = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="
				txHash      = "3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889"
				envelopeXDR = "AAAAAGL8HQvQkbK2HA3WVjRrKmjX00fG8sLI7m0ERwJW/AX3AAAACgAAAAAAAAABAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAArqN6LeOagjxMaUP96Bzfs9e0corNZXzBWJkFoK7kvkwAAAAAO5rKAAAAAAAAAAABVvwF9wAAAEAKZ7IPj/46PuWU6ZOtyMosctNAkXRNX9WCAI5RnfRk+AyxDLoDZP/9l3NvsxQtWj9juQOuoBlFLnWu8intgxQA"
			)
			tx, err := tw.txModel.UpdateStellarTransactionHashAndXDRSent(ctx, txJob.Transaction.ID, txHash, envelopeXDR)
			require.NoError(t, err)
			txJob.Transaction = *tx

			// PART 1: mock call to jitter (TransactionProcessingLimiter)
			if tc.hErr.IsHorizonError() {
				mockTxProcessingLimiter := engineMocks.NewMockTransactionProcessingLimiter(t)
				mockTxProcessingLimiter.On("AdjustLimitIfNeeded", tc.hErr).Return().Once()
				tw.txProcessingLimiter = mockTxProcessingLimiter
			}

			// PART 2: mock deferred LogAndMonitorTransaction
			mMonitorClient := monitorMocks.NewMockMonitorClient(t)
			mMonitorClient.
				On("MonitorCounters", sdpMonitor.PaymentErrorTag, mock.Anything).
				Return(nil).
				Once()
			tssMonitorService := tssMonitor.TSSMonitorService{
				Version:       "0.01",
				GitCommitHash: "0xABC",
				Client:        mMonitorClient,
			}
			tw.monitorSvc = tssMonitorService

			// Run test:
			hTransaction := horizon.Transaction{
				ID:          txHash,
				ResultXdr:   resultXDR,
				EnvelopeXdr: envelopeXDR,
				Successful:  false,
				Account:     txJob.ChannelAccount.PublicKey,
			}
			err = tw.handleFailedTransaction(context.Background(), &txJob, hTransaction, tc.hErr)
			require.NoError(t, err)

			// Assert transaction status
			updatedTx, err := tw.txModel.Get(ctx, txJob.Transaction.ID)
			require.NoError(t, err)
			assert.Equal(t, store.TransactionStatusProcessing, updatedTx.Status)
		})
	}
}

func Test_TransactionWorker_handleSuccessfulTransaction(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
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
		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, uuid.NewString())
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

		// Run test:
		err := transactionWorker.handleSuccessfulTransaction(ctx, &txJob, horizon.Transaction{Successful: true})
		require.Error(t, err)
		wantErr := utils.NewTransactionStatusUpdateError("SUCCESS", txJob.Transaction.ID, false, errReturned)
		require.Equal(t, wantErr, err)

		mockTxStore.AssertExpectations(t)
	})

	t.Run("returns an error if eventProducer.WriteMessages fails", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, uuid.NewString())
		require.NotEmpty(t, txJob)

		// mock UpdateStatusToSuccess ✅
		txJob.Transaction.Status = store.TransactionStatusSuccess
		mockTxStore := &storeMocks.MockTransactionStore{}
		mockTxStore.
			On("UpdateStatusToSuccess", ctx, mock.AnythingOfType("store.Transaction")).
			Return(&txJob.Transaction, nil).
			Once()
		mockTxStore.
			On("UpdateStellarTransactionXDRReceived", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txJob.Transaction, nil).
			Once()
		transactionWorker.txModel = mockTxStore

		// mock eventProducer WriteMessages (FAIL)
		errReturned := fmt.Errorf("something went wrong")
		mockEventProducer := &events.MockProducer{}
		mockEventProducer.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				messages, ok := args.Get(1).([]events.Message)
				require.True(t, ok)
				require.Len(t, messages, 1)

				msg := messages[0]

				assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
				assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
				assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
				assert.Equal(t, events.PaymentCompletedSuccessType, msg.Type)

				msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
				require.True(t, ok)
				assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
				assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
				assert.Equal(t, string(data.SuccessPaymentStatus), msgData.PaymentStatus)
				assert.Empty(t, msgData.PaymentStatusMessage)
				assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*100)
				assert.Empty(t, msgData.StellarTransactionID)
			}).
			Return(errReturned).
			Once()
		transactionWorker.eventProducer = mockEventProducer

		// Run test:
		expectedError := fmt.Sprintf(
			"producing payment completed event Status %s - Job %v: writing messages [Message{Topic: %s, Key: %s, Type: %s, TenantID: %s",
			store.TransactionStatusSuccess,
			txJob,
			events.PaymentCompletedTopic,
			txJob.Transaction.ExternalID,
			events.PaymentCompletedSuccessType,
			txJob.Transaction.TenantID,
		)
		err := transactionWorker.handleSuccessfulTransaction(ctx, &txJob, horizon.Transaction{Successful: true})
		assert.ErrorContains(t, err, expectedError)

		mockTxStore.AssertExpectations(t)
		mockEventProducer.AssertExpectations(t)
	})

	t.Run("returns an error if ChannelAccountModel.Unlock fails", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, uuid.NewString())
		require.NotEmpty(t, txJob)

		// mock UpdateStatusToSuccess ✅
		mockTxStore := &storeMocks.MockTransactionStore{}
		mockTxStore.
			On("UpdateStatusToSuccess", ctx, mock.AnythingOfType("store.Transaction")).
			Return(&txJob.Transaction, nil).
			Once()
		mockTxStore.
			On("UpdateStellarTransactionXDRReceived", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txJob.Transaction, nil).
			Once()
		transactionWorker.txModel = mockTxStore

		// mock eventProducer WriteMessages ✅
		mockEventProducer := &events.MockProducer{}
		mockEventProducer.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				messages, ok := args.Get(1).([]events.Message)
				require.True(t, ok)
				require.Len(t, messages, 1)

				msg := messages[0]

				assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
				assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
				assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
				assert.Equal(t, events.PaymentCompletedSuccessType, msg.Type)

				msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
				require.True(t, ok)
				assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
				assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
				assert.Equal(t, string(data.SuccessPaymentStatus), msgData.PaymentStatus)
				assert.Empty(t, msgData.PaymentStatusMessage)
				assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*100)
				assert.Empty(t, msgData.StellarTransactionID)
			}).
			Return(nil).
			Once()
		transactionWorker.eventProducer = mockEventProducer

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
		mockEventProducer.AssertExpectations(t)
	})

	t.Run("returns an error TransactionModel.Unlock fails", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, uuid.NewString())
		require.NotEmpty(t, txJob)

		// mock UpdateStatusToSuccess ✅
		mockTxStore := &storeMocks.MockTransactionStore{}
		mockTxStore.
			On("UpdateStellarTransactionXDRReceived", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
			Return(&txJob.Transaction, nil).
			Once()
		mockTxStore.
			On("UpdateStatusToSuccess", ctx, mock.AnythingOfType("store.Transaction")).
			Return(&txJob.Transaction, nil).
			Once()

		// mock eventProducer WriteMessages ✅
		mockEventProducer := &events.MockProducer{}
		mockEventProducer.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				messages, ok := args.Get(1).([]events.Message)
				require.True(t, ok)
				require.Len(t, messages, 1)

				msg := messages[0]

				assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
				assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
				assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
				assert.Equal(t, events.PaymentCompletedSuccessType, msg.Type)

				msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
				require.True(t, ok)
				assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
				assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
				assert.Equal(t, string(data.SuccessPaymentStatus), msgData.PaymentStatus)
				assert.Empty(t, msgData.PaymentStatusMessage)
				assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*100)
				assert.Empty(t, msgData.StellarTransactionID)
			}).
			Return(nil).
			Once()
		transactionWorker.eventProducer = mockEventProducer

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
		mockEventProducer.AssertExpectations(t)
	})

	t.Run("🎉 successfully handles a transaction success", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		require.NotEmpty(t, transactionWorker)

		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, uuid.NewString())
		require.NotEmpty(t, txJob)

		// mock eventProducer WriteMessages ✅
		mockEventProducer := &events.MockProducer{}
		mockEventProducer.
			On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
			Run(func(args mock.Arguments) {
				messages, ok := args.Get(1).([]events.Message)
				require.True(t, ok)
				require.Len(t, messages, 1)

				msg := messages[0]

				assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
				assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
				assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
				assert.Equal(t, events.PaymentCompletedSuccessType, msg.Type)

				msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
				require.True(t, ok)
				assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
				assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
				assert.Equal(t, string(data.SuccessPaymentStatus), msgData.PaymentStatus)
				assert.Empty(t, msgData.PaymentStatusMessage)
				assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*100)
				assert.Empty(t, msgData.StellarTransactionID)
			}).
			Return(nil).
			Once()
		transactionWorker.eventProducer = mockEventProducer

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

		mockEventProducer.AssertExpectations(t)
	})

	t.Run("if a transaction with successful=false is passed, we save the xdr and leave it to be checked on reconciliation", func(t *testing.T) {
		defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

		transactionWorker := getTransactionWorkerInstance(t, dbConnectionPool)
		require.NotEmpty(t, transactionWorker)

		txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, uuid.NewString())
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
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
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
			name:                      "🎉 successfully verifies the tx went through and marks it as successful",
			horizonTxResponse:         horizon.Transaction{Successful: true, ResultXdr: resultXDR},
			shouldBePushedBackToQueue: false,
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

			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, uuid.NewString())
			tx, err := transactionWorker.txModel.UpdateStellarTransactionHashAndXDRSent(ctx, txJob.Transaction.ID, txHash, envelopeXDR)
			require.NoError(t, err)
			txJob.Transaction = *tx

			// mock LedgerNumberTracker
			mockLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			mockLedgerNumberTracker.On("GetLedgerNumber").Return(currentLedger, nil).Once()
			transactionWorker.engine.LedgerNumberTracker = mockLedgerNumberTracker

			// mock TransactionDetail
			hMock := &horizonclient.MockClient{}
			hMock.On("TransactionDetail", txHash).Return(tc.horizonTxResponse, tc.horizonTxError).Once()
			transactionWorker.engine.HorizonClient = hMock

			mockEventProducer := &events.MockProducer{}
			if tc.horizonTxResponse.Successful {
				mockEventProducer.
					On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
					Run(func(args mock.Arguments) {
						messages, ok := args.Get(1).([]events.Message)
						require.True(t, ok)
						require.Len(t, messages, 1)

						msg := messages[0]

						assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
						assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
						assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
						assert.Equal(t, events.PaymentCompletedSuccessType, msg.Type)

						msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
						require.True(t, ok)
						assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
						assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
						assert.Equal(t, string(data.SuccessPaymentStatus), msgData.PaymentStatus)
						assert.Empty(t, msgData.PaymentStatusMessage)
						assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*100)
						assert.Equal(t, txHash, msgData.StellarTransactionID)
					}).
					Return(nil).
					Once()
			}
			transactionWorker.eventProducer = mockEventProducer

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
			mockEventProducer.AssertExpectations(t)
		})
	}
}

func Test_TransactionWorker_validateJob(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
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

			ledgerNumberTracker, err := preconditions.NewLedgerNumberTracker(hMock)
			require.NoError(t, err)

			// Create a transaction worker:
			submitterEngine := engine.SubmitterEngine{
				HorizonClient:       hMock,
				LedgerNumberTracker: ledgerNumberTracker,
			}
			transactionWorker := &TransactionWorker{
				engine:     &submitterEngine,
				txModel:    store.NewTransactionModel(dbConnectionPool),
				chAccModel: store.NewChannelAccountModel(dbConnectionPool),
			}

			// create txJob:
			txJob := createTxJobFixture(t, ctx, dbConnectionPool, false, int(currentLedger), int(lockedToLedger), uuid.NewString())

			// Update status for txJob.Transaction
			var updatedTx store.Transaction
			q := `UPDATE submitter_transactions SET status = $1 WHERE id = $2 RETURNING ` + store.TransactionColumnNames("", "")
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
	distAccount := schema.NewStellarEnvTransactionAccount(distributionKP.Address())

	mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccResolver.
		On("DistributionAccount", ctx, mock.AnythingOfType("string")).
		Return(distAccount, nil)

	distAccEncryptionPassphrase := keypair.MustRandom().Seed()
	sigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		NetworkPassphrase:         network.TestNetworkPassphrase,
		DBConnectionPool:          dbConnectionPool,
		DistributionPrivateKey:    distributionKP.Seed(),
		ChAccEncryptionPassphrase: chAccEncryptionPassphrase,
		LedgerNumberTracker:       preconditionsMocks.NewMockLedgerNumberTracker(t),

		DistributionAccountResolver: mDistAccResolver,
		DistAccEncryptionPassphrase: distAccEncryptionPassphrase,
	})
	require.NoError(t, err)

	testCases := []struct {
		name                    string
		assetCode               string
		assetIssuer             string
		getAccountResponseObj   horizon.Account
		getAccountResponseError *horizonclient.Error
		wantErrorContains       string
		destinationAddress      string
		memoType                schema.MemoType
		memoValue               string
		wantMemo                txnbuild.Memo
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
			name:                  "returns an error if memo is present for C destination",
			assetCode:             "USDC",
			assetIssuer:           "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
			destinationAddress:    "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
			memoType:              schema.MemoTypeText,
			memoValue:             "HelloWorld!",
			wantErrorContains:     "memo is not supported for contract destination",
		},
		{
			name:                  "🎉 successfully build and sign a payment transaction for G destination",
			assetCode:             "USDC",
			assetIssuer:           "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			destinationAddress:    "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
		},
		{
			name:                  "🎉 successfully build and sign a payment transaction with native asset for G destination",
			assetCode:             "XLM",
			assetIssuer:           "",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
			destinationAddress:    "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		},
		{
			name:                  "🎉 successfully build and sign a payment transaction with memo for G destination",
			assetCode:             "USDC",
			assetIssuer:           "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
			destinationAddress:    "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
			memoType:              schema.MemoTypeText,
			memoValue:             "HelloWorld!",
			wantMemo:              txnbuild.MemoText("HelloWorld!"),
		},
		{
			name:                  "🎉 successfully build and sign a SAC transfer transaction for C destination",
			assetCode:             "USDC",
			assetIssuer:           "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
			destinationAddress:    "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
			wantMemo:              nil,
		},
		{
			name:                  "🎉 successfully build and sign a SAC transfer transaction with native asset for C destination",
			assetCode:             "XLM",
			assetIssuer:           "",
			getAccountResponseObj: horizon.Account{Sequence: accountSequence},
			destinationAddress:    "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
			wantMemo:              nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
			defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

			tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "test-tenant", distributionKP.Address())
			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, currentLedger, lockedToLedger, tnt.ID)
			txJob.Transaction.AssetCode = tc.assetCode
			txJob.Transaction.AssetIssuer = tc.assetIssuer
			txJob.Transaction.Destination = tc.destinationAddress
			txJob.Transaction.Memo = tc.memoValue
			txJob.Transaction.MemoType = tc.memoType

			// mock horizon
			mockHorizon := &horizonclient.MockClient{}
			if !sdpUtils.IsEmpty(tc.getAccountResponseObj) || !sdpUtils.IsEmpty(tc.getAccountResponseError) {
				var hErr error
				if tc.getAccountResponseError != nil {
					hErr = tc.getAccountResponseError
				}
				mockHorizon.On("AccountDetail", horizonclient.AccountRequest{AccountID: txJob.ChannelAccount.PublicKey}).Return(tc.getAccountResponseObj, hErr).Once()
			}
			mockStore := &storeMocks.MockChannelAccountStore{}
			mockStore.On("Get", ctx, mock.Anything, txJob.ChannelAccount.PublicKey, 0).Return(txJob.ChannelAccount, nil)

			// Create a transaction worker:
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			submitterEngine := &engine.SubmitterEngine{
				HorizonClient:       mockHorizon,
				LedgerNumberTracker: mLedgerNumberTracker,
				SignatureService:    sigService,
				MaxBaseFee:          100,
			}
			transactionWorker := &TransactionWorker{
				engine:     submitterEngine,
				txModel:    store.NewTransactionModel(dbConnectionPool),
				chAccModel: store.NewChannelAccountModel(dbConnectionPool),
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

				var operation txnbuild.Operation
				amount := strconv.FormatFloat(txJob.Transaction.Amount, 'f', 6, 32)
				if strkey.IsValidEd25519PublicKey(tc.destinationAddress) {
					operation = &txnbuild.Payment{
						SourceAccount: distributionKP.Address(),
						Amount:        amount,
						Destination:   txJob.Transaction.Destination,
						Asset:         wantAsset,
					}
				} else if strkey.IsValidContractAddress(tc.destinationAddress) {
					params := txnbuild.PaymentToContractParams{
						NetworkPassphrase: network.TestNetworkPassphrase,
						Destination:       txJob.Transaction.Destination,
						Amount:            amount,
						Asset:             wantAsset,
						SourceAccount:     distributionKP.Address(),
					}
					op, _ := txnbuild.NewPaymentToContract(params)
					operation = &op
				}

				wantInnerTx, err := txnbuild.NewTransaction(
					txnbuild.TransactionParams{
						SourceAccount: &txnbuild.SimpleAccount{
							AccountID: txJob.ChannelAccount.PublicKey,
							Sequence:  accountSequence,
						},
						Memo:       tc.wantMemo,
						Operations: []txnbuild.Operation{operation},
						BaseFee:    int64(transactionWorker.engine.MaxBaseFee),
						Preconditions: txnbuild.Preconditions{
							TimeBounds:   txnbuild.NewTimeout(300),
							LedgerBounds: &txnbuild.LedgerBounds{MaxLedger: uint32(txJob.LockedUntilLedgerNumber)},
						},
						IncrementSequenceNum: true,
					},
				)
				require.NoError(t, err)
				chAccount := schema.NewDefaultChannelAccount(txJob.ChannelAccount.PublicKey)
				wantInnerTx, err = sigService.SignerRouter.SignStellarTransaction(ctx, wantInnerTx, chAccount, distAccount)
				require.NoError(t, err)

				wantFeeBumpTx, err := txnbuild.NewFeeBumpTransaction(
					txnbuild.FeeBumpTransactionParams{
						Inner:      wantInnerTx,
						FeeAccount: distributionKP.Address(),
						BaseFee:    int64(transactionWorker.engine.MaxBaseFee),
					},
				)
				require.NoError(t, err)
				wantFeeBumpTx, err = sigService.SignerRouter.SignFeeBumpStellarTransaction(ctx, wantFeeBumpTx, distAccount)
				require.NoError(t, err)
				assert.Equal(t, wantFeeBumpTx, gotFeeBumpTx)
			}

			mockHorizon.AssertExpectations(t)
		})
	}
}

func Test_TransactionWorker_submit(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	txModel := store.NewTransactionModel(dbConnectionPool)
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)
	const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="

	horizonError := horizonclient.Error{
		Problem: problem.P{
			Status: http.StatusBadRequest,
			Extras: map[string]interface{}{
				"result_codes": map[string]interface{}{
					"transaction": "tx_failed",
					"operations":  []string{"op_underfunded"}, // <--- this should make the transaction be marked as ERROR
				},
			},
		},
	}

	testCases := []struct {
		name                       string
		horizonResponse            horizon.Transaction
		horizonError               error
		wantFinalTransactionStatus store.TransactionStatus
		wantFinalResultXDR         string
		prepareMocks               func(*testing.T, TxJob, *crashtracker.MockCrashTrackerClient, *events.MockProducer)
	}{
		{
			name:                       "unrecoverable horizon error is handled and tx status is marked as ERROR",
			horizonResponse:            horizon.Transaction{},
			horizonError:               horizonError,
			wantFinalTransactionStatus: store.TransactionStatusError,
			prepareMocks: func(t *testing.T, txJob TxJob, mockCrashTrackerClient *crashtracker.MockCrashTrackerClient, mockEventProducer *events.MockProducer) {
				mockCrashTrackerClient.On("LogAndReportErrors", ctx, utils.NewHorizonErrorWrapper(horizonError), "transaction error - cannot be retried").Once()
				mockEventProducer.
					On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
					Run(func(args mock.Arguments) {
						messages, ok := args.Get(1).([]events.Message)
						require.True(t, ok)
						require.Len(t, messages, 1)

						msg := messages[0]

						assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
						assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
						assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
						assert.Equal(t, events.PaymentCompletedErrorType, msg.Type)

						msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
						require.True(t, ok)
						assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
						assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
						assert.Equal(t, string(data.FailedPaymentStatus), msgData.PaymentStatus)
						assert.Equal(t, "horizon response error: StatusCode=400, Extras=transaction: tx_failed - operation codes: [ op_underfunded ]", msgData.PaymentStatusMessage)
						assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*100)
						assert.Empty(t, msgData.StellarTransactionID)
					}).
					Return(nil).
					Once()
			},
		},
		{
			name:                       "successful horizon error is handled and tx status is marked as SUCCESS",
			horizonResponse:            horizon.Transaction{Successful: true, ResultXdr: resultXDR},
			horizonError:               nil,
			wantFinalTransactionStatus: store.TransactionStatusSuccess,
			wantFinalResultXDR:         resultXDR,
			prepareMocks: func(t *testing.T, txJob TxJob, _ *crashtracker.MockCrashTrackerClient, mockEventProducer *events.MockProducer) {
				mockEventProducer.
					On("WriteMessages", ctx, mock.AnythingOfType("[]events.Message")).
					Run(func(args mock.Arguments) {
						messages, ok := args.Get(1).([]events.Message)
						require.True(t, ok)
						require.Len(t, messages, 1)

						msg := messages[0]

						assert.Equal(t, events.PaymentCompletedTopic, msg.Topic)
						assert.Equal(t, txJob.Transaction.ExternalID, msg.Key)
						assert.Equal(t, txJob.Transaction.TenantID, msg.TenantID)
						assert.Equal(t, events.PaymentCompletedSuccessType, msg.Type)

						msgData, ok := msg.Data.(schemas.EventPaymentCompletedData)
						require.True(t, ok)
						assert.Equal(t, txJob.Transaction.ID, msgData.TransactionID)
						assert.Equal(t, txJob.Transaction.ExternalID, msgData.PaymentID)
						assert.Equal(t, string(data.SuccessPaymentStatus), msgData.PaymentStatus)
						assert.Empty(t, msgData.PaymentStatusMessage)
						assert.WithinDuration(t, time.Now(), msgData.PaymentCompletedAt, time.Millisecond*100)
					}).
					Return(nil).
					Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)

			txJob := createTxJobFixture(t, ctx, dbConnectionPool, true, 1, 2, uuid.NewString())
			feeBumpTx := &txnbuild.FeeBumpTransaction{}

			mockHorizonClient := &horizonclient.MockClient{}
			mockCrashTrackerClient := &crashtracker.MockCrashTrackerClient{}
			mockEventProducer := &events.MockProducer{}

			if tc.prepareMocks != nil {
				tc.prepareMocks(t, txJob, mockCrashTrackerClient, mockEventProducer)
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
				eventProducer:       mockEventProducer,
			}

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
			mockEventProducer.AssertExpectations(t)
		})
	}
}

func Test_TransactionWorker_buildPaymentCompletedEvent(t *testing.T) {
	transactionWorker := TransactionWorker{}

	t.Run("returns error when an unexpected payment status is passed", func(t *testing.T) {
		msg, err := transactionWorker.buildPaymentCompletedEvent(events.PaymentCompletedSuccessType, &store.Transaction{}, data.PendingPaymentStatus, "")
		assert.EqualError(t, err, "invalid payment status to produce payment completed event")
		assert.Nil(t, msg)
	})

	t.Run("🎉 successfully builds sync payment event of type=ERROR", func(t *testing.T) {
		tx := store.Transaction{
			ID:                     "tx-id",
			ExternalID:             "payment-id",
			TenantID:               "tenant-id",
			StellarTransactionHash: sql.NullString{},
		}

		msg, err := transactionWorker.buildPaymentCompletedEvent(events.PaymentCompletedErrorType, &tx, data.FailedPaymentStatus, "error status message")
		assert.NoError(t, err)

		gotPaymentCompletedAt := msg.Data.(schemas.EventPaymentCompletedData).PaymentCompletedAt
		assert.WithinDuration(t, time.Now(), gotPaymentCompletedAt, time.Millisecond*100)
		wantMsg := &events.Message{
			Topic:    events.PaymentCompletedTopic,
			Key:      tx.ExternalID,
			TenantID: tx.TenantID,
			Type:     events.PaymentCompletedErrorType,
			Data: schemas.EventPaymentCompletedData{
				TransactionID:        tx.ID,
				PaymentID:            tx.ExternalID,
				PaymentStatus:        string(data.FailedPaymentStatus),
				PaymentStatusMessage: "error status message",
				PaymentCompletedAt:   gotPaymentCompletedAt,
				StellarTransactionID: tx.StellarTransactionHash.String,
			},
		}
		assert.Equal(t, wantMsg, msg)
	})

	t.Run("🎉 successfully builds sync payment event of type=SUCCESS", func(t *testing.T) {
		tx := store.Transaction{
			ID:                     "tx-id",
			ExternalID:             "payment-id",
			TenantID:               "tenant-id",
			StellarTransactionHash: sql.NullString{},
		}

		msg, err := transactionWorker.buildPaymentCompletedEvent(events.PaymentCompletedSuccessType, &tx, data.SuccessPaymentStatus, "")
		assert.NoError(t, err)

		gotPaymentCompletedAt := msg.Data.(schemas.EventPaymentCompletedData).PaymentCompletedAt
		assert.WithinDuration(t, time.Now(), gotPaymentCompletedAt, time.Millisecond*100)
		wantMsg := &events.Message{
			Topic:    events.PaymentCompletedTopic,
			Key:      tx.ExternalID,
			TenantID: tx.TenantID,
			Type:     events.PaymentCompletedSuccessType,
			Data: schemas.EventPaymentCompletedData{
				TransactionID:        tx.ID,
				PaymentID:            tx.ExternalID,
				PaymentStatus:        string(data.SuccessPaymentStatus),
				PaymentCompletedAt:   gotPaymentCompletedAt,
				StellarTransactionID: tx.StellarTransactionHash.String,
			},
		}
		assert.Equal(t, wantMsg, msg)
	})
}
