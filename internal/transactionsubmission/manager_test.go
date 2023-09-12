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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/mocks"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	storeMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_SubmitterOptions_validate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	testCases := []struct {
		name             string
		wantErrContains  string
		submitterOptions SubmitterOptions
	}{
		{
			name:             "validate DatabaseDSN",
			submitterOptions: SubmitterOptions{},
			wantErrContains:  "database DSN cannot be empty",
		},
		{
			name: "validate horizonURL",
			submitterOptions: SubmitterOptions{
				DatabaseDSN: dbt.DSN,
			},
			wantErrContains: "horizon url cannot be empty",
		},
		{
			name: "validate networkPassphrase",
			submitterOptions: SubmitterOptions{
				DatabaseDSN: dbt.DSN,
				HorizonURL:  "https://horizon-testnet.stellar.org",
			},
			wantErrContains: "network passphrase \"\" is invalid",
		},
		{
			name: "validate PrivateKeyEncrypter",
			submitterOptions: SubmitterOptions{
				DatabaseDSN:       dbt.DSN,
				HorizonURL:        "https://horizon-testnet.stellar.org",
				NetworkPassphrase: network.TestNetworkPassphrase,
			},
			wantErrContains: "private key encrypter cannot be nil",
		},
		{
			name: "validate DistributionSeed",
			submitterOptions: SubmitterOptions{
				DatabaseDSN:         dbt.DSN,
				HorizonURL:          "https://horizon-testnet.stellar.org",
				NetworkPassphrase:   network.TestNetworkPassphrase,
				PrivateKeyEncrypter: &utils.PrivateKeyEncrypterMock{},
			},
			wantErrContains: "distribution seed is invalid",
		},
		{
			name: "validate NumChannelAccounts (min)",
			submitterOptions: SubmitterOptions{
				DatabaseDSN:         dbt.DSN,
				HorizonURL:          "https://horizon-testnet.stellar.org",
				NetworkPassphrase:   network.TestNetworkPassphrase,
				PrivateKeyEncrypter: &utils.PrivateKeyEncrypterMock{},
				DistributionSeed:    "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
				NumChannelAccounts:  0,
			},
			wantErrContains: "num channel accounts must stay in the range from 1 to 1000",
		},
		{
			name: "validate NumChannelAccounts (min)",
			submitterOptions: SubmitterOptions{
				DatabaseDSN:         dbt.DSN,
				HorizonURL:          "https://horizon-testnet.stellar.org",
				NetworkPassphrase:   network.TestNetworkPassphrase,
				PrivateKeyEncrypter: &utils.PrivateKeyEncrypterMock{},
				DistributionSeed:    "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
				NumChannelAccounts:  1001,
			},
			wantErrContains: "num channel accounts must stay in the range from 1 to 1000",
		},
		{
			name: "validate QueuePollingInterval",
			submitterOptions: SubmitterOptions{
				DatabaseDSN:         dbt.DSN,
				HorizonURL:          "https://horizon-testnet.stellar.org",
				NetworkPassphrase:   network.TestNetworkPassphrase,
				PrivateKeyEncrypter: &utils.PrivateKeyEncrypterMock{},
				DistributionSeed:    "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
				NumChannelAccounts:  1,
			},
			wantErrContains: "queue polling interval must be greater than 6 seconds",
		},
		{
			name: "validate MaxBaseFee",
			submitterOptions: SubmitterOptions{
				DatabaseDSN:          dbt.DSN,
				HorizonURL:           "https://horizon-testnet.stellar.org",
				NetworkPassphrase:    network.TestNetworkPassphrase,
				PrivateKeyEncrypter:  &utils.PrivateKeyEncrypterMock{},
				DistributionSeed:     "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
				NumChannelAccounts:   1,
				QueuePollingInterval: 10,
			},
			wantErrContains: "max base fee must be greater than or equal to 100",
		},
		{
			name: "validate monitorService",
			submitterOptions: SubmitterOptions{
				DatabaseDSN:          dbt.DSN,
				MonitorService:       tssMonitor.TSSMonitorService{},
				HorizonURL:           "https://horizon-testnet.stellar.org",
				NetworkPassphrase:    network.TestNetworkPassphrase,
				PrivateKeyEncrypter:  &utils.PrivateKeyEncrypterMock{},
				DistributionSeed:     "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
				NumChannelAccounts:   1,
				QueuePollingInterval: 10,
				MaxBaseFee:           txnbuild.MinBaseFee,
			},
			wantErrContains: "monitor service cannot be nil",
		},
		{
			name: "🎉 successfully finishes validation with nil crash tracker client",
			submitterOptions: SubmitterOptions{
				DatabaseDSN: dbt.DSN,
				MonitorService: tssMonitor.TSSMonitorService{
					Client:        &monitorMocks.MockMonitorClient{},
					GitCommitHash: "0xABC",
					Version:       "0.01",
				},
				HorizonURL:           "https://horizon-testnet.stellar.org",
				NetworkPassphrase:    network.TestNetworkPassphrase,
				PrivateKeyEncrypter:  &utils.PrivateKeyEncrypterMock{},
				DistributionSeed:     "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
				NumChannelAccounts:   1,
				QueuePollingInterval: 10,
				MaxBaseFee:           txnbuild.MinBaseFee,
			},
		},
		{
			name: "🎉 successfully finishes validation with existing crash tracker client",
			submitterOptions: SubmitterOptions{
				DatabaseDSN: dbt.DSN,
				MonitorService: tssMonitor.TSSMonitorService{
					Client:        &monitorMocks.MockMonitorClient{},
					GitCommitHash: "0xABC",
					Version:       "0.01",
				},
				HorizonURL:           "https://horizon-testnet.stellar.org",
				NetworkPassphrase:    network.TestNetworkPassphrase,
				PrivateKeyEncrypter:  &utils.PrivateKeyEncrypterMock{},
				DistributionSeed:     "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
				NumChannelAccounts:   1,
				QueuePollingInterval: 10,
				MaxBaseFee:           txnbuild.MinBaseFee,
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
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	validSubmitterOptions := SubmitterOptions{
		DatabaseDSN: dbt.DSN,
		MonitorService: tssMonitor.TSSMonitorService{
			Client:        &monitorMocks.MockMonitorClient{},
			GitCommitHash: "0xABC",
			Version:       "0.01",
		},
		HorizonURL:           "https://horizon-testnet.stellar.org",
		NetworkPassphrase:    network.TestNetworkPassphrase,
		PrivateKeyEncrypter:  &utils.PrivateKeyEncrypterMock{},
		DistributionSeed:     "SBDBQFZIIZ53A7JC2X23LSQLI5RTKV5YWDRT33YXW5LRMPKRSJYXS2EW",
		NumChannelAccounts:   5,
		QueuePollingInterval: 10,
		MaxBaseFee:           txnbuild.MinBaseFee,
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
				opts.DatabaseDSN = "invalid-dsn"
				return opts
			},
			wantErrContains: "opening db connection pool: error pinging app DB connection pool: ",
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
				defer gotManager.dbConnectionPool.Close()

				// Assert the resulting manager state:
				wantConnectionPool := gotManager.dbConnectionPool
				wantTxModel := &store.TransactionModel{DBConnectionPool: wantConnectionPool}
				wantChAccModel := &store.ChannelAccountModel{DBConnectionPool: wantConnectionPool}
				wantChTxBundleModel, err := store.NewChannelTransactionBundleModel(wantConnectionPool)
				require.NoError(t, err)

				wantSubmitterEngine, err := engine.NewSubmitterEngine(&horizonclient.Client{
					HorizonURL: submitterOptions.HorizonURL,
					HTTP:       httpclient.DefaultClient(),
				})
				require.NoError(t, err)

				wantSigService, err := engine.NewDefaultSignatureService(
					submitterOptions.NetworkPassphrase,
					wantConnectionPool,
					submitterOptions.DistributionSeed, wantChAccModel,
					submitterOptions.PrivateKeyEncrypter,
					submitterOptions.DistributionSeed,
				)
				require.NoError(t, err)

				wantCrashTrackerClient := submitterOptions.CrashTrackerClient
				if tc.wantCrashTrackerClientFn != nil {
					wantCrashTrackerClient = tc.wantCrashTrackerClientFn()
				}

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

					engine:     wantSubmitterEngine,
					sigService: wantSigService,
					maxBaseFee: submitterOptions.MaxBaseFee,

					crashTrackerClient: wantCrashTrackerClient,
					monitorService:     submitterOptions.MonitorService,

					txProcessingLimiter: txProcessingLimiter,
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
			ctx, cancel := context.WithCancel(context.Background())

			defer store.DeleteAllFromChannelAccounts(t, context.Background(), dbConnectionPool)
			defer store.DeleteAllTransactionFixtures(t, context.Background(), dbConnectionPool)

			// Create channel accounts to be used by the tx submitter
			channelAccounts := store.CreateChannelAccountFixtures(t, ctx, dbConnectionPool, 2)
			assert.Len(t, channelAccounts, 2)
			channelAccountsMap := map[string]*store.ChannelAccount{}
			for _, ca := range channelAccounts {
				channelAccountsMap[ca.PublicKey] = ca
			}

			// Create transactions to be used by the tx submitter
			transactions := store.CreateTransactionFixtures(t, ctx, dbConnectionPool, 10, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", keypair.MustRandom().Address(), store.TransactionStatusPending, 1)
			assert.Len(t, transactions, 10)

			// Signature service
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

			// mock ledger number tracker
			const currentLedgerNumber = 123
			mockLedgerNumberTracker := &mocks.MockLedgerNumberTracker{}
			mockLedgerNumberTracker.On("GetLedgerNumber").Return(currentLedgerNumber, nil)
			defer mockLedgerNumberTracker.AssertExpectations(t)

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
			}

			dryRunCrashTracker, err := crashtracker.NewDryRunClient()
			require.NoError(t, err)

			queueService := defaultQueueService{
				pollingInterval:    500 * time.Millisecond,
				numChannelAccounts: 2,
			}

			chTxBundleModel, err := store.NewChannelTransactionBundleModel(dbConnectionPool)
			require.NoError(t, err)

			mMonitorClient := monitorMocks.MockMonitorClient{}
			mMonitorClient.On("MonitorCounters", mock.Anything, mock.Anything).Return(nil).Times(3)
			defer mMonitorClient.AssertExpectations(t)

			manager := &Manager{
				dbConnectionPool:    dbConnectionPool,
				chTxBundleModel:     chTxBundleModel,
				chAccModel:          store.NewChannelAccountModel(dbConnectionPool),
				txModel:             store.NewTransactionModel(dbConnectionPool),
				engine:              submitterEngine,
				crashTrackerClient:  dryRunCrashTracker,
				queueService:        queueService,
				sigService:          sigService,
				maxBaseFee:          txnbuild.MinBaseFee,
				txProcessingLimiter: engine.NewTransactionProcessingLimiter(queueService.numChannelAccounts),
				monitorService: tssMonitor.TSSMonitorService{
					Client:        &mMonitorClient,
					GitCommitHash: "gitCommitHash0x",
					Version:       "version123",
				},
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
			}

			cancel()
		})
	}
}
