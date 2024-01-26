package cmd

import (
	"context"
	"go/types"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	txSub "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
)

type TxSubmitterCommand struct{}

type TxSubmitterServiceInterface interface {
	StartSubmitter(context.Context, txSub.SubmitterOptions)
	StartMetricsServe(ctx context.Context, opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface, crashTrackerClient crashtracker.CrashTrackerClient)
}

type TxSubmitterService struct{}

// StartSubmitter starts the Transaction Submission Service
func (t *TxSubmitterService) StartSubmitter(ctx context.Context, opts txSub.SubmitterOptions) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		// Wait for a termination signal
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig

		// Cancel the context to signal the submitterService to exit
		cancel()
	}()

	tssManager, err := txSub.NewManager(ctx, opts)
	if err != nil {
		defer opts.DBConnectionPool.Close()
		opts.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cannot start submitter service")
		log.Fatalf("Error starting transaction submission service: %s", err.Error())
	}

	tssManager.ProcessTransactions(ctx)
}

func (s *TxSubmitterService) StartMetricsServe(ctx context.Context, opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface, crashTrackerClient crashtracker.CrashTrackerClient) {
	err := serve.MetricsServe(opts, httpServer)
	if err != nil {
		crashTrackerClient.LogAndReportErrors(ctx, err, "Cannot start metrics service")
		log.Fatalf("Error starting metrics server: %s", err.Error())
	}
}

func (c *TxSubmitterCommand) Command(submitterService TxSubmitterServiceInterface) *cobra.Command {
	submitterOpts := txSub.SubmitterOptions{}

	configOpts := config.ConfigOptions{
		cmdUtils.HorizonURLConfigOption(&submitterOpts.HorizonURL),
		{
			Name:        "num-channel-accounts",
			Usage:       "Number of channel accounts to utilize for transaction submission",
			OptType:     types.Int,
			ConfigKey:   &submitterOpts.NumChannelAccounts,
			FlagDefault: 2,
			Required:    false,
		},
		{
			Name:        "queue-polling-interval",
			Usage:       "Polling interval (seconds) to query the database for pending transactions to process",
			OptType:     types.Int,
			ConfigKey:   &submitterOpts.QueuePollingInterval,
			FlagDefault: 6,
			Required:    true,
		},
		cmdUtils.MaxBaseFee(&submitterOpts.MaxBaseFee),
	}

	// metrics server options
	metricsServeOpts := serve.MetricsServeOptions{}
	configOpts = append(configOpts,
		&config.ConfigOption{
			Name:           "tss-metrics-type",
			Usage:          `Metric monitor type. Options: "TSS_PROMETHEUS"`,
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMetricType,
			ConfigKey:      &metricsServeOpts.MetricType,
			FlagDefault:    "TSS_PROMETHEUS",
			Required:       true,
		},
		&config.ConfigOption{
			Name:        "tss-metrics-port",
			Usage:       `Port where the metrics server will be listening on. Default: 9002"`,
			OptType:     types.Int,
			ConfigKey:   &metricsServeOpts.Port,
			FlagDefault: 9002,
			Required:    true,
		})

	// crash tracker options
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}
	configOpts = append(configOpts, cmdUtils.CrashTrackerTypeConfigOption(&crashTrackerOptions.CrashTrackerType))

	// signature service config options:
	sigServiceOptions := di.SignatureServiceOptions{}
	configOpts = append(configOpts,
		cmdUtils.ChannelAccountEncryptionPassphraseConfigOption(&sigServiceOptions.EncryptionPassphrase),
		cmdUtils.DistributionSeed(&sigServiceOptions.DistributionPrivateKey),
	)

	// event broker options:
	eventBrokerOptions := cmdUtils.EventBrokerOptions{}
	configOpts = append(configOpts, cmdUtils.EventBrokerConfigOptions(&eventBrokerOptions)...)

	cmd := &cobra.Command{
		Use:   "tss",
		Short: "Run the Transaction Submission Service",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			ctx := cmd.Context()

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}

			// Initializing monitor service
			metricOptions := monitor.MetricOptions{
				MetricType:  metricsServeOpts.MetricType,
				Environment: globalOptions.Environment,
			}

			monitorClient, err := monitor.GetClient(metricOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating monitor client: %s", err.Error())
			}

			tssMonitorSvc := tssMonitor.TSSMonitorService{
				Client:        monitorClient,
				GitCommitHash: globalOptions.GitCommit,
				Version:       globalOptions.Version,
			}
			metricsServeOpts.MonitorService = &tssMonitorSvc

			// Setup the TSSDBConnectionPool
			tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, di.TSSDBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL})
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting TSS DB connection pool: %v", err)
			}

			// Setup the signature service
			sigServiceOptions.DBConnectionPool = tssDBConnectionPool
			sigServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
			signatureService, err := di.NewSignatureService(ctx, sigServiceOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating signature service: %v", err)
			}

			// Inject server dependencies
			submitterOpts.DBConnectionPool = tssDBConnectionPool
			submitterOpts.SignatureService = signatureService
			submitterOpts.MonitorService = tssMonitorSvc

			// Inject crash tracker options dependencies
			globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)
			// Setup default Crash Tracker client
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating crash tracker client: %s", err.Error())
			}
			submitterOpts.CrashTrackerClient = crashTrackerClient
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			if eventBrokerOptions.EventBrokerType == events.KafkaEventBrokerType {
				kafkaProducer, err := events.NewKafkaProducer(eventBrokerOptions.BrokerURLs)
				if err != nil {
					log.Ctx(ctx).Fatalf("error creating Kafka Producer: %v", err)
				}
				defer kafkaProducer.Close()
				submitterOpts.EventProducer = kafkaProducer
			} else {
				log.Ctx(ctx).Warn("Event Broker Type is NONE.")
			}

			// Starting Metrics Server (background job)
			go submitterService.StartMetricsServe(ctx, metricsServeOpts, &serve.HTTPServer{}, submitterOpts.CrashTrackerClient)

			// Start transaction submission service
			submitterService.StartSubmitter(ctx, submitterOpts)
		},
	}
	err := configOpts.Init(cmd)
	if err != nil {
		log.Fatalf("Error initializing a config option: %s", err.Error())
	}

	return cmd
}
