package cmd

import (
	"context"
	"fmt"
	"go/types"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler/jobs"

	"github.com/spf13/cobra"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
)

type ServeCommand struct{}

type ServerServiceInterface interface {
	StartServe(opts serve.ServeOptions, httpServer serve.HTTPServerInterface)
	StartMetricsServe(opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface)
	GetSchedulerJobRegistrars(ctx context.Context, serveOpts serve.ServeOptions, schedulerOptions scheduler.SchedulerOptions, apAPIService anchorplatform.AnchorPlatformAPIServiceInterface) ([]scheduler.SchedulerJobRegisterOption, error)
}

type ServerService struct{}

// Making sure that ServerService implements ServerServiceInterface
var _ ServerServiceInterface = (*ServerService)(nil)

func (s *ServerService) StartServe(opts serve.ServeOptions, httpServer serve.HTTPServerInterface) {
	err := serve.Serve(opts, httpServer)
	if err != nil {
		log.Fatalf("Error starting server: %s", err.Error())
	}
}

func (s *ServerService) StartMetricsServe(opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface) {
	err := serve.MetricsServe(opts, httpServer)
	if err != nil {
		log.Fatalf("Error starting metrics server: %s", err.Error())
	}
}

func (s *ServerService) GetSchedulerJobRegistrars(ctx context.Context, serveOpts serve.ServeOptions, schedulerOptions scheduler.SchedulerOptions, apAPIService anchorplatform.AnchorPlatformAPIServiceInterface) ([]scheduler.SchedulerJobRegisterOption, error) {
	// TODO: inject these in the server options, to do the Dependency Injection properly.
	dbConnectionPool, err := db.OpenDBConnectionPool(globalOptions.databaseURL)
	if err != nil {
		log.Ctx(ctx).Fatalf("error getting DB connection in Job Scheduler: %s", err.Error())
	}
	models, err := data.NewModels(dbConnectionPool)
	if err != nil {
		log.Ctx(ctx).Fatalf("error creating models in Job Scheduler: %s", err.Error())
	}

	return []scheduler.SchedulerJobRegisterOption{
		scheduler.WithPaymentToSubmitterJobOption(models),
		scheduler.WithPaymentFromSubmitterJobOption(models),
		scheduler.WithSendReceiverWalletsSMSInvitationJobOption(jobs.SendReceiverWalletsSMSInvitationJobOptions{
			AnchorPlatformBaseSepURL: serveOpts.AnchorPlatformBaseSepURL,
			Models:                   models,
			MessengerClient:          serveOpts.SMSMessengerClient,
			MinDaysBetweenRetries:    schedulerOptions.MinDaysBetweenRetries,
			MaxRetries:               schedulerOptions.MaxRetries,
			Sep10SigningPrivateKey:   serveOpts.Sep10SigningPrivateKey,
			CrashTrackerClient:       serveOpts.CrashTrackerClient.Clone(),
		}),
		scheduler.WithAPAuthEnforcementJob(apAPIService, serveOpts.MonitorService, serveOpts.CrashTrackerClient.Clone()),
	}, nil
}

func (c *ServeCommand) Command(serverService ServerServiceInterface, monitorService monitor.MonitorServiceInterface) *cobra.Command {
	serveOpts := serve.ServeOptions{}
	metricsServeOpts := serve.MetricsServeOptions{}
	schedulerOptions := scheduler.SchedulerOptions{}
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}

	configOpts := config.ConfigOptions{
		{
			Name:        "port",
			Usage:       "Port where the server will be listening on",
			OptType:     types.Int,
			ConfigKey:   &serveOpts.Port,
			FlagDefault: 8000,
			Required:    true,
		},
		{
			Name:           "metrics-type",
			Usage:          `Metric monitor type. Options: "PROMETHEUS"`,
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMetricType,
			ConfigKey:      &metricsServeOpts.MetricType,
			FlagDefault:    "PROMETHEUS",
			Required:       true,
		},
		{
			Name:        "metrics-port",
			Usage:       "Port where the metrics server will be listening on",
			OptType:     types.Int,
			ConfigKey:   &metricsServeOpts.Port,
			FlagDefault: 8002,
			Required:    true,
		},
		{
			Name:           "crash-tracker-type",
			Usage:          `Crash tracker type. Options: "SENTRY", "DRY_RUN"`,
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionCrashTrackerType,
			ConfigKey:      &crashTrackerOptions.CrashTrackerType,
			FlagDefault:    "DRY_RUN",
			Required:       true,
		},
		{
			Name:           "ec256-public-key",
			Usage:          "The EC256 Public Key. This key is used to validate the token signature",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionEC256PublicKey,
			ConfigKey:      &serveOpts.EC256PublicKey,
			Required:       true,
		},
		{
			Name:           "ec256-private-key",
			Usage:          "The EC256 Private Key. This key is used to sign the authentication token",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionEC256PrivateKey,
			ConfigKey:      &serveOpts.EC256PrivateKey,
			Required:       true,
		},
		{
			Name:           "cors-allowed-origins",
			Usage:          `Cors URLs that are allowed to access the endpoints, separated by ","`,
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetCorsAllowedOrigins,
			ConfigKey:      &serveOpts.CorsAllowedOrigins,
			Required:       true,
		},
		{
			Name:      "sep24-jwt-secret",
			Usage:     `The JWT secret that's used by the Anchor Platform to sign the SEP-24 JWT token`,
			OptType:   types.String,
			ConfigKey: &serveOpts.SEP24JWTSecret,
			Required:  true,
		},
		{
			Name:           "sep10-signing-public-key",
			Usage:          "The public key of the Stellar account that signs the SEP-10 transactions. It's also used to sign URLs.",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPublicKey,
			ConfigKey:      &serveOpts.Sep10SigningPublicKey,
			Required:       true,
		},
		{
			Name:           "sep10-signing-private-key",
			Usage:          "The private key of the Stellar account that signs the SEP-10 transactions. It's also used to sign URLs.",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &serveOpts.Sep10SigningPrivateKey,
			Required:       true,
		},
		{
			Name: "anchor-platform-base-platform-url",
			Usage: "The Base URL of the platform server of the anchor platform. This is the base URL where the Anchor Platform " +
				"exposes its private API that is meant to be reached only by the SDP server, such as the PATCH /sep24/transactions endpoint.",
			OptType:   types.String,
			ConfigKey: &serveOpts.AnchorPlatformBasePlatformURL,
			Required:  true,
		},
		{
			Name: "anchor-platform-base-sep-url",
			Usage: "The Base URL of the sep server of the anchor platform. This is the base URL where the Anchor Platform " +
				"exposes its public API that is meant to be reached by a client application, such as the stellar.toml file.",
			OptType:   types.String,
			ConfigKey: &serveOpts.AnchorPlatformBaseSepURL,
			Required:  true,
		},
		{
			Name:      "anchor-platform-outgoing-jwt-secret",
			Usage:     "The JWT secret used to create a JWT token used to send requests to the anchor platform.",
			OptType:   types.String,
			ConfigKey: &serveOpts.AnchorPlatformOutgoingJWTSecret,
			Required:  true,
		},
		{
			Name:        "reset-token-expiration-hours",
			Usage:       "The expiration time in hours of the Reset Token",
			OptType:     types.Int,
			ConfigKey:   &serveOpts.ResetTokenExpirationHours,
			FlagDefault: 24,
			Required:    true,
		},
		{
			Name:        "min-days-between-retries",
			Usage:       "The minimum amount of days that the invitation SMS was sent to the Receiver Wallets before we send the invitation again.",
			OptType:     types.Int,
			ConfigKey:   &schedulerOptions.MinDaysBetweenRetries,
			FlagDefault: 7,
			Required:    true,
		},
		{
			Name:        "max-retries",
			Usage:       "The maximum amount of tries to send the SMS invitation to the Receiver Wallets.",
			OptType:     types.Int,
			ConfigKey:   &schedulerOptions.MaxRetries,
			FlagDefault: 3,
			Required:    true,
		},
		{
			Name:           "distribution-public-key",
			Usage:          "The public key of the Stellar distribution account that sends the Stellar payments.",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPublicKey,
			ConfigKey:      &serveOpts.DistributionPublicKey,
			Required:       true,
		},
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account used to disburse funds",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &serveOpts.DistributionSeed,
			Required:       true,
		},
		{
			Name:      "recaptcha-site-key",
			Usage:     "The Google 'reCAPTCHA v2 - I'm not a robot' site key.",
			OptType:   types.String,
			ConfigKey: &serveOpts.ReCAPTCHASiteKey,
			Required:  true,
		},
		{
			Name:      "recaptcha-site-secret-key",
			Usage:     "The Google 'reCAPTCHA v2 - I'm not a robot' site SECRET key.",
			OptType:   types.String,
			ConfigKey: &serveOpts.ReCAPTCHASiteSecretKey,
			Required:  true,
		},
		{
			Name:           "sdp-ui-base-url",
			Usage:          "The SDP UI/dashboard Base URL.",
			OptType:        types.String,
			ConfigKey:      &serveOpts.UIBaseURL,
			FlagDefault:    "http://localhost:3000",
			CustomSetValue: cmdUtils.SetConfigOptionURLString,
			Required:       true,
		},
		{
			Name:        "enable-mfa",
			Usage:       "Enable MFA using email.",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.EnableMFA,
			FlagDefault: true,
			Required:    false,
		},
		{
			Name:        "enable-recaptcha",
			Usage:       "Enable ReCAPTCHA for login and forgot password.",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.EnableReCAPTCHA,
			FlagDefault: true,
			Required:    false,
		},
		{
			Name:        "horizon-url",
			Usage:       "Stellar Horizon URL.",
			OptType:     types.String,
			ConfigKey:   &serveOpts.HorizonURL,
			FlagDefault: horizonclient.DefaultTestNetClient.HorizonURL,
			Required:    true,
		},
	}

	messengerOptions := message.MessengerOptions{}

	// messenger config options:
	configOpts = append(configOpts, cmdUtils.TwilioConfigOptions(&messengerOptions)...)
	configOpts = append(configOpts, cmdUtils.AWSConfigOptions(&messengerOptions)...)

	// sms
	smsOpts := di.SMSClientOptions{MessengerOptions: &messengerOptions}
	configOpts = append(configOpts,
		&config.ConfigOption{
			// message sender type
			Name:           "sms-sender-type",
			Usage:          fmt.Sprintf("SMS Sender Type. Options: %+v", message.MessengerType("").ValidSMSTypes()),
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMessengerType,
			ConfigKey:      &smsOpts.SMSType,
			FlagDefault:    string(message.MessengerTypeDryRun),
			Required:       true,
		})

	// email
	emailOpts := di.EmailClientOptions{MessengerOptions: &messengerOptions}
	configOpts = append(configOpts,
		&config.ConfigOption{
			// message sender type
			Name:           "email-sender-type",
			Usage:          fmt.Sprintf("Email Sender Type. Options: %+v", message.MessengerType("").ValidEmailTypes()),
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMessengerType,
			ConfigKey:      &emailOpts.EmailType,
			FlagDefault:    string(message.MessengerTypeDryRun),
			Required:       true,
		})

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Stellar Disbursement Platform API",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}

			// Initializing monitor service
			metricOptions := monitor.MetricOptions{
				MetricType:  metricsServeOpts.MetricType,
				Environment: globalOptions.environment,
			}

			err = monitorService.Start(metricOptions)
			if err != nil {
				log.Fatalf("Error creating monitor service: %s", err.Error())
			}

			// Inject crash tracker options dependencies
			globalOptions.populateCrashTrackerOptions(&crashTrackerOptions)

			// Inject server dependencies
			serveOpts.Environment = globalOptions.environment
			serveOpts.GitCommit = globalOptions.gitCommit
			serveOpts.DatabaseDSN = globalOptions.databaseURL
			serveOpts.Version = globalOptions.version
			serveOpts.MonitorService = monitorService
			serveOpts.BaseURL = globalOptions.baseURL
			serveOpts.NetworkPassphrase = globalOptions.networkPassphrase

			// Inject metrics server dependencies
			metricsServeOpts.MonitorService = monitorService
			metricsServeOpts.Environment = globalOptions.environment
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			// Setup default Crash Tracker client
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating crash tracker client: %s", err.Error())
			}
			serveOpts.CrashTrackerClient = crashTrackerClient

			// Setup default Email client
			emailMessengerClient, err := di.NewEmailClient(emailOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating email client: %s", err.Error())
			}
			serveOpts.EmailMessengerClient = emailMessengerClient

			// Setup default SMS client
			smsMessengerClient, err := di.NewSMSClient(smsOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating SMS client: %s", err.Error())
			}
			serveOpts.SMSMessengerClient = smsMessengerClient

			// Setup default AP Auth enforcer
			apAPIService, err := di.NewAnchorPlatformAPIService(serveOpts.AnchorPlatformBasePlatformURL, serveOpts.AnchorPlatformOutgoingJWTSecret)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating Anchor Platform API Service: %v", err)
			}
			serveOpts.AnchorPlatformAPIService = apAPIService

			// Starting Scheduler Service (background job)
			log.Ctx(ctx).Info("Starting Scheduler Service...")
			schedulerJobRegistrars, err := serverService.GetSchedulerJobRegistrars(ctx, serveOpts, schedulerOptions, apAPIService)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting scheduler job registrars: %v", err)
			}
			go scheduler.StartScheduler(crashTrackerClient.Clone(), schedulerJobRegistrars...)

			// Starting Metrics Server (background job)
			log.Ctx(ctx).Info("Starting Metrics Server...")
			go serverService.StartMetricsServe(metricsServeOpts, &serve.HTTPServer{})

			// Starting Application Server
			log.Ctx(ctx).Info("Starting Application Server...")
			serverService.StartServe(serveOpts, &serve.HTTPServer{})
		},
	}
	err := configOpts.Init(cmd)
	if err != nil {
		log.Fatalf("Error initializing a config option: %s", err.Error())
	}

	return cmd
}
