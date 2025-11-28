package cmd

import (
	"context"
	"go/types"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/support/config"
	"github.com/stellar/go-stellar-sdk/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	txSub "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
)

type TxSubmitterCommand struct{}

type TxSubmitterServiceInterface interface {
	StartSubmitter(context.Context, txSub.SubmitterOptions)
	StartMetricsServe(ctx context.Context, opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface, crashTrackerClient crashtracker.CrashTrackerClient)
}

type TxSubmitterService struct{}

// StartSubmitter starts the Transaction Submission Service
func (s *TxSubmitterService) StartSubmitter(ctx context.Context, opts txSub.SubmitterOptions) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		// Wait for a termination signal
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
		<-sig

		// Cancel the context to signal the submitterService to exit
		cancel()
	}()

	tssManager, err := txSub.NewManager(ctx, opts)
	if err != nil {
		opts.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cannot start submitter service")
		log.Ctx(ctx).Fatalf("Error starting transaction submission service: %s", err.Error())
	}

	tssManager.ProcessTransactions(ctx)
}

func (s *TxSubmitterService) StartMetricsServe(ctx context.Context, opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface, crashTrackerClient crashtracker.CrashTrackerClient) {
	err := serve.MetricsServe(opts, httpServer)
	if err != nil {
		crashTrackerClient.LogAndReportErrors(ctx, err, "Cannot start metrics service")
		log.Ctx(ctx).Fatalf("Error starting metrics server: %s", err.Error())
	}
}

func (c *TxSubmitterCommand) Command(submitterService TxSubmitterServiceInterface) *cobra.Command {
	tssOpts := txSub.SubmitterOptions{}

	configOpts := config.ConfigOptions{
		{
			Name:        "num-channel-accounts",
			Usage:       "Number of channel accounts to utilize for transaction submission",
			OptType:     types.Int,
			ConfigKey:   &tssOpts.NumChannelAccounts,
			FlagDefault: 2,
			Required:    false,
		},
		{
			Name:        "queue-polling-interval",
			Usage:       "Polling interval (seconds) to query the database for pending transactions to process",
			OptType:     types.Int,
			ConfigKey:   &tssOpts.QueuePollingInterval,
			FlagDefault: 6,
			Required:    true,
		},
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

	// distribution account resolver options:
	distAccResolverOpts := signing.DistributionAccountResolverOptions{}
	configOpts = append(
		configOpts,
		cmdUtils.DistributionPublicKey(&distAccResolverOpts.HostDistributionAccountPublicKey),
	)

	// txSubmitterOpts
	txSubmitterOpts := di.TxSubmitterEngineOptions{}
	configOpts = append(
		configOpts,
		cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)...,
	)

	// DB pool tuning options (tss)
	configOpts = append(
		configOpts,
		cmdUtils.DBPoolConfigOptions(&globalOptions.DBPool)...,
	)

	// rpc options
	rpcOptions := stellar.RPCOptions{}
	configOpts = append(configOpts, cmdUtils.RPCConfigOptions(&rpcOptions)...)

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

			// Initializing the MonitorService
			tssMonitorSvc := tssMonitor.TSSMonitorService{
				GitCommitHash: globalOptions.GitCommit,
				Version:       globalOptions.Version,
			}
			err = tssMonitorSvc.Start(monitor.MetricOptions{
				MetricType:  metricsServeOpts.MetricType,
				Environment: globalOptions.Environment,
			})
			if err != nil {
				log.Ctx(ctx).Fatalf("error starting TSS monitor service: %v", err)
			}
			metricsServeOpts.MonitorService = &tssMonitorSvc
			tssOpts.MonitorService = tssMonitorSvc

			// Initializing the TSSDBConnectionPool
			dbcpOptions := di.DBConnectionPoolOptions{
				DatabaseURL:            globalOptions.DatabaseURL,
				MonitorService:         &tssMonitorSvc,
				MaxOpenConns:           globalOptions.DBPool.DBMaxOpenConns,
				MaxIdleConns:           globalOptions.DBPool.DBMaxIdleConns,
				ConnMaxIdleTimeSeconds: globalOptions.DBPool.DBConnMaxIdleTimeSeconds,
				ConnMaxLifetimeSeconds: globalOptions.DBPool.DBConnMaxLifetimeSeconds,
			}
			tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting TSS DB connection pool: %v", err)
			}
			tssOpts.DBConnectionPool = tssDBConnectionPool

			// Initializing the AdminDBConnectionPool
			adminDBConnectionPool, err := di.NewAdminDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting Admin DB connection pool: %v", err)
			}
			distAccResolverOpts.AdminDBConnectionPool = adminDBConnectionPool

			// Initializing the DistributionAccountResolver
			distributionAccountResolver, err := di.NewDistributionAccountResolver(ctx, distAccResolverOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating distribution account resolver: %v", err)
			}
			txSubmitterOpts.SignatureServiceOptions.DistributionAccountResolver = distributionAccountResolver

			// Initializing the Submitter Engine
			txSubmitterOpts.SignatureServiceOptions.DBConnectionPool = tssDBConnectionPool
			txSubmitterOpts.SignatureServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
			submitterEngine, err := di.NewTxSubmitterEngine(ctx, txSubmitterOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating submitter engine: %v", err)
			}
			tssOpts.SubmitterEngine = submitterEngine

			// Initializing the RPC Client
			if rpcOptions.RPCUrl != "" {
				rpcClient, rpcClientErr := di.NewRPCClient(ctx, rpcOptions)
				if rpcClientErr != nil {
					log.Ctx(ctx).Fatalf("error creating RPC client: %s", rpcClientErr.Error())
				}
				log.Ctx(ctx).Infof("Using RPC client with URL %s", rpcOptions.RPCUrl)
				tssOpts.RPCClient = rpcClient
			} else {
				log.Ctx(ctx).Warn("No RPC client URL provided. Embedded wallet transactions will not be submitted.")
			}

			// Initializing the CrashTrackerClient
			globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions) // parses globalOptions relevant to the crash crash tracker
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating crash tracker client: %s", err.Error())
			}
			tssOpts.CrashTrackerClient = crashTrackerClient
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			// Starting Metrics Server (background job)
			go submitterService.StartMetricsServe(ctx, metricsServeOpts, &serve.HTTPServer{}, tssOpts.CrashTrackerClient)

			// Start transaction submission service
			submitterService.StartSubmitter(ctx, tssOpts)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			di.CleanupInstanceByValue(cmd.Context(), tssOpts.DBConnectionPool)
			di.CleanupInstanceByKey(cmd.Context(), di.AdminDBConnectionPoolInstanceName)
		},
	}
	err := configOpts.Init(cmd)
	if err != nil {
		log.Ctx(cmd.Context()).Fatalf("Error initializing a config option: %s", err.Error())
	}

	return cmd
}
