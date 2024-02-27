package transactionsubmission

import (
	"context"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_SubmitterOptions_validate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	signatureService, _, _, _, distAccResolver := signing.NewMockSignatureService(t)
	mSubmitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		LedgerNumberTracker: mLedgerNumberTracker,
		SignatureService:    signatureService,
		MaxBaseFee:          txnbuild.MinBaseFee,
	}
	tssMonitorService := tssMonitor.TSSMonitorService{
		Client:        monitorMocks.NewMockMonitorClient(t),
		GitCommitHash: "gitCommitHash0x",
		Version:       "version123",
	}

	testCases := []struct {
		name             string
		wantErrContains  string
		submitterOptions SubmitterOptions
	}{
		{
			name:             "validate DBConnectionPool",
			submitterOptions: SubmitterOptions{},
			wantErrContains:  "database connection pool cannot be nil",
		},
		{
			name: "validate submitter engine's Horizon Client",
			submitterOptions: SubmitterOptions{
				DBConnectionPool: dbConnectionPool,
				SubmitterEngine:  engine.SubmitterEngine{},
			},
			wantErrContains: "validating submitter engine: horizon client cannot be nil",
		},
		{
			name: "validate submitter engine's Ledger Number Tracker",
			submitterOptions: SubmitterOptions{
				DBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient: mHorizonClient,
				},
			},
			wantErrContains: "validating submitter engine: ledger number tracker cannot be nil",
		},
		{
			name: "validate submitter engine's Signature Service",
			submitterOptions: SubmitterOptions{
				DBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
				},
			},
			wantErrContains: "validating submitter engine: signature service cannot be empty",
		},
		{
			name: "validate submitter engine's Signature Service",
			submitterOptions: SubmitterOptions{
				DBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
					SignatureService: signing.SignatureService{
						DistributionAccountResolver: distAccResolver,
					},
				},
			},
			wantErrContains: "validating submitter engine: validating signature service: channel account signer cannot be nil",
		},
		{
			name: "validate submitter engine's Max Base Fee",
			submitterOptions: SubmitterOptions{
				DBConnectionPool: dbConnectionPool,
				SubmitterEngine: engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
					SignatureService:    signatureService,
				},
			},
			wantErrContains: "validating submitter engine: maxBaseFee must be greater than or equal to",
		},
		{
			name: "validate NumChannelAccounts (min)",
			submitterOptions: SubmitterOptions{
				DBConnectionPool:   dbConnectionPool,
				SubmitterEngine:    mSubmitterEngine,
				NumChannelAccounts: 0,
			},
			wantErrContains: "num channel accounts must stay in the range from 1 to 1000",
		},
		{
			name: "validate NumChannelAccounts (max)",
			submitterOptions: SubmitterOptions{
				DBConnectionPool:   dbConnectionPool,
				SubmitterEngine:    mSubmitterEngine,
				NumChannelAccounts: 1001,
			},
			wantErrContains: "num channel accounts must stay in the range from 1 to 1000",
		},
		{
			name: "validate QueuePollingInterval",
			submitterOptions: SubmitterOptions{
				DBConnectionPool:   dbConnectionPool,
				SubmitterEngine:    mSubmitterEngine,
				NumChannelAccounts: 1,
			},
			wantErrContains: "queue polling interval must be greater than 6 seconds",
		},
		{
			name: "validate monitorService",
			submitterOptions: SubmitterOptions{
				DBConnectionPool:     dbConnectionPool,
				SubmitterEngine:      mSubmitterEngine,
				NumChannelAccounts:   1,
				QueuePollingInterval: 10,
			},
			wantErrContains: "monitor service cannot be nil",
		},
		{
			name: "🎉 successfully finishes validation with nil crash tracker client",
			submitterOptions: SubmitterOptions{
				DBConnectionPool:     dbConnectionPool,
				SubmitterEngine:      mSubmitterEngine,
				NumChannelAccounts:   1,
				QueuePollingInterval: 10,
				MonitorService:       tssMonitorService,
				EventProducer:        &events.MockProducer{},
			},
		},
		{
			name: "🎉 successfully finishes validation with existing crash tracker client",
			submitterOptions: SubmitterOptions{
				DBConnectionPool:     dbConnectionPool,
				SubmitterEngine:      mSubmitterEngine,
				NumChannelAccounts:   1,
				QueuePollingInterval: 10,
				MonitorService:       tssMonitorService,
				EventProducer:        &events.MockProducer{},
				CrashTrackerClient:   &crashtracker.MockCrashTrackerClient{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.submitterOptions.validate()
			if tc.wantErrContains == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_NewManager(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mHorizonClient := &horizonclient.MockClient{}
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigService, _, _, _, _ := signing.NewMockSignatureService(t)
	mSubmitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		LedgerNumberTracker: mLedgerNumberTracker,
		SignatureService:    sigService,
		MaxBaseFee:          txnbuild.MinBaseFee,
	}

	ctx := context.Background()
	validSubmitterOptions := SubmitterOptions{
		DBConnectionPool: dbConnectionPool,
		MonitorService: tssMonitor.TSSMonitorService{
			Client:        &monitorMocks.MockMonitorClient{},
			GitCommitHash: "0xABC",
			Version:       "0.01",
		},
		SubmitterEngine:      mSubmitterEngine,
		NumChannelAccounts:   5,
		QueuePollingInterval: 10,
		EventProducer:        &events.MockProducer{},
	}

	testCases := []struct {
		name                         string
		getSubmitterOptionsFn        func() SubmitterOptions
		numOfChannelAccountsToCreate int
		wantCrashTrackerClientFn     func() crashtracker.CrashTrackerClient
		wantErrContains              string
	}{
		{
			name:            "returns an error if the SubmitterOptions validation fails",
			wantErrContains: "validating options: ",
		},
		{
			name: "returns an error if the database connection cannot be opened",
			getSubmitterOptionsFn: func() SubmitterOptions {
				opts := validSubmitterOptions
				opts.DBConnectionPool = nil
				return opts
			},
			wantErrContains: "validating options: database connection pool cannot be nil",
		},
		{
			name:                  "returns an error if there are zero channel accounts in the database",
			getSubmitterOptionsFn: func() SubmitterOptions { return validSubmitterOptions },
			wantErrContains:       "no channel accounts found in the database, use the 'channel-accounts ensure' command to configure the number of accounts you want to use",
		},
		{
			name:                         "🎉 Successfully creates a submitter manager. Num of channel accounts intended is EXACT MATCH (Crash Tracker initially nil)",
			getSubmitterOptionsFn:        func() SubmitterOptions { return validSubmitterOptions },
			numOfChannelAccountsToCreate: 5,
			wantCrashTrackerClientFn: func() crashtracker.CrashTrackerClient {
				crashTrackerClient, innerErr := crashtracker.NewDryRunClient()
				require.NoError(t, innerErr)
				return crashTrackerClient
			},
		},
		{
			name: "🎉 Successfully creates a submitter manager. Num of channel accounts intended is EXACT MATCH (Crash Tracker initially not nil)",
			getSubmitterOptionsFn: func() SubmitterOptions {
				opts := validSubmitterOptions
				opts.CrashTrackerClient, err = crashtracker.NewDryRunClient()
				require.NoError(t, err)
				return opts
			},
			numOfChannelAccountsToCreate: 5,
		},
		{
			name:                         "🎉 Successfully creates a submitter manager. Num of channel accounts intended is SMALLER than intended (Crash Tracker initially nil)",
			numOfChannelAccountsToCreate: 1,
			getSubmitterOptionsFn: func() SubmitterOptions {
				opts := validSubmitterOptions
				opts.CrashTrackerClient, err = crashtracker.NewDryRunClient()
				require.NoError(t, err)
				return opts
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)

			// override empty options with the one from getSubmitterOptionsFn()
			submitterOptions := SubmitterOptions{}
			if tc.getSubmitterOptionsFn != nil {
				submitterOptions = tc.getSubmitterOptionsFn()
			}

			// create the channel accounts in the DB, if `tc.numOfChannelAccountsToCreate > 0`
			if tc.numOfChannelAccountsToCreate > 0 {
				_ = store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, tc.numOfChannelAccountsToCreate)
			}

			getLogEntries := log.DefaultLogger.StartTest(log.WarnLevel)
			gotManager, err := NewManager(ctx, submitterOptions)
			logEntries := getLogEntries()

			if tc.wantErrContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Nil(t, gotManager)

			} else {
				require.NoError(t, err)
				require.NotNil(t, gotManager)
				assert.NotEmpty(t, gotManager.dbConnectionPool)

				// Assert the resulting manager state:
				wantConnectionPool := gotManager.dbConnectionPool
				wantTxModel := &store.TransactionModel{DBConnectionPool: wantConnectionPool}
				wantChAccModel := &store.ChannelAccountModel{DBConnectionPool: wantConnectionPool}
				wantChTxBundleModel, err := store.NewChannelTransactionBundleModel(wantConnectionPool)
				require.NoError(t, err)

				wantSubmitterEngine := &engine.SubmitterEngine{
					HorizonClient:       mHorizonClient,
					LedgerNumberTracker: mLedgerNumberTracker,
					SignatureService:    sigService,
					MaxBaseFee:          txnbuild.MinBaseFee,
				}

				wantCrashTrackerClient := submitterOptions.CrashTrackerClient
				if tc.wantCrashTrackerClientFn != nil {
					wantCrashTrackerClient = tc.wantCrashTrackerClientFn()
				}

				wantEventProducer := &events.MockProducer{}

				txProcessingLimiter := engine.NewTransactionProcessingLimiter(submitterOptions.NumChannelAccounts)
				txProcessingLimiter.CounterLastUpdated = gotManager.txProcessingLimiter.CounterLastUpdated
				wantManager := &Manager{
					dbConnectionPool: wantConnectionPool,
					chAccModel:       wantChAccModel,
					txModel:          wantTxModel,
					chTxBundleModel:  wantChTxBundleModel,

					queueService: defaultQueueService{
						pollingInterval:    time.Duration(submitterOptions.QueuePollingInterval) * time.Second,
						numChannelAccounts: submitterOptions.NumChannelAccounts,
					},

					engine: wantSubmitterEngine,

					crashTrackerClient: wantCrashTrackerClient,
					monitorService:     submitterOptions.MonitorService,

					txProcessingLimiter: txProcessingLimiter,

					eventProducer: wantEventProducer,
				}
				assert.Equal(t, wantManager, gotManager)

				if tc.numOfChannelAccountsToCreate < submitterOptions.NumChannelAccounts {
					didFindExpectedLogEntry := false
					for _, logEntry := range logEntries {
						if strings.Contains(logEntry.Message, "The number of channel accounts in the database is smaller than expected") {
							didFindExpectedLogEntry = true
						}
					}
					assert.True(t, didFindExpectedLogEntry)
				}
			}
		})
	}
}

func Test_Manager_ProcessTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Signature service
	encrypter := &utils.DefaultPrivateKeyEncrypter{}
	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	distributionKP := keypair.MustRandom()
	sigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		DistributionSignerType:    signing.DistributionAccountEnvSignatureClientType,
		NetworkPassphrase:         network.TestNetworkPassphrase,
		DistributionPrivateKey:    distributionKP.Seed(),
		DBConnectionPool:          dbConnectionPool,
		ChAccEncryptionPassphrase: chAccEncryptionPassphrase,
		LedgerNumberTracker:       preconditionsMocks.NewMockLedgerNumberTracker(t),
		Encrypter:                 encrypter,
	})
	require.NoError(t, err)

	// Signal types
	type signalType string
	const (
		signalTypeCancel    signalType = "CANCEL"
		signalTypeOSSigterm signalType = "SIGTERM"
		signalTypeOSSigint  signalType = "SIGINT"
		signalTypeOSSigquit signalType = "SIGQUIT"
	)

	testCases := []struct {
		signalType signalType
	}{
		{signalTypeCancel},
		{signalTypeOSSigterm},
		{signalTypeOSSigint},
		{signalTypeOSSigquit},
	}

	for _, tc := range testCases {
		t.Run(string(tc.signalType), func(t *testing.T) {
			rawCtx := context.Background()
			ctx, cancel := context.WithCancel(rawCtx)

			defer store.DeleteAllFromChannelAccounts(t, rawCtx, dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, rawCtx, dbConnectionPool)
			defer tenant.DeleteAllTenantsFixture(t, rawCtx, dbConnectionPool)

			// Create channel accounts to be used by the tx submitter
			channelAccounts := store.CreateChannelAccountFixturesEncrypted(t, ctx, dbConnectionPool, encrypter, chAccEncryptionPassphrase, 2)
			assert.Len(t, channelAccounts, 2)
			channelAccountsMap := map[string]*store.ChannelAccount{}
			for _, ca := range channelAccounts {
				channelAccountsMap[ca.PublicKey] = ca
			}

			// Create transactions to be used by the tx submitter
			tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "test-tenant", distributionKP.Address())
			transactions := store.CreateTransactionFixturesNew(t, ctx, dbConnectionPool, 10, store.TransactionFixture{
				AssetCode:          "USDC",
				AssetIssuer:        "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				DestinationAddress: keypair.MustRandom().Address(),
				Status:             store.TransactionStatusPending,
				Amount:             1,
				TenantID:           tnt.ID,
			})

			assert.Len(t, transactions, 10)

			// mock ledger number tracker
			const currentLedgerNumber = 123
			mockLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			mockLedgerNumberTracker.On("GetLedgerNumber").Return(currentLedgerNumber, nil)

			// mock horizon client
			const sequenceNumber = 456
			mockHorizonClient := &horizonclient.MockClient{}
			mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{Sequence: sequenceNumber}, nil)
			mockChannelAccountStore := &storeMocks.MockChannelAccountStore{}
			for pubKey, ca := range channelAccountsMap {
				mockChannelAccountStore.On("Get", ctx, mock.Anything, pubKey, 0).Return(ca, nil)
			}
			const resultXDR = "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAAOAAAAAAAAAABw2JZZYIt4n/WXKcnDow3mbTBMPrOnldetgvGUlpTSEQAAAAA="
			mockHorizonClient.
				On("SubmitFeeBumpTransactionWithOptions", mock.AnythingOfType("*txnbuild.FeeBumpTransaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
				Return(horizon.Transaction{Successful: true, ResultXdr: resultXDR}, nil).Twice()
			defer mockHorizonClient.AssertExpectations(t)

			submitterEngine := &engine.SubmitterEngine{
				LedgerNumberTracker: mockLedgerNumberTracker,
				HorizonClient:       mockHorizonClient,
				SignatureService:    sigService,
				MaxBaseFee:          txnbuild.MinBaseFee,
			}

			dryRunCrashTracker, err := crashtracker.NewDryRunClient()
			require.NoError(t, err)

			queueService := defaultQueueService{
				pollingInterval:    500 * time.Millisecond,
				numChannelAccounts: 2,
			}

			chTxBundleModel, err := store.NewChannelTransactionBundleModel(dbConnectionPool)
			require.NoError(t, err)

			mMonitorClient := monitorMocks.NewMockMonitorClient(t)
			mMonitorClient.On("MonitorCounters", mock.Anything, mock.Anything).Return(nil).Times(3)

			mockEventProducer := &events.MockProducer{}
			mockEventProducer.On("WriteMessages", mock.Anything, mock.AnythingOfType("[]events.Message")).Return(nil).Once()
			defer mockEventProducer.AssertExpectations(t)

			manager := &Manager{
				dbConnectionPool:    dbConnectionPool,
				chTxBundleModel:     chTxBundleModel,
				chAccModel:          store.NewChannelAccountModel(dbConnectionPool),
				txModel:             store.NewTransactionModel(dbConnectionPool),
				engine:              submitterEngine,
				crashTrackerClient:  dryRunCrashTracker,
				queueService:        queueService,
				txProcessingLimiter: engine.NewTransactionProcessingLimiter(queueService.numChannelAccounts),
				monitorService: tssMonitor.TSSMonitorService{
					Client:        mMonitorClient,
					GitCommitHash: "gitCommitHash0x",
					Version:       "version123",
				},
				eventProducer: mockEventProducer,
			}

			go manager.ProcessTransactions(ctx)
			time.Sleep(750 * time.Millisecond) // <--- this time.Sleep is used wait for the manager (QueuePollingInterval) to start and load the transactions.
			// cancel()
			switch tc.signalType {
			case signalTypeOSSigterm:
				err = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
				require.NoError(t, err)

			case signalTypeOSSigint:
				err = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
				require.NoError(t, err)

			case signalTypeOSSigquit:
				err = syscall.Kill(syscall.Getpid(), syscall.SIGQUIT)
				require.NoError(t, err)

			default:
				// NO-OP
			}

			cancel()
		})
	}
}
