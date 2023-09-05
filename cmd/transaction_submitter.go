package cmd

import (
	"context"
	"go/types"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	txSub "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
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
	metricsServeOpts := serve.MetricsServeOptions{}
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}

	configOpts := config.ConfigOptions{
		{
			Name:        "tss-metrics-port",
			Usage:       `Port where the metrics server will be listening on. Default: 9002"`,
			OptType:     types.Int,
			ConfigKey:   &metricsServeOpts.Port,
			FlagDefault: 9002,
			Required:    true,
		},
		{
			Name:           "tss-metrics-type",
			Usage:          `Metric monitor type. Options: "TSS_PROMETHEUS"`,
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMetricType,
			ConfigKey:      &metricsServeOpts.MetricType,
			FlagDefault:    "TSS_PROMETHEUS",
			Required:       true,
		},
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account used to disburse funds",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &submitterOpts.DistributionSeed,
			Required:       true,
		},
		{
			Name:        "horizon-url",
			Usage:       "Horizon URL",
			OptType:     types.String,
			ConfigKey:   &submitterOpts.HorizonURL,
			FlagDefault: horizonclient.DefaultTestNetClient.HorizonURL,
			Required:    true,
		},
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
		{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a Stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &submitterOpts.MaxBaseFee,
			FlagDefault: 100 * txnbuild.MinBaseFee,
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
	}
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
				Environment: globalOptions.environment,
			}

			monitorClient, err := monitor.GetClient(metricOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating monitor client: %s", err.Error())
			}

			tssMonitorSvc := tssMonitor.TSSMonitorService{
				Client:        monitorClient,
				GitCommitHash: globalOptions.gitCommit,
				Version:       globalOptions.version,
			}
			metricsServeOpts.MonitorService = &tssMonitorSvc

			// Inject server dependencies
			submitterOpts.MonitorService = tssMonitorSvc
			submitterOpts.DatabaseDSN = globalOptions.databaseURL
			submitterOpts.NetworkPassphrase = globalOptions.networkPassphrase
			submitterOpts.PrivateKeyEncrypter = tssUtils.DefaultPrivateKeyEncrypter{}

			// Inject crash tracker options dependencies
			globalOptions.populateCrashTrackerOptions(&crashTrackerOptions)
			// Setup default Crash Tracker client
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating crash tracker client: %s", err.Error())
			}
			submitterOpts.CrashTrackerClient = crashTrackerClient
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
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
