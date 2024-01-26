package cmd

import (
	"context"
	"fmt"
	"go/types"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/eventhandlers"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	serveadmin "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/serve"
)

type TearDownFunc func()

type ServeCommand struct{}

type ServerServiceInterface interface {
	StartServe(opts serve.ServeOptions, httpServer serve.HTTPServerInterface)
	StartMetricsServe(opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface)
	StartAdminServe(opts serveadmin.ServeOptions, httpServer serveadmin.HTTPServerInterface)
	GetSchedulerJobRegistrars(ctx context.Context, serveOpts serve.ServeOptions, schedulerOptions scheduler.SchedulerOptions, apAPIService anchorplatform.AnchorPlatformAPIServiceInterface) ([]scheduler.SchedulerJobRegisterOption, error)
	SetupConsumers(ctx context.Context, eventBrokerOptions cmdUtils.EventBrokerOptions, eventHandlerOptions events.EventHandlerOptions, serveOpts serve.ServeOptions) (_ TearDownFunc, err error)
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

func (s *ServerService) StartAdminServe(opts serveadmin.ServeOptions, httpServer serveadmin.HTTPServerInterface) {
	err := serveadmin.StartServe(opts, httpServer)
	if err != nil {
		log.Fatalf("Error starting metrics server: %s", err.Error())
	}
}

func (s *ServerService) GetSchedulerJobRegistrars(ctx context.Context, serveOpts serve.ServeOptions, schedulerOptions scheduler.SchedulerOptions, apAPIService anchorplatform.AnchorPlatformAPIServiceInterface) ([]scheduler.SchedulerJobRegisterOption, error) {
	// TODO: inject these in the server options, to do the Dependency Injection properly.
	dbConnectionPool, err := db.OpenDBConnectionPool(globalOptions.DatabaseURL)
	if err != nil {
		log.Ctx(ctx).Fatalf("error getting DB connection in Job Scheduler: %s", err.Error())
	}
	models, err := data.NewModels(dbConnectionPool)
	if err != nil {
		log.Ctx(ctx).Fatalf("error creating models in Job Scheduler: %s", err.Error())
	}

	return []scheduler.SchedulerJobRegisterOption{
		scheduler.WithPaymentToSubmitterJobOption(models),
		scheduler.WithAPAuthEnforcementJob(apAPIService, serveOpts.MonitorService, serveOpts.CrashTrackerClient.Clone()),
		scheduler.WithReadyPaymentsCancellationJobOption(models),
	}, nil
}

func (s *ServerService) SetupConsumers(ctx context.Context, eventBrokerOptions cmdUtils.EventBrokerOptions, eventHandlerOptions events.EventHandlerOptions, serveOpts serve.ServeOptions) (_ TearDownFunc, err error) {
	dbConnectionPool, err := db.OpenDBConnectionPool(globalOptions.DatabaseURL)
	if err != nil {
		log.Ctx(ctx).Fatalf("error getting DB connection in Setup Consumers: %s", err.Error())
	}
	defer func() {
		if err != nil && dbConnectionPool != nil {
			dbConnectionPool.Close()
		}
	}()

	tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, di.TSSDBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL})
	if err != nil {
		return nil, fmt.Errorf("getting TSS DB connection pool: %w", err)
	}
	defer func() {
		if err != nil && tssDBConnectionPool != nil {
			tssDBConnectionPool.Close()
		}
	}()

	smsInvitationConsumer, err := events.NewKafkaConsumer(
		cmdUtils.KafkaConfig(eventBrokerOptions),
		events.ReceiverWalletNewInvitationTopic,
		eventBrokerOptions.ConsumerGroupID,
		eventhandlers.NewSendReceiverWalletsSMSInvitationEventHandler(eventhandlers.SendReceiverWalletsSMSInvitationEventHandlerOptions{
			DBConnectionPool:               dbConnectionPool,
			AnchorPlatformBaseSepURL:       serveOpts.AnchorPlatformBasePlatformURL,
			MessengerClient:                serveOpts.SMSMessengerClient,
			MaxInvitationSMSResendAttempts: int64(eventHandlerOptions.MaxInvitationSMSResendAttempts),
			Sep10SigningPrivateKey:         serveOpts.Sep10SigningPrivateKey,
			CrashTrackerClient:             serveOpts.CrashTrackerClient.Clone(),
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("creating SMS Invitation Kafka Consumer: %w", err)
	}

	paymentCompletedConsumer, err := events.NewKafkaConsumer(
		cmdUtils.KafkaConfig(eventBrokerOptions),
		events.PaymentCompletedTopic,
		eventBrokerOptions.ConsumerGroupID,
		eventhandlers.NewPaymentFromSubmitterEventHandler(eventhandlers.PaymentFromSubmitterEventHandlerOptions{
			DBConnectionPool:    dbConnectionPool,
			TSSDBConnectionPool: tssDBConnectionPool,
			CrashTrackerClient:  serveOpts.CrashTrackerClient.Clone(),
		}),
		eventhandlers.NewPatchAnchorPlatformTransactionCompletionEventHandler(eventhandlers.PatchAnchorPlatformTransactionCompletionEventHandlerOptions{
			DBConnectionPool:   dbConnectionPool,
			APapiSvc:           serveOpts.AnchorPlatformAPIService,
			CrashTrackerClient: serveOpts.CrashTrackerClient.Clone(),
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Payment Completed Kafka Consumer: %w", err)
	}

	go events.Consume(ctx, smsInvitationConsumer, serveOpts.CrashTrackerClient.Clone())
	go events.Consume(ctx, paymentCompletedConsumer, serveOpts.CrashTrackerClient.Clone())

	return TearDownFunc(func() {
		defer dbConnectionPool.Close()
		defer tssDBConnectionPool.Close()
	}), nil
}

func (c *ServeCommand) Command(serverService ServerServiceInterface, monitorService monitor.MonitorServiceInterface) *cobra.Command {
	serveOpts := serve.ServeOptions{}
	schedulerOptions := scheduler.SchedulerOptions{}

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
			Name:      "instance-name",
			Usage:     `Name of the SDP instance. Example: "SDP Testnet".`,
			OptType:   types.String,
			ConfigKey: &serveOpts.InstanceName,
			Required:  true,
		},
		{
			Name:           "ec256-public-key",
			Usage:          "The EC256 Public Key used to validate the token signature. This EC key needs to be at least as strong as prime256v1 (P-256).",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionEC256PublicKey,
			ConfigKey:      &serveOpts.EC256PublicKey,
			Required:       true,
		},
		{
			Name:           "ec256-private-key",
			Usage:          "The EC256 Private Key used to sign the authentication token. This EC key needs to be at least as strong as prime256v1 (P-256).",
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
		cmdUtils.DistributionPublicKey(&serveOpts.DistributionPublicKey),
		cmdUtils.DistributionSeed(&serveOpts.DistributionSeed),
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
		cmdUtils.HorizonURLConfigOption(&serveOpts.HorizonURL),
		{
			Name:        "enable-scheduler",
			Usage:       "Enable Scheduler for SDP Backend Jobs",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.EnableScheduler,
			FlagDefault: true,
			Required:    false,
		},
		{
			Name:        "enable-multitenant-db",
			Usage:       "Enable Multi-tenant DB for SDP Backend API",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.EnableMultiTenantDB,
			FlagDefault: true,
			Required:    false,
		},
	}

	// crash tracker options
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}
	configOpts = append(configOpts, cmdUtils.CrashTrackerTypeConfigOption(&crashTrackerOptions.CrashTrackerType))

	// admin endpoint(s) options
	adminServeOpts := serveadmin.ServeOptions{}
	configOpts = append(configOpts,
		&config.ConfigOption{
			Name:        "admin-port",
			Usage:       "Port where the admin tenant server will be listening on",
			OptType:     types.Int,
			ConfigKey:   &adminServeOpts.Port,
			FlagDefault: 8003,
			Required:    true,
		})

	// metrics server options
	metricsServeOpts := serve.MetricsServeOptions{}
	configOpts = append(configOpts,
		&config.ConfigOption{
			Name:           "metrics-type",
			Usage:          `Metric monitor type. Options: "PROMETHEUS"`,
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMetricType,
			ConfigKey:      &metricsServeOpts.MetricType,
			FlagDefault:    "PROMETHEUS",
			Required:       true,
		},
		&config.ConfigOption{
			Name:        "metrics-port",
			Usage:       "Port where the metrics server will be listening on",
			OptType:     types.Int,
			ConfigKey:   &metricsServeOpts.Port,
			FlagDefault: 8002,
			Required:    true,
		})

	// event handler options
	eventHandlerOptions := events.EventHandlerOptions{}
	configOpts = append(configOpts,
		&config.ConfigOption{
			Name:        "max-invitation-sms-resend-attempts",
			Usage:       "The maximum number of attempts to resend the SMS invitation to the Receiver Wallets.",
			OptType:     types.Int,
			ConfigKey:   &eventHandlerOptions.MaxInvitationSMSResendAttempts,
			FlagDefault: 3,
			Required:    true,
		})

	// messenger config options:
	messengerOptions := message.MessengerOptions{}
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

	// event config options:
	eventBrokerOptions := cmdUtils.EventBrokerOptions{}
	configOpts = append(configOpts, cmdUtils.EventBrokerConfigOptions(&eventBrokerOptions)...)

	// signature service config options:
	sigServiceOptions := engine.SignatureServiceOptions{}
	configOpts = append(configOpts, cmdUtils.BaseSignatureServiceConfigOptions(&sigServiceOptions)...)

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
				Environment: globalOptions.Environment,
			}

			err = monitorService.Start(metricOptions)
			if err != nil {
				log.Fatalf("Error creating monitor service: %s", err.Error())
			}

			// Inject crash tracker options dependencies
			globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)

			// Inject server dependencies
			serveOpts.Environment = globalOptions.Environment
			serveOpts.GitCommit = globalOptions.GitCommit
			serveOpts.DatabaseDSN = globalOptions.DatabaseURL
			serveOpts.Version = globalOptions.Version
			serveOpts.MonitorService = monitorService
			serveOpts.BaseURL = globalOptions.BaseURL
			serveOpts.NetworkPassphrase = globalOptions.NetworkPassphrase

			// Inject metrics server dependencies
			metricsServeOpts.MonitorService = monitorService
			metricsServeOpts.Environment = globalOptions.Environment

			// Inject tenant server dependencies
			adminServeOpts.DatabaseDSN = globalOptions.DatabaseURL
			adminServeOpts.Environment = globalOptions.Environment
			adminServeOpts.GitCommit = globalOptions.GitCommit
			adminServeOpts.Version = globalOptions.Version
			adminServeOpts.NetworkPassphrase = globalOptions.NetworkPassphrase
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			// Setup the Crash Tracker client
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating crash tracker client: %s", err.Error())
			}
			serveOpts.CrashTrackerClient = crashTrackerClient

			// Setup the Email client
			emailMessengerClient, err := di.NewEmailClient(emailOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating email client: %s", err.Error())
			}
			serveOpts.EmailMessengerClient = emailMessengerClient
			adminServeOpts.EmailMessengerClient = emailMessengerClient

			// Setup the SMS client
			serveOpts.SMSMessengerClient, err = di.NewSMSClient(smsOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating SMS client: %s", err.Error())
			}

			// Setup the AP Auth enforcer
			apAPIService, err := di.NewAnchorPlatformAPIService(serveOpts.AnchorPlatformBasePlatformURL, serveOpts.AnchorPlatformOutgoingJWTSecret)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating Anchor Platform API Service: %v", err)
			}
			serveOpts.AnchorPlatformAPIService = apAPIService

			// Setup the TSSDBConnectionPool
			tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, di.TSSDBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL})
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting TSS DB connection pool: %v", err)
			}
			defer func() {
				err = db.CloseConnectionPoolIfNeeded(ctx, tssDBConnectionPool)
				if err != nil {
					log.Ctx(ctx).Errorf("closing TSS DB connection pool: %v", err)
				}
			}()

			// Setup the signature service
			sigServiceOptions.DBConnectionPool = tssDBConnectionPool
			sigServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
			sigServiceOptions.DistributionPrivateKey = serveOpts.DistributionSeed
			serveOpts.SignatureService, err = di.NewSignatureService(ctx, sigServiceOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating signature service: %v", err)
			}

			// Kafka (background)
			if eventBrokerOptions.EventBrokerType == events.KafkaEventBrokerType {
				kafkaProducer, err := events.NewKafkaProducer(cmdUtils.KafkaConfig(eventBrokerOptions))
				if err != nil {
					log.Ctx(ctx).Fatalf("error creating Kafka Producer: %v", err)
				}
				defer kafkaProducer.Close()
				serveOpts.EventProducer = kafkaProducer

				tearDownFunc, err := serverService.SetupConsumers(ctx, eventBrokerOptions, eventHandlerOptions, serveOpts)
				if err != nil {
					log.Fatalf("error setting up consumers: %v", err)
				}
				defer tearDownFunc()
			} else {
				log.Ctx(ctx).Warn("Event Broker Type is NONE.")
			}

			// Starting Scheduler Service (background job) if enabled
			if serveOpts.EnableScheduler {
				log.Ctx(ctx).Info("Starting Scheduler Service...")
				schedulerJobRegistrars, innerErr := serverService.GetSchedulerJobRegistrars(ctx, serveOpts, schedulerOptions, apAPIService)
				if innerErr != nil {
					log.Ctx(ctx).Fatalf("Error getting scheduler job registrars: %v", innerErr)
				}
				go scheduler.StartScheduler(crashTrackerClient.Clone(), schedulerJobRegistrars...)
			} else {
				log.Ctx(ctx).Warn("Scheduler Service is disabled.")
			}

			// Starting Metrics Server (background job)
			log.Ctx(ctx).Info("Starting Metrics Server...")
			go serverService.StartMetricsServe(metricsServeOpts, &serve.HTTPServer{})

			log.Ctx(ctx).Info("Starting Tenant Server...")
			go serverService.StartAdminServe(adminServeOpts, &serveadmin.HTTPServer{})

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
