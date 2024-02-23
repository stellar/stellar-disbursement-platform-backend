package cmd

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	serveadmin "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/serve"
)

type mockServer struct {
	wg sync.WaitGroup
	mock.Mock
}

// Making sure that mockServer implements ServerServiceInterface
var _ ServerServiceInterface = (*mockServer)(nil)

func (m *mockServer) StartServe(opts serve.ServeOptions, httpServer serve.HTTPServerInterface) {
	m.Called(opts, httpServer)
	m.wg.Wait()
}

func (m *mockServer) StartMetricsServe(opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface) {
	m.Called(opts, httpServer)
	m.wg.Done()
}

func (m *mockServer) StartAdminServe(opts serveadmin.ServeOptions, httpServer serveadmin.HTTPServerInterface) {
	m.Called(opts, httpServer)
	m.wg.Done()
}

func (m *mockServer) GetSchedulerJobRegistrars(ctx context.Context, serveOpts serve.ServeOptions, schedulerOptions scheduler.SchedulerOptions, apAPIService anchorplatform.AnchorPlatformAPIServiceInterface) ([]scheduler.SchedulerJobRegisterOption, error) {
	args := m.Called(ctx, serveOpts, schedulerOptions, apAPIService)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]scheduler.SchedulerJobRegisterOption), args.Error(1)
}

func (m *mockServer) SetupConsumers(ctx context.Context, eventBrokerOptions cmdUtils.EventBrokerOptions, eventHandlerOptions events.EventHandlerOptions, serveOpts serve.ServeOptions) (TearDownFunc, error) {
	args := m.Called(ctx, eventBrokerOptions, eventHandlerOptions, serveOpts)
	return args.Get(0).(TearDownFunc), args.Error(1)
}

func Test_serve_wasCalled(t *testing.T) {
	// setup
	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	serveCmdFound := false

	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "serve" {
			serveCmdFound = true
		}
	}
	require.True(t, serveCmdFound, "serve command not found")
	rootCmd.SetArgs([]string{"serve", "--help"})
	var out bytes.Buffer
	rootCmd.SetOut(&out)

	// test
	err := rootCmd.Execute()
	require.NoError(t, err)

	// assert
	assert.Contains(t, out.String(), "stellar-disbursement-platform serve [flags]", "should have printed help message for serve command")
}

func Test_serve(t *testing.T) {
	dbt := dbtest.Open(t)
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	dbConnectionPool.Close()
	dbt.Close()

	cmdUtils.ClearTestEnvironment(t)
	distributionSeed := "SBHQEYSACD5DOK5I656NKLAMOHC6VT64ATOWWM2VJ3URGDGMVGNPG4ON"

	// Populate dependency injection:
	di.SetInstance(di.TSSDBConnectionPoolInstanceName, dbConnectionPool)

	mHorizonClient := &horizonclient.MockClient{}
	di.SetInstance(di.HorizonClientInstanceName, mHorizonClient)

	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	di.SetInstance(di.LedgerNumberTrackerInstanceName, mLedgerNumberTracker)

	sigService, _, _, _, _ := signing.NewMockSignatureService(t)

	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}
	di.SetInstance(di.TxSubmitterEngineInstanceName, submitterEngine)

	ctx := context.Background()

	// mock metric service
	mMonitorService := monitor.MockMonitorService{}

	serveOpts := serve.ServeOptions{
		Environment:                     "test",
		GitCommit:                       "1234567890abcdef",
		Port:                            8000,
		Version:                         "x.y.z",
		InstanceName:                    "SDP Testnet",
		MonitorService:                  &mMonitorService,
		DatabaseDSN:                     dbt.DSN,
		EC256PublicKey:                  "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAER88h7AiQyVDysRTxKvBB6CaiO/kS\ncvGyimApUE/12gFhNTRf37SE19CSCllKxstnVFOpLLWB7Qu5OJ0Wvcz3hg==\n-----END PUBLIC KEY-----",
		EC256PrivateKey:                 "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx\nJn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy\n8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG\n-----END PRIVATE KEY-----",
		CorsAllowedOrigins:              []string{"*"},
		SEP24JWTSecret:                  "jwt_secret_1234567890",
		BaseURL:                         "https://sdp.com",
		ResetTokenExpirationHours:       24,
		NetworkPassphrase:               network.TestNetworkPassphrase,
		Sep10SigningPublicKey:           "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
		Sep10SigningPrivateKey:          "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
		AnchorPlatformBaseSepURL:        "localhost:8080",
		AnchorPlatformBasePlatformURL:   "localhost:8085",
		AnchorPlatformOutgoingJWTSecret: "jwt_secret_1234567890",
		DistributionPublicKey:           "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
		ReCAPTCHASiteKey:                "reCAPTCHASiteKey",
		ReCAPTCHASiteSecretKey:          "reCAPTCHASiteSecretKey",
		DisableMFA:                      false,
		DisableReCAPTCHA:                false,
		EnableScheduler:                 true,
		EnableMultiTenantDB:             false,
		SubmitterEngine:                 submitterEngine,
	}
	serveOpts.AnchorPlatformAPIService, err = anchorplatform.NewAnchorPlatformAPIService(httpclient.DefaultClient(), serveOpts.AnchorPlatformBasePlatformURL, serveOpts.AnchorPlatformOutgoingJWTSecret)
	require.NoError(t, err)

	serveOpts.CrashTrackerClient, err = di.NewCrashTracker(ctx, crashtracker.CrashTrackerOptions{
		Environment:      serveOpts.Environment,
		GitCommit:        serveOpts.GitCommit,
		CrashTrackerType: "DRY_RUN",
	})
	require.NoError(t, err)

	messengerClient, err := di.NewEmailClient(di.EmailClientOptions{EmailType: message.MessengerTypeDryRun})
	require.NoError(t, err)
	serveOpts.EmailMessengerClient = messengerClient

	serveOpts.SMSMessengerClient, err = di.NewSMSClient(di.SMSClientOptions{SMSType: message.MessengerTypeDryRun})
	require.NoError(t, err)

	kafkaConfig := events.KafkaConfig{
		Brokers:          []string{"kafka:9092"},
		SecurityProtocol: events.KafkaProtocolPlaintext,
	}
	serveOpts.EventProducer, err = events.NewKafkaProducer(kafkaConfig)
	require.NoError(t, err)

	metricOptions := monitor.MetricOptions{
		MetricType:  monitor.MetricTypePrometheus,
		Environment: "test",
	}
	mMonitorService.On("Start", metricOptions).Return(nil).Once()
	defer mMonitorService.AssertExpectations(t)

	chAccEncryptionPassphrase := keypair.MustRandom().Seed()

	serveMetricOpts := serve.MetricsServeOptions{
		Port:        8002,
		Environment: "test",

		MetricType:     monitor.MetricTypePrometheus,
		MonitorService: &mMonitorService,
	}

	serveTenantOpts := serveadmin.ServeOptions{
		Environment:          "test",
		EmailMessengerClient: messengerClient,
		DatabaseDSN:          dbt.DSN,
		GitCommit:            "1234567890abcdef",
		NetworkPassphrase:    network.TestNetworkPassphrase,
		Port:                 8003,
		Version:              "x.y.z",
	}

	eventBrokerOptions := cmdUtils.EventBrokerOptions{
		EventBrokerType: events.KafkaEventBrokerType,
		BrokerURLs:      []string{"kafka:9092"},
		ConsumerGroupID: "group-id",

		KafkaSecurityProtocol: events.KafkaProtocolPlaintext,
	}

	eventHandlerOptions := events.EventHandlerOptions{
		MaxInvitationSMSResendAttempts: 3,
	}

	// mock server
	mServer := mockServer{}
	mServer.On("StartMetricsServe", serveMetricOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.On("StartServe", serveOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.On("StartAdminServe", serveTenantOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.
		On("GetSchedulerJobRegistrars", mock.Anything, serveOpts, scheduler.SchedulerOptions{}, mock.Anything).
		Return([]scheduler.SchedulerJobRegisterOption{}, nil).
		Once()
	mServer.On("SetupConsumers", ctx, eventBrokerOptions, eventHandlerOptions, serveOpts).Return(TearDownFunc(func() { t.Log("tear down func called") }), nil).Once()
	mServer.wg.Add(2)
	defer mServer.AssertExpectations(t)

	// SetupCLI and replace the serve command with one containing a mocked server
	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	originalCommands := rootCmd.Commands()
	rootCmd.ResetCommands()
	serveCmdFound := false
	for _, cmd := range originalCommands {
		if cmd.Use == "serve" {
			serveCmdFound = true
			rootCmd.AddCommand((&ServeCommand{}).Command(&mServer, &mMonitorService))
		} else {
			rootCmd.AddCommand(cmd)
		}
	}
	require.True(t, serveCmdFound, "serve command not found")

	t.Setenv("DATABASE_URL", serveOpts.DatabaseDSN)
	t.Setenv("EC256_PUBLIC_KEY", serveOpts.EC256PublicKey)
	t.Setenv("EC256_PRIVATE_KEY", serveOpts.EC256PrivateKey)
	t.Setenv("SEP24_JWT_SECRET", serveOpts.SEP24JWTSecret)
	t.Setenv("SEP10_SIGNING_PUBLIC_KEY", serveOpts.Sep10SigningPublicKey)
	t.Setenv("SEP10_SIGNING_PRIVATE_KEY", serveOpts.Sep10SigningPrivateKey)
	t.Setenv("ANCHOR_PLATFORM_BASE_SEP_URL", serveOpts.AnchorPlatformBaseSepURL)
	t.Setenv("ANCHOR_PLATFORM_BASE_PLATFORM_URL", serveOpts.AnchorPlatformBasePlatformURL)
	t.Setenv("ANCHOR_PLATFORM_OUTGOING_JWT_SECRET", serveOpts.AnchorPlatformOutgoingJWTSecret)
	t.Setenv("DISTRIBUTION_PUBLIC_KEY", serveOpts.DistributionPublicKey)
	t.Setenv("DISABLE_MFA", fmt.Sprintf("%t", serveOpts.DisableMFA))
	t.Setenv("DISABLE_RECAPTCHA", fmt.Sprintf("%t", serveOpts.DisableMFA))
	t.Setenv("DISTRIBUTION_SEED", distributionSeed)
	t.Setenv("BASE_URL", serveOpts.BaseURL)
	t.Setenv("RECAPTCHA_SITE_KEY", serveOpts.ReCAPTCHASiteKey)
	t.Setenv("RECAPTCHA_SITE_SECRET_KEY", serveOpts.ReCAPTCHASiteSecretKey)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("INSTANCE_NAME", serveOpts.InstanceName)
	t.Setenv("ENABLE_SCHEDULER", "true")
	t.Setenv("ENABLE_MULTITENANT_DB", "false")
	t.Setenv("EVENT_BROKER", "kafka")
	t.Setenv("BROKER_URLS", "kafka:9092")
	t.Setenv("CONSUMER_GROUP_ID", "group-id")
	t.Setenv("CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE", chAccEncryptionPassphrase)
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("METRICS_TYPE", "PROMETHEUS")
	t.Setenv("KAFKA_SECURITY_PROTOCOL", string(events.KafkaProtocolPlaintext))

	// test & assert
	rootCmd.SetArgs([]string{"serve"})
	err = rootCmd.Execute()
	require.NoError(t, err)
}
