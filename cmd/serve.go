package cmd

import (
	"context"
	"fmt"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/eventhandlers"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler/jobs"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	serveadmin "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/serve"
)

type ServeCommand struct{}

type ServerServiceInterface interface {
	StartServe(opts serve.ServeOptions, httpServer serve.HTTPServerInterface)
	StartMetricsServe(opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface)
	StartAdminServe(opts serveadmin.ServeOptions, httpServer serveadmin.HTTPServerInterface)
	GetSchedulerJobRegistrars(ctx context.Context,
		serveOpts serve.ServeOptions,
		schedulerOptions scheduler.SchedulerOptions,
		apAPIService anchorplatform.AnchorPlatformAPIServiceInterface,
		tssDBConnectionPool db.DBConnectionPool) ([]scheduler.SchedulerJobRegisterOption, error)
	SetupConsumers(ctx context.Context, o SetupConsumersOptions) error
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

func (s *ServerService) GetSchedulerJobRegistrars(
	ctx context.Context,
	serveOpts serve.ServeOptions,
	schedulerOptions scheduler.SchedulerOptions,
	apAPIService anchorplatform.AnchorPlatformAPIServiceInterface,
	tssDBConnectionPool db.DBConnectionPool,
) ([]scheduler.SchedulerJobRegisterOption, error) {
	models, err := data.NewModels(serveOpts.MtnDBConnectionPool)
	if err != nil {
		log.Ctx(ctx).Fatalf("error creating models in Job Scheduler: %s", err.Error())
	}

	sj := []scheduler.SchedulerJobRegisterOption{
		scheduler.WithAPAuthEnforcementJob(apAPIService, serveOpts.MonitorService, serveOpts.CrashTrackerClient.Clone()),
		scheduler.WithReadyPaymentsCancellationJobOption(models),
	}

	if serveOpts.EnableScheduler {
		if schedulerOptions.PaymentJobIntervalSeconds < jobs.DefaultMinimumJobIntervalSeconds {
			log.Fatalf("PaymentJobIntervalSeconds is lower than the default value of %d", jobs.DefaultMinimumJobIntervalSeconds)
		}

		if schedulerOptions.ReceiverInvitationJobIntervalSeconds < jobs.DefaultMinimumJobIntervalSeconds {
			log.Fatalf("ReceiverInvitationJobIntervalSeconds is lower than the default value of %d", jobs.DefaultMinimumJobIntervalSeconds)
		}

		sj = append(sj,
			scheduler.WithPaymentToSubmitterJobOption(schedulerOptions.PaymentJobIntervalSeconds, models, tssDBConnectionPool),
			scheduler.WithPaymentFromSubmitterJobOption(schedulerOptions.PaymentJobIntervalSeconds, models, tssDBConnectionPool),
			scheduler.WithPatchAnchorPlatformTransactionsCompletionJobOption(schedulerOptions.PaymentJobIntervalSeconds, apAPIService, models),
			scheduler.WithSendReceiverWalletsSMSInvitationJobOption(jobs.SendReceiverWalletsSMSInvitationJobOptions{
				Models:                         models,
				MessengerClient:                serveOpts.SMSMessengerClient,
				MaxInvitationSMSResendAttempts: int64(serveOpts.MaxInvitationSMSResendAttempts),
				Sep10SigningPrivateKey:         serveOpts.Sep10SigningPrivateKey,
				CrashTrackerClient:             serveOpts.CrashTrackerClient.Clone(),
				JobIntervalSeconds:             schedulerOptions.ReceiverInvitationJobIntervalSeconds,
			}),
		)
	}

	return sj, nil
}

type SetupConsumersOptions struct {
	EventBrokerOptions  cmdUtils.EventBrokerOptions
	ServeOpts           serve.ServeOptions
	TSSDBConnectionPool db.DBConnectionPool
}

func (s *ServerService) SetupConsumers(ctx context.Context, o SetupConsumersOptions) error {
	kafkaConfig := cmdUtils.KafkaConfig(o.EventBrokerOptions)

	smsInvitationConsumer, err := events.NewKafkaConsumer(
		kafkaConfig,
		events.ReceiverWalletNewInvitationTopic,
		o.EventBrokerOptions.ConsumerGroupID,
		eventhandlers.NewSendReceiverWalletsSMSInvitationEventHandler(eventhandlers.SendReceiverWalletsSMSInvitationEventHandlerOptions{
			MtnDBConnectionPool:            o.ServeOpts.MtnDBConnectionPool,
			AdminDBConnectionPool:          o.ServeOpts.AdminDBConnectionPool,
			AnchorPlatformBaseSepURL:       o.ServeOpts.AnchorPlatformBasePlatformURL,
			MessengerClient:                o.ServeOpts.SMSMessengerClient,
			MaxInvitationSMSResendAttempts: int64(o.ServeOpts.MaxInvitationSMSResendAttempts),
			Sep10SigningPrivateKey:         o.ServeOpts.Sep10SigningPrivateKey,
		}),
	)
	if err != nil {
		return fmt.Errorf("creating SMS Invitation Kafka Consumer: %w", err)
	}

	paymentCompletedConsumer, err := events.NewKafkaConsumer(
		kafkaConfig,
		events.PaymentCompletedTopic,
		o.EventBrokerOptions.ConsumerGroupID,
		eventhandlers.NewPaymentFromSubmitterEventHandler(eventhandlers.PaymentFromSubmitterEventHandlerOptions{
			AdminDBConnectionPool: o.ServeOpts.AdminDBConnectionPool,
			MtnDBConnectionPool:   o.ServeOpts.MtnDBConnectionPool,
			TSSDBConnectionPool:   o.TSSDBConnectionPool,
		}),
		eventhandlers.NewPatchAnchorPlatformTransactionCompletionEventHandler(eventhandlers.PatchAnchorPlatformTransactionCompletionEventHandlerOptions{
			AdminDBConnectionPool: o.ServeOpts.AdminDBConnectionPool,
			MtnDBConnectionPool:   o.ServeOpts.MtnDBConnectionPool,
			APapiSvc:              o.ServeOpts.AnchorPlatformAPIService,
		}),
	)
	if err != nil {
		return fmt.Errorf("creating Payment Completed Kafka Consumer: %w", err)
	}

	paymentReadyToPayConsumer, err := events.NewKafkaConsumer(
		kafkaConfig,
		events.PaymentReadyToPayTopic,
		o.EventBrokerOptions.ConsumerGroupID,
		eventhandlers.NewPaymentToSubmitterEventHandler(eventhandlers.PaymentToSubmitterEventHandlerOptions{
			AdminDBConnectionPool: o.ServeOpts.AdminDBConnectionPool,
			MtnDBConnectionPool:   o.ServeOpts.MtnDBConnectionPool,
			TSSDBConnectionPool:   o.TSSDBConnectionPool,
		}),
	)
	if err != nil {
		return fmt.Errorf("creating Payment Ready to Pay Kafka Consumer: %w", err)
	}

	producer, err := events.NewKafkaProducer(kafkaConfig)
	if err != nil {
		return fmt.Errorf("creating Kafka producer: %w", err)
	}

	go events.NewEventConsumer(smsInvitationConsumer, producer, o.ServeOpts.CrashTrackerClient.Clone()).Consume(ctx)
	go events.NewEventConsumer(paymentCompletedConsumer, producer, o.ServeOpts.CrashTrackerClient.Clone()).Consume(ctx)
	go events.NewEventConsumer(paymentReadyToPayConsumer, producer, o.ServeOpts.CrashTrackerClient.Clone()).Consume(ctx)

	return nil
}

func (c *ServeCommand) Command(serverService ServerServiceInterface, monitorService monitor.MonitorServiceInterface) *cobra.Command {
	serveOpts := serve.ServeOptions{}

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
			Name:        "disable-mfa",
			Usage:       "Disables the email Multi-Factor Authentication (MFA).",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.DisableMFA,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:        "disable-recaptcha",
			Usage:       "Disables ReCAPTCHA for login and forgot password.",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.DisableReCAPTCHA,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:        "enable-scheduler",
			Usage:       "Enable Scheduler Jobs. Either this or Event Brokers must be enabled.",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.EnableScheduler,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:        "max-invitation-sms-resend-attempts",
			Usage:       "The maximum number of attempts to resend the SMS invitation to the Receiver Wallets.",
			OptType:     types.Int,
			ConfigKey:   &serveOpts.MaxInvitationSMSResendAttempts,
			FlagDefault: 3,
			Required:    true,
		},
		{
			Name:        "single-tenant-mode",
			Usage:       "This option enables the Single Tenant Mode feature. In the case where multi-tenancy is not required, this options bypasses the tenant resolution by always resolving to the default tenant configured in the database.",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.SingleTenantMode,
			FlagDefault: false,
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
		},
		&config.ConfigOption{
			Name:      "admin-account",
			Usage:     "ID of the admin account. To use, add to the request header as 'Authorization', formatted as Base64-encoded 'ADMIN_ACCOUNT:ADMIN_API_KEY'.",
			OptType:   types.String,
			ConfigKey: &adminServeOpts.AdminAccount,
			Required:  true,
		},
		&config.ConfigOption{
			Name:      "admin-api-key",
			Usage:     "API key for the admin account. To use, add to the request header as 'Authorization', formatted as Base64-encoded 'ADMIN_ACCOUNT:ADMIN_API_KEY'.",
			OptType:   types.String,
			ConfigKey: &adminServeOpts.AdminApiKey,
			Required:  true,
		},
		cmdUtils.TenantXLMBootstrapAmount(&adminServeOpts.TenantAccountNativeAssetBootstrapAmount),
	)

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

	// distribution account resolver options:
	distAccResolverOpts := signing.DistributionAccountResolverOptions{}
	configOpts = append(
		configOpts,
		cmdUtils.DistributionPublicKey(&distAccResolverOpts.HostDistributionAccountPublicKey),
	)

	// signature service config options:
	txSubmitterOpts := di.TxSubmitterEngineOptions{}
	configOpts = append(
		configOpts,
		cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)...,
	)

	// scheduler options
	schedulerOpts := scheduler.SchedulerOptions{}
	configOpts = append(
		configOpts,
		cmdUtils.SchedulerConfigOptions(&schedulerOpts)...,
	)

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
			serveOpts.Version = globalOptions.Version
			serveOpts.MonitorService = monitorService
			serveOpts.BaseURL = globalOptions.BaseURL
			serveOpts.NetworkPassphrase = globalOptions.NetworkPassphrase
			serveOpts.DistAccEncryptionPassphrase = txSubmitterOpts.SignatureServiceOptions.DistAccEncryptionPassphrase

			// Inject metrics server dependencies
			metricsServeOpts.MonitorService = monitorService
			metricsServeOpts.Environment = globalOptions.Environment

			// Inject tenant server dependencies
			adminServeOpts.Environment = globalOptions.Environment
			adminServeOpts.GitCommit = globalOptions.GitCommit
			adminServeOpts.Version = globalOptions.Version
			adminServeOpts.NetworkPassphrase = globalOptions.NetworkPassphrase
			adminServeOpts.BaseURL = globalOptions.BaseURL
			adminServeOpts.SDPUIBaseURL = globalOptions.SDPUIBaseURL
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			// Setup the Admin DB connection pool
			dbcpOptions := di.DBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL, MonitorService: monitorService}
			adminDBConnectionPool, err := di.NewAdminDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting Admin DB connection pool: %v", err)
			}
			defer func() {
				di.CleanupInstanceByValue(ctx, adminDBConnectionPool)
			}()
			serveOpts.AdminDBConnectionPool = adminDBConnectionPool
			adminServeOpts.AdminDBConnectionPool = adminDBConnectionPool

			// Setup the Multi-tenant DB connection pool
			mtnDBConnectionPool, err := di.NewMtnDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting Multi-tenant DB connection pool: %v", err)
			}
			defer func() {
				di.CleanupInstanceByValue(ctx, serveOpts.MtnDBConnectionPool)
			}()
			serveOpts.MtnDBConnectionPool = mtnDBConnectionPool
			adminServeOpts.MTNDBConnectionPool = mtnDBConnectionPool

			// Setup the TSSDBConnectionPool
			tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting TSS DB connection pool: %v", err)
			}
			defer func() {
				di.CleanupInstanceByValue(ctx, tssDBConnectionPool)
			}()

			// Setup the Crash Tracker client
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating crash tracker client: %s", err.Error())
			}
			serveOpts.CrashTrackerClient = crashTrackerClient
			adminServeOpts.CrashTrackerClient = crashTrackerClient

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

			// Setup Distribution Account Resolver
			distAccResolverOpts.AdminDBConnectionPool = adminDBConnectionPool
			distAccResolverOpts.MTNDBConnectionPool = mtnDBConnectionPool
			distAccResolver, err := di.NewDistributionAccountResolver(ctx, distAccResolverOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating distribution account resolver: %v", err)
			}
			txSubmitterOpts.SignatureServiceOptions.DistributionAccountResolver = distAccResolver

			// Setup the Submitter Engine
			txSubmitterOpts.SignatureServiceOptions.DBConnectionPool = tssDBConnectionPool
			txSubmitterOpts.SignatureServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
			submitterEngine, err := di.NewTxSubmitterEngine(ctx, txSubmitterOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating submitter engine: %v", err)
			}
			serveOpts.SubmitterEngine = submitterEngine
			adminServeOpts.SubmitterEngine = submitterEngine

			// Setup NetworkType
			serveOpts.NetworkType, err = utils.GetNetworkTypeFromNetworkPassphrase(serveOpts.NetworkPassphrase)
			if err != nil {
				log.Ctx(ctx).Fatalf("error parsing network type: %v", err)
			}

			distributionAccountServiceOptions := services.DistributionAccountServiceOptions{HorizonClient: submitterEngine.HorizonClient}
			distributionAccountService, err := di.NewDistributionAccountService(ctx, distributionAccountServiceOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating distribution account service: %v", err)
			}
			serveOpts.DistributionAccountService = distributionAccountService
			adminServeOpts.DistributionAccountService = distributionAccountService

			// Inject Circle Service dependencies
			circleService, err := di.NewCircleService(ctx, circle.ServiceOptions{
				ClientFactory:        circle.NewClient,
				ClientConfigModel:    circle.NewClientConfigModel(serveOpts.MtnDBConnectionPool),
				NetworkType:          serveOpts.NetworkType,
				EncryptionPassphrase: serveOpts.DistAccEncryptionPassphrase,
			})
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating Circle service: %v", err)
			}
			serveOpts.CircleService = circleService

			// Validate the Event Broker Type and Scheduler Jobs
			if eventBrokerOptions.EventBrokerType == events.NoneEventBrokerType && !serveOpts.EnableScheduler {
				log.Ctx(ctx).Fatalf("Both Event Brokers and Scheduler are disabled. Please enable one.")
			}
			if eventBrokerOptions.EventBrokerType != events.NoneEventBrokerType && serveOpts.EnableScheduler {
				log.Ctx(ctx).Fatalf("Both Event Brokers and Scheduler are enabled. Please enable only one.")
			}

			// Kafka (background)
			if eventBrokerOptions.EventBrokerType == events.KafkaEventBrokerType {
				kafkaProducer, kafkaErr := events.NewKafkaProducer(cmdUtils.KafkaConfig(eventBrokerOptions))
				if kafkaErr != nil {
					log.Ctx(ctx).Fatalf("error creating Kafka Producer: %v", kafkaErr)
				}
				defer kafkaProducer.Close(ctx)
				serveOpts.EventProducer = kafkaProducer

				kafkaErr = serverService.SetupConsumers(ctx, SetupConsumersOptions{
					EventBrokerOptions:  eventBrokerOptions,
					ServeOpts:           serveOpts,
					TSSDBConnectionPool: tssDBConnectionPool,
				})
				if kafkaErr != nil {
					log.Fatalf("error setting up consumers: %v", kafkaErr)
				}
			} else {
				log.Ctx(ctx).Warn("Event Broker Type is NONE. Using Noop producer for logging events")
				serveOpts.EventProducer = events.NoopProducer{}
			}

			// Starting Scheduler Service (background job) if enabled
			log.Ctx(ctx).Info("Starting Scheduler Service...")
			schedulerJobRegistrars, innerErr := serverService.GetSchedulerJobRegistrars(ctx, serveOpts, schedulerOpts, apAPIService, tssDBConnectionPool)
			if innerErr != nil {
				log.Ctx(ctx).Fatalf("Error getting scheduler job registrars: %v", innerErr)
			}
			go scheduler.StartScheduler(serveOpts.AdminDBConnectionPool, crashTrackerClient.Clone(), schedulerJobRegistrars...)

			// Starting Metrics Server (background job)
			log.Ctx(ctx).Info("Starting Metrics Server...")
			go serverService.StartMetricsServe(metricsServeOpts, &serve.HTTPServer{})

			log.Ctx(ctx).Info("Starting Tenant Server...")
			adminServeOpts.SingleTenantMode = serveOpts.SingleTenantMode
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
