package cmd

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/network"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func (m *mockServer) GetSchedulerJobRegistrars(ctx context.Context, serveOpts serve.ServeOptions, schedulerOptions scheduler.SchedulerOptions, apAPIService anchorplatform.AnchorPlatformAPIServiceInterface) ([]scheduler.SchedulerJobRegisterOption, error) {
	args := m.Called(ctx, serveOpts, schedulerOptions, apAPIService)
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
	randomDatabaseDSN := dbt.DSN
	dbt.Close()

	cmdUtils.ClearTestEnvironment(t)

	ctx := context.Background()

	// mock metric service
	mMonitorService := monitor.MockMonitorService{}

	serveOpts := serve.ServeOptions{
		Environment:                     "test",
		GitCommit:                       "1234567890abcdef",
		Port:                            8000,
		Version:                         "x.y.z",
		MonitorService:                  &mMonitorService,
		DatabaseDSN:                     randomDatabaseDSN,
		EC256PublicKey:                  "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAER88h7AiQyVDysRTxKvBB6CaiO/kS\ncvGyimApUE/12gFhNTRf37SE19CSCllKxstnVFOpLLWB7Qu5OJ0Wvcz3hg==\n-----END PUBLIC KEY-----",
		EC256PrivateKey:                 "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgIqI1MzMZIw2pQDLx\nJn0+FcNT/hNjwtn2TW43710JKZqhRANCAARHzyHsCJDJUPKxFPEq8EHoJqI7+RJy\n8bKKYClQT/XaAWE1NF/ftITX0JIKWUrGy2dUU6kstYHtC7k4nRa9zPeG\n-----END PRIVATE KEY-----",
		CorsAllowedOrigins:              []string{"*"},
		SEP24JWTSecret:                  "jwt_secret_1234567890",
		BaseURL:                         "https://sdp.com",
		UIBaseURL:                       "http://localhost:3000",
		ResetTokenExpirationHours:       24,
		NetworkPassphrase:               network.TestNetworkPassphrase,
		HorizonURL:                      horizonclient.DefaultTestNetClient.HorizonURL,
		Sep10SigningPublicKey:           "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
		Sep10SigningPrivateKey:          "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
		AnchorPlatformBaseSepURL:        "localhost:8080",
		AnchorPlatformBasePlatformURL:   "localhost:8085",
		AnchorPlatformOutgoingJWTSecret: "jwt_secret_1234567890",
		DistributionPublicKey:           "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
		DistributionSeed:                "SBHQEYSACD5DOK5I656NKLAMOHC6VT64ATOWWM2VJ3URGDGMVGNPG4ON",
		ReCAPTCHASiteKey:                "reCAPTCHASiteKey",
		ReCAPTCHASiteSecretKey:          "reCAPTCHASiteSecretKey",
		DisableMFA:                      false,
		DisableReCAPTCHA:                false,
	}
	var err error
	serveOpts.AnchorPlatformAPIService, err = anchorplatform.NewAnchorPlatformAPIService(httpclient.DefaultClient(), serveOpts.AnchorPlatformBasePlatformURL, serveOpts.AnchorPlatformOutgoingJWTSecret)
	require.NoError(t, err)

	crashTrackerClient, err := di.NewCrashTracker(ctx, crashtracker.CrashTrackerOptions{
		Environment:      serveOpts.Environment,
		GitCommit:        serveOpts.GitCommit,
		CrashTrackerType: "DRY_RUN",
	})
	require.NoError(t, err)
	serveOpts.CrashTrackerClient = crashTrackerClient

	messengerClient, err := di.NewEmailClient(di.EmailClientOptions{EmailType: message.MessengerTypeDryRun})
	require.NoError(t, err)
	serveOpts.EmailMessengerClient = messengerClient

	smsMessengerClient, err := di.NewSMSClient(di.SMSClientOptions{SMSType: message.MessengerTypeDryRun})
	require.NoError(t, err)
	serveOpts.SMSMessengerClient = smsMessengerClient

	metricOptions := monitor.MetricOptions{
		MetricType:  monitor.MetricTypePrometheus,
		Environment: "test",
	}
	mMonitorService.On("Start", metricOptions).Return(nil).Once()

	serveMetricOpts := serve.MetricsServeOptions{
		Port:        8002,
		Environment: "test",

		MetricType:     monitor.MetricTypePrometheus,
		MonitorService: &mMonitorService,
	}

	schedulerOptions := scheduler.SchedulerOptions{
		MaxInvitationSMSResendAttempts: 3,
	}

	// mock server
	mServer := mockServer{}
	mServer.On("StartMetricsServe", serveMetricOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.On("StartServe", serveOpts, mock.AnythingOfType("*serve.HTTPServer")).Once()
	mServer.
		On("GetSchedulerJobRegistrars", mock.AnythingOfType("*context.emptyCtx"), serveOpts, schedulerOptions, mock.Anything).
		Return([]scheduler.SchedulerJobRegisterOption{}, nil).
		Once()
	mServer.wg.Add(1)

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
	t.Setenv("DISTRIBUTION_SEED", serveOpts.DistributionSeed)
	t.Setenv("DISABLE_MFA", fmt.Sprintf("%t", serveOpts.DisableMFA))
	t.Setenv("DISABLE_RECAPTCHA", fmt.Sprintf("%t", serveOpts.DisableMFA))
	t.Setenv("BASE_URL", serveOpts.BaseURL)
	t.Setenv("RECAPTCHA_SITE_KEY", serveOpts.ReCAPTCHASiteKey)
	t.Setenv("RECAPTCHA_SITE_SECRET_KEY", serveOpts.ReCAPTCHASiteSecretKey)
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	// test & assert
	rootCmd.SetArgs([]string{"--environment", "test", "serve", "--metrics-type", "PROMETHEUS"})
	err = rootCmd.Execute()
	require.NoError(t, err)
	mServer.AssertExpectations(t)
	mMonitorService.AssertExpectations(t)
}
