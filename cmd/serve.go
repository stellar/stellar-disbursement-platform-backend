package cmd

import (
	"context"
	"fmt"
	"go/types"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/support/config"
	"github.com/stellar/go-stellar-sdk/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/bridge"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler/jobs"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	serveadmin "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type ServeCommand struct{}

type ServerServiceInterface interface {
	StartServe(opts serve.ServeOptions, httpServer serve.HTTPServerInterface)
	StartMetricsServe(opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface)
	StartAdminServe(opts serveadmin.ServeOptions, httpServer serveadmin.HTTPServerInterface)
	GetSchedulerJobRegistrars(ctx context.Context,
		serveOpts serve.ServeOptions,
		schedulerOptions scheduler.SchedulerOptions,
		tssDBConnectionPool db.DBConnectionPool) ([]scheduler.SchedulerJobRegisterOption, error)
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
	tssDBConnectionPool db.DBConnectionPool,
) ([]scheduler.SchedulerJobRegisterOption, error) {
	models, err := data.NewModels(serveOpts.MtnDBConnectionPool)
	if err != nil {
		log.Ctx(ctx).Fatalf("error creating models in Job Scheduler: %s", err.Error())
	}

	sj := []scheduler.SchedulerJobRegisterOption{
		scheduler.WithReadyPaymentsCancellationJobOption(models),
		scheduler.WithCircleReconciliationJobOption(jobs.CircleReconciliationJobOptions{
			Models:              models,
			DistAccountResolver: serveOpts.SubmitterEngine.DistributionAccountResolver,
			CircleService:       serveOpts.CircleService,
		}),
	}

	if schedulerOptions.PaymentJobIntervalSeconds < jobs.DefaultMinimumJobIntervalSeconds {
		log.Fatalf("PaymentJobIntervalSeconds is lower than the default value of %d", jobs.DefaultMinimumJobIntervalSeconds)
	}

	if schedulerOptions.ReceiverInvitationJobIntervalSeconds < jobs.DefaultMinimumJobIntervalSeconds {
		log.Fatalf("ReceiverInvitationJobIntervalSeconds is lower than the default value of %d", jobs.DefaultMinimumJobIntervalSeconds)
	}

	sj = append(sj,
		scheduler.WithCirclePaymentToSubmitterJobOption(jobs.CirclePaymentToSubmitterJobOptions{
			JobIntervalSeconds:  schedulerOptions.PaymentJobIntervalSeconds,
			Models:              models,
			DistAccountResolver: serveOpts.SubmitterEngine.DistributionAccountResolver,
			CircleService:       serveOpts.CircleService,
			CircleAPIType:       serveOpts.CircleAPIType,
		}),
		scheduler.WithStellarPaymentToSubmitterJobOption(jobs.StellarPaymentToSubmitterJobOptions{
			JobIntervalSeconds:  schedulerOptions.PaymentJobIntervalSeconds,
			Models:              models,
			TSSDBConnectionPool: tssDBConnectionPool,
			DistAccountResolver: serveOpts.SubmitterEngine.DistributionAccountResolver,
		}),
		scheduler.WithPaymentFromSubmitterJobOption(schedulerOptions.PaymentJobIntervalSeconds, models, tssDBConnectionPool),
		scheduler.WithSendReceiverWalletsInvitationJobOption(jobs.SendReceiverWalletsInvitationJobOptions{
			Models:                      models,
			MessageDispatcher:           serveOpts.MessageDispatcher,
			EmbeddedWalletService:       serveOpts.EmbeddedWalletService,
			MaxInvitationResendAttempts: int64(serveOpts.MaxInvitationResendAttempts),
			Sep10SigningPrivateKey:      serveOpts.Sep10SigningPrivateKey,
			CrashTrackerClient:          serveOpts.CrashTrackerClient.Clone(),
			JobIntervalSeconds:          schedulerOptions.ReceiverInvitationJobIntervalSeconds,
		}),
	)

	// Add embedded wallet sync jobs only if enabled
	if serveOpts.EnableEmbeddedWallets {
		sj = append(sj,
			scheduler.WithWalletCreationToSubmitterJobOption(jobs.WalletCreationToSubmitterJobOptions{
				JobIntervalSeconds:  schedulerOptions.PaymentJobIntervalSeconds,
				Models:              models,
				TSSDBConnectionPool: tssDBConnectionPool,
				DistAccountResolver: serveOpts.SubmitterEngine.DistributionAccountResolver,
			}),
			scheduler.WithSponsoredTransactionsToSubmitterJobOption(jobs.SponsoredTransactionsToSubmitterJobOptions{
				JobIntervalSeconds:  schedulerOptions.PaymentJobIntervalSeconds,
				Models:              models,
				TSSDBConnectionPool: tssDBConnectionPool,
				DistAccountResolver: serveOpts.SubmitterEngine.DistributionAccountResolver,
			}),
			scheduler.WithWalletCreationFromSubmitterJobOption(
				schedulerOptions.PaymentJobIntervalSeconds,
				models,
				tssDBConnectionPool,
				serveOpts.NetworkPassphrase,
			),
			scheduler.WithSponsoredTransactionFromSubmitterJobOption(
				schedulerOptions.PaymentJobIntervalSeconds,
				models,
				tssDBConnectionPool,
			),
		)
	}

	return sj, nil
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
			Usage:     `The JWT secret that's used to sign the SEP-24 JWT token`,
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
			Name:        "enable-embedded-wallets",
			Usage:       "Enable embedded wallet features that require Stellar RPC integration",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.EnableEmbeddedWallets,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:      "embedded-wallets-wasm-hash",
			Usage:     "The WASM hash of the smart contract for embedded wallets (required when --enable-embedded-wallets is true)",
			OptType:   types.String,
			ConfigKey: &serveOpts.EmbeddedWalletsWasmHash,
			Required:  false,
		},
		{
			Name:        "webauthn-session-cache-max-entries",
			Usage:       "Maximum number of WebAuthn sessions stored for passkey flows",
			OptType:     types.Int,
			ConfigKey:   &serveOpts.WebAuthnSessionCacheMaxEntries,
			FlagDefault: 1024,
			Required:    false,
		},
		{
			Name:        "webauthn-session-ttl-seconds",
			Usage:       "Duration that WebAuthn sessions remain valid, in seconds",
			OptType:     types.Int,
			ConfigKey:   &serveOpts.WebAuthnSessionTTLSeconds,
			FlagDefault: 300,
			Required:    false,
		},
		{
			Name:        "enable-sep45",
			Usage:       "Enable SEP-45 web authentication features that require Stellar RPC integration",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.EnableSep45,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:           "sep45-contract-id",
			Usage:          "The ID of the SEP-45 web authentication contract (required when --enable-sep45 is true)",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarContractID,
			ConfigKey:      &serveOpts.Sep45ContractID,
			Required:       false,
		},
		{
			Name:        "sep10-client-attribution-required",
			Usage:       "If true, SEP-10 authentication requires client_domain to be provided and validated. If false, client_domain is optional.",
			OptType:     types.Bool,
			ConfigKey:   &serveOpts.Sep10ClientAttributionRequired,
			FlagDefault: true,
			Required:    false,
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
			Name:           "captcha-type",
			Usage:          `The type of CAPTCHA to use. Options: ["GOOGLE_RECAPTCHA_V2", "GOOGLE_RECAPTCHA_V3"].`,
			OptType:        types.String,
			ConfigKey:      &serveOpts.CAPTCHAType,
			Required:       false,
			CustomSetValue: cmdUtils.SetConfigOptionCAPTCHAType,
			FlagDefault:    string(validators.GoogleReCAPTCHAV2),
		},
		{
			Name:        "recaptcha-v3-min-score",
			Usage:       "The minimum score threshold for reCAPTCHA v3 (0.0 to 1.0, where 1.0 is very likely a good interaction). Only used when captcha-type is GOOGLE_RECAPTCHA_V3.",
			OptType:     types.Float64,
			ConfigKey:   &serveOpts.ReCAPTCHAV3MinScore,
			FlagDefault: 0.5,
			Required:    false,
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
			Name:        "max-invitation-resend-attempts",
			Usage:       "The maximum number of attempts to resend the invitation to the Receiver Wallets.",
			OptType:     types.Int,
			ConfigKey:   &serveOpts.MaxInvitationResendAttempts,
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
		{
			Name:           "circle-api-type",
			Usage:          `The Circle API type. Options: ["TRANSFERS", "PAYOUTS"]. `,
			OptType:        types.String,
			ConfigKey:      &serveOpts.CircleAPIType,
			Required:       true,
			CustomSetValue: cmdUtils.SetConfigOptionCircleAPIType,
			FlagDefault:    string(circle.APITypeTransfers),
		},
	}
	// rpc options
	configOpts = append(configOpts, cmdUtils.RPCConfigOptions(&serveOpts.RPCConfig)...)

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
			ConfigKey: &adminServeOpts.AdminAPIKey,
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

	// DB pool tuning options (serve)
	configOpts = append(
		configOpts,
		cmdUtils.DBPoolConfigOptions(&globalOptions.DBPool)...,
	)

	// bridge integration options
	bridgeIntegrationOpts := cmdUtils.BridgeIntegrationOptions{}
	configOpts = append(
		configOpts,
		cmdUtils.BridgeIntegrationConfigOptions(&bridgeIntegrationOpts)...,
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
			dbcpOptions := di.DBConnectionPoolOptions{
				DatabaseURL:            globalOptions.DatabaseURL,
				MonitorService:         monitorService,
				MaxOpenConns:           globalOptions.DBPool.DBMaxOpenConns,
				MaxIdleConns:           globalOptions.DBPool.DBMaxIdleConns,
				ConnMaxIdleTimeSeconds: globalOptions.DBPool.DBConnMaxIdleTimeSeconds,
				ConnMaxLifetimeSeconds: globalOptions.DBPool.DBConnMaxLifetimeSeconds,
			}
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
			serveOpts.TSSDBConnectionPool = tssDBConnectionPool

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

			// Setup the Message Dispatcher
			messageDispatcherOpts := di.MessageDispatcherOpts{
				EmailOpts: &emailOpts,
				SMSOpts:   &smsOpts,
			}
			serveOpts.MessageDispatcher, err = di.NewMessageDispatcher(ctx, messageDispatcherOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating message dispatcher: %s", err.Error())
			}

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

			// Inject Circle Service dependencies
			circleService, err := di.NewCircleService(ctx, circle.ServiceOptions{
				ClientFactory:        circle.NewClient,
				ClientConfigModel:    circle.NewClientConfigModel(serveOpts.MtnDBConnectionPool),
				NetworkType:          serveOpts.NetworkType,
				EncryptionPassphrase: serveOpts.DistAccEncryptionPassphrase,
				TenantManager:        tenant.NewManager(tenant.WithDatabase(serveOpts.AdminDBConnectionPool)),
				MonitorService:       serveOpts.MonitorService,
			})
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating Circle service: %v", err)
			}
			serveOpts.CircleService = circleService

			if err = bridgeIntegrationOpts.ValidateFlags(); err != nil {
				log.Ctx(ctx).Fatalf("error validating Bridge integration options: %v", err)
			}

			// Setup Distribution Account Service
			distributionAccountServiceOptions := services.DistributionAccountServiceOptions{
				NetworkType:   serveOpts.NetworkType,
				HorizonClient: submitterEngine.HorizonClient,
				CircleService: circleService,
			}
			distributionAccountService, err := di.NewDistributionAccountService(ctx, distributionAccountServiceOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating distribution account service: %v", err)
			}
			serveOpts.DistributionAccountService = distributionAccountService
			adminServeOpts.DistributionAccountService = distributionAccountService

			if bridgeIntegrationOpts.EnableBridgeIntegration {
				bridgeModels, brErr := data.NewModels(mtnDBConnectionPool)
				if brErr != nil {
					log.Ctx(ctx).Fatalf("error creating models for Bridge service: %v", brErr)
				}
				bridgeService, brErr := bridge.NewService(bridge.ServiceOptions{
					BaseURL:                     bridgeIntegrationOpts.BridgeBaseURL,
					APIKey:                      bridgeIntegrationOpts.BridgeAPIKey,
					Models:                      bridgeModels,
					DistributionAccountResolver: submitterEngine.DistributionAccountResolver,
					DistributionAccountService:  serveOpts.DistributionAccountService,
					NetworkType:                 serveOpts.NetworkType,
				})
				if brErr != nil {
					log.Ctx(ctx).Fatalf("error creating Bridge service: %v", brErr)
				}
				serveOpts.BridgeService = bridgeService
				log.Ctx(ctx).Infof("🌉 Bridge integration is enabled for base URL %s", bridgeIntegrationOpts.BridgeBaseURL)
			}

			// Setup Embedded Wallet Service (only if enabled)
			if serveOpts.EnableEmbeddedWallets {
				serveOpts.EmbeddedWalletService, err = di.NewEmbeddedWalletService(context.Background(), services.EmbeddedWalletServiceOptions{
					MTNDBConnectionPool: serveOpts.MtnDBConnectionPool,
					WasmHash:            serveOpts.EmbeddedWalletsWasmHash,
				})
				log.Info("Embedded wallet features enabled")
				if err != nil {
					log.Ctx(ctx).Fatalf("error creating embedded wallet service: %v", err)
				}

				serveOpts.WebAuthnService, err = di.NewWebAuthnService(context.Background(), di.WebAuthnServiceOptions{
					MTNDBConnectionPool:    serveOpts.MtnDBConnectionPool,
					SessionTTL:             time.Duration(serveOpts.WebAuthnSessionTTLSeconds) * time.Second,
					SessionCacheMaxEntries: serveOpts.WebAuthnSessionCacheMaxEntries,
				})
				if err != nil {
					log.Ctx(ctx).Fatalf("error creating WebAuthn service: %v", err)
				}
				log.Info("WebAuthn passkey authentication enabled")
			}

			log.Ctx(ctx).Info("Starting Scheduler Service...")
			schedulerJobRegistrars, innerErr := serverService.GetSchedulerJobRegistrars(ctx, serveOpts, schedulerOpts, tssDBConnectionPool)
			if innerErr != nil {
				log.Ctx(ctx).Fatalf("Error getting scheduler job registrars: %v", innerErr)
			}
			go scheduler.StartScheduler(serveOpts.AdminDBConnectionPool, crashTrackerClient.Clone(), schedulerJobRegistrars...)

			// Starting Metrics Server (background job)
			log.Ctx(ctx).Info("Starting Metrics Server...")
			go serverService.StartMetricsServe(metricsServeOpts, &serve.HTTPServer{})

			log.Ctx(ctx).Info("Starting Tenant Server...")
			adminServeOpts.SingleTenantMode = serveOpts.SingleTenantMode
			adminServeOpts.DisableMFA = serveOpts.DisableMFA
			adminServeOpts.DisableReCAPTCHA = serveOpts.DisableReCAPTCHA
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
