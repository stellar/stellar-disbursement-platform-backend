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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	monitorMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/monitor/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	svcMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	serveadmin "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
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

func (m *mockServer) GetSchedulerJobRegistrars(ctx context.Context,
	serveOpts serve.ServeOptions,
	schedulerOptions scheduler.SchedulerOptions,
	apAPIService anchorplatform.AnchorPlatformAPIServiceInterface,
	tssDBConnectinPool db.DBConnectionPool,
) ([]scheduler.SchedulerJobRegisterOption, error) {
	args := m.Called(ctx, serveOpts, schedulerOptions, apAPIService, tssDBConnectinPool)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]scheduler.SchedulerJobRegisterOption), args.Error(1)
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
	distributionAccKP := keypair.MustRandom()
	distributionAccPrivKey := distributionAccKP.Seed()

	// Populate dependency injection:
	di.SetInstance(di.TSSDBConnectionPoolInstanceName, dbConnectionPool)
	di.SetInstance(di.AdminDBConnectionPoolInstanceName, dbConnectionPool)
	di.SetInstance(di.MtnDBConnectionPoolInstanceName, dbConnectionPool)

	mHorizonClient := &horizonclient.MockClient{}
	di.SetInstance(di.HorizonClientInstanceName, mHorizonClient)

	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	di.SetInstance(di.LedgerNumberTrackerInstanceName, mLedgerNumberTracker)

	sigService, _, _ := signing.NewMockSignatureService(t)

	submitterEngine := engine.SubmitterEngine{
		HorizonClient:       mHorizonClient,
		SignatureService:    sigService,
		LedgerNumberTracker: mLedgerNumberTracker,
		MaxBaseFee:          100 * txnbuild.MinBaseFee,
	}
	di.SetInstance(di.TxSubmitterEngineInstanceName, submitterEngine)

	mDistAccService := svcMocks.NewMockDistributionAccountService(t)
	di.SetInstance(di.DistributionAccountServiceInstanceName, mDistAccService)

	ctx := context.Background()

	// mock metric service
	mMonitorService := monitorMocks.NewMockMonitorService(t)

	// mock circle service
	mCircleService := circle.NewMockService(t)
	di.SetInstance(di.CircleServiceInstanceName, mCircleService)

	serveOpts := serve.ServeOptions{
		Environment:                     "test",
		GitCommit:                       "1234567890abcdef",
		Port:                            8000,
		Version:                         "x.y.z",
		InstanceName:                    "SDP Testnet",
		MonitorService:                  mMonitorService,
		AdminDBConnectionPool:           dbConnectionPool,
		MtnDBConnectionPool:             dbConnectionPool,
		TSSDBConnectionPool:             dbConnectionPool,
		EC256PrivateKey:                 "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx\nJn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy\n8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG\n-----END PRIVATE KEY-----",
		CorsAllowedOrigins:              []string{"*"},
		SEP24JWTSecret:                  "jwt_secret_ducrCcqnKmIqG6mYG48Hqlf9TWb7CJh4",
		BaseURL:                         "https://sdp-backend.stellar.org",
		ResetTokenExpirationHours:       24,
		NetworkPassphrase:               network.TestNetworkPassphrase,
		NetworkType:                     utils.TestnetNetworkType,
		Sep10SigningPublicKey:           "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
		Sep10SigningPrivateKey:          "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
		Sep45ContractID:                 "CD3LA6RKF5D2FN2R2L57MWXLBRSEWWENE74YBEFZSSGNJRJGICFGQXMX",
		RPCConfig:                       stellar.RPCOptions{RPCUrl: "https://soroban-testnet.stellar.org"},
		AnchorPlatformBaseSepURL:        "localhost:8080",
		AnchorPlatformBasePlatformURL:   "localhost:8085",
		AnchorPlatformOutgoingJWTSecret: "jwt_secret_1234567890",
		ReCAPTCHASiteKey:                "reCAPTCHASiteKey",
		ReCAPTCHASiteSecretKey:          "reCAPTCHASiteSecretKey",
		CAPTCHAType:                     validators.GoogleReCAPTCHAV2,
		ReCAPTCHAV3MinScore:             0.5,
		DisableMFA:                      false,
		DisableReCAPTCHA:                false,
		SubmitterEngine:                 submitterEngine,
		DistributionAccountService:      mDistAccService,
		MaxInvitationResendAttempts:     3,
		DistAccEncryptionPassphrase:     distributionAccPrivKey,
		CircleService:                   mCircleService,
		CircleAPIType:                   circle.APITypeTransfers,
		WebAuthnSessionCacheMaxEntries:  1024,
		WebAuthnSessionTTLSeconds:       300,
	}
	serveOpts.AnchorPlatformAPIService, err = anchorplatform.NewAnchorPlatformAPIService(httpclient.DefaultClient(), serveOpts.AnchorPlatformBasePlatformURL, serveOpts.AnchorPlatformOutgoingJWTSecret)
	require.NoError(t, err)

	crashTrackerClient, err := di.NewCrashTracker(ctx, crashtracker.CrashTrackerOptions{
		Environment:      serveOpts.Environment,
		GitCommit:        serveOpts.GitCommit,
		CrashTrackerType: "DRY_RUN",
	})
	require.NoError(t, err)
	serveOpts.CrashTrackerClient = crashTrackerClient

	emailOpts := di.EmailClientOptions{EmailType: message.MessengerTypeDryRun}
	emailClient, err := di.NewEmailClient(emailOpts)
	require.NoError(t, err)
	serveOpts.EmailMessengerClient = emailClient

	smsOpts := di.SMSClientOptions{SMSType: message.MessengerTypeDryRun}

	messageDispatcherOpts := di.MessageDispatcherOpts{
		EmailOpts: &emailOpts,
		SMSOpts:   &smsOpts,
	}
	serveOpts.MessageDispatcher, err = di.NewMessageDispatcher(ctx, messageDispatcherOpts)
	require.NoError(t, err)

	metricOptions := monitor.MetricOptions{
		MetricType:  monitor.MetricTypePrometheus,
		Environment: "test",
	}
	mMonitorService.On("Start", metricOptions).Return(nil).Once()

	chAccEncryptionPassphrase := keypair.MustRandom().Seed()
	serveMetricOpts := serve.MetricsServeOptions{
		Port:           8002,
		Environment:    "test",
		MetricType:     monitor.MetricTypePrometheus,
		MonitorService: mMonitorService,
	}

	serveTenantOpts := serveadmin.ServeOptions{
		Environment:                             "test",
		EmailMessengerClient:                    emailClient,
		AdminDBConnectionPool:                   dbConnectionPool,
		MTNDBConnectionPool:                     dbConnectionPool,
		CrashTrackerClient:                      crashTrackerClient,
		GitCommit:                               "1234567890abcdef",
		NetworkPassphrase:                       network.TestNetworkPassphrase,
		Port:                                    8003,
		Version:                                 "x.y.z",
		SubmitterEngine:                         submitterEngine,
		DistributionAccountService:              mDistAccService,
		TenantAccountNativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
		AdminAccount:                            "admin-account",
		AdminAPIKey:                             "admin-api-key",
		BaseURL:                                 "https://sdp-backend.stellar.org",
		SDPUIBaseURL:                            "https://sdp-ui.stellar.org",
	}

	schedulerOpts := scheduler.SchedulerOptions{}
	schedulerOpts.ReceiverInvitationJobIntervalSeconds = 600
	schedulerOpts.PaymentJobIntervalSeconds = 600

	// mock server
	mServer := mockServer{}
	mServer.On("StartMetricsServe", serveMetricOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.On("StartServe", serveOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.On("StartAdminServe", serveTenantOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.
		On("GetSchedulerJobRegistrars", mock.Anything, serveOpts, schedulerOpts, serveOpts.AnchorPlatformAPIService, mock.Anything).
		Return([]scheduler.SchedulerJobRegisterOption{}, nil).
		Once()
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
			rootCmd.AddCommand((&ServeCommand{}).Command(&mServer, mMonitorService))
		} else {
			rootCmd.AddCommand(cmd)
		}
	}
	require.True(t, serveCmdFound, "serve command not found")

	t.Setenv("DATABASE_URL", dbt.DSN)
	t.Setenv("EC256_PRIVATE_KEY", serveOpts.EC256PrivateKey)
	t.Setenv("SEP24_JWT_SECRET", serveOpts.SEP24JWTSecret)
	t.Setenv("SEP10_SIGNING_PUBLIC_KEY", serveOpts.Sep10SigningPublicKey)
	t.Setenv("SEP10_SIGNING_PRIVATE_KEY", serveOpts.Sep10SigningPrivateKey)
	t.Setenv("SEP45_CONTRACT_ID", serveOpts.Sep45ContractID)
	t.Setenv("RPC_URL", serveOpts.RPCConfig.RPCUrl)
	t.Setenv("ANCHOR_PLATFORM_BASE_SEP_URL", serveOpts.AnchorPlatformBaseSepURL)
	t.Setenv("ANCHOR_PLATFORM_BASE_PLATFORM_URL", serveOpts.AnchorPlatformBasePlatformURL)
	t.Setenv("ANCHOR_PLATFORM_OUTGOING_JWT_SECRET", serveOpts.AnchorPlatformOutgoingJWTSecret)
	t.Setenv("DISTRIBUTION_PUBLIC_KEY", "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA")
	t.Setenv("DISABLE_MFA", fmt.Sprintf("%t", serveOpts.DisableMFA))
	t.Setenv("DISABLE_RECAPTCHA", fmt.Sprintf("%t", serveOpts.DisableMFA))
	t.Setenv("DISTRIBUTION_SEED", distributionAccPrivKey)
	t.Setenv("DISTRIBUTION_ACCOUNT_ENCRYPTION_PASSPHRASE", distributionAccPrivKey)
	t.Setenv("BASE_URL", serveOpts.BaseURL)
	t.Setenv("SDP_UI_BASE_URL", serveTenantOpts.SDPUIBaseURL)
	t.Setenv("WEBAUTHN_SESSION_TTL_SECONDS", fmt.Sprintf("%d", serveOpts.WebAuthnSessionTTLSeconds))
	t.Setenv("WEBAUTHN_SESSION_CACHE_MAX_ENTRIES", fmt.Sprintf("%d", serveOpts.WebAuthnSessionCacheMaxEntries))
	t.Setenv("RECAPTCHA_SITE_KEY", serveOpts.ReCAPTCHASiteKey)
	t.Setenv("RECAPTCHA_SITE_SECRET_KEY", serveOpts.ReCAPTCHASiteSecretKey)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("INSTANCE_NAME", serveOpts.InstanceName)
	t.Setenv("CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE", chAccEncryptionPassphrase)
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("METRICS_TYPE", "PROMETHEUS")
	t.Setenv("ADMIN_ACCOUNT", "admin-account")
	t.Setenv("ADMIN_API_KEY", "admin-api-key")
	t.Setenv("SCHEDULER_RECEIVER_INVITATION_JOB_SECONDS", "600")
	t.Setenv("SCHEDULER_PAYMENT_JOB_SECONDS", "600")

	// test & assert
	rootCmd.SetArgs([]string{"serve"})
	err = rootCmd.Execute()
	require.NoError(t, err)
}
