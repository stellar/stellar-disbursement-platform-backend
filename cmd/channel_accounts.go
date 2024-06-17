package cmd

import (
	"context"
	"fmt"
	"go/types"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	tssMonitor "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/monitor"
	txSubSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
)

//go:generate mockery --name=ChAccCmdServiceInterface --case=underscore --structname=MockChAccCmdServiceInterface
type ChAccCmdServiceInterface interface {
	ViewChannelAccounts(ctx context.Context, dbConnectionPool db.DBConnectionPool) error
	CreateChannelAccounts(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, count int) error
	EnsureChannelAccountsCount(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, count int) error
	DeleteChannelAccount(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, opts txSubSvc.DeleteChannelAccountsOptions) error
	VerifyChannelAccounts(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, deleteInvalidAccounts bool) error
}

type ChAccCmdService struct{}

func (s *ChAccCmdService) ViewChannelAccounts(ctx context.Context, dbConnectionPool db.DBConnectionPool) error {
	return txSubSvc.ViewChannelAccounts(ctx, dbConnectionPool)
}

func (s *ChAccCmdService) CreateChannelAccounts(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, count int) error {
	return chAccService.CreateChannelAccounts(ctx, count)
}

func (s *ChAccCmdService) EnsureChannelAccountsCount(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, count int) error {
	return chAccService.EnsureChannelAccountsCount(ctx, count)
}

func (s *ChAccCmdService) DeleteChannelAccount(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, opts txSubSvc.DeleteChannelAccountsOptions) error {
	return chAccService.DeleteChannelAccount(ctx, opts)
}

func (s *ChAccCmdService) VerifyChannelAccounts(ctx context.Context, chAccService txSubSvc.ChannelAccountsService, deleteInvalidAccounts bool) error {
	return chAccService.VerifyChannelAccounts(ctx, deleteInvalidAccounts)
}

var _ ChAccCmdServiceInterface = (*ChAccCmdService)(nil)

type ChannelAccountsCommand struct {
	// Shared:
	CrashTrackerClient  crashtracker.CrashTrackerClient
	TSSDBConnectionPool db.DBConnectionPool
	DistAccResolver     signing.DistributionAccountResolver
}

func (c *ChannelAccountsCommand) Command(cmdService ChAccCmdServiceInterface) *cobra.Command {
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}

	configOpts := config.ConfigOptions{
		cmdUtils.CrashTrackerTypeConfigOption(&crashTrackerOptions.CrashTrackerType),
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

	// distribution account resolver options:
	distAccResolverOpts := signing.DistributionAccountResolverOptions{}
	configOpts = append(
		configOpts,
		cmdUtils.DistributionPublicKey(&distAccResolverOpts.HostDistributionAccountPublicKey),
	)

	channelAccountsCmd := &cobra.Command{
		Use:   "channel-accounts",
		Short: "Channel accounts related commands",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %v", err)
			}

			// Initializing the monitor service
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

			// Setup the TSSDBConnectionPool
			dbcpOptions := di.DBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL, MonitorService: &tssMonitorSvc}
			c.TSSDBConnectionPool, err = di.NewTSSDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating TSS DB connection pool: %v", err)
			}

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
			c.DistAccResolver = distributionAccountResolver

			// Inject crash tracker options dependencies
			globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)
			c.CrashTrackerClient, err = di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating crash tracker client: %v", err)
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			di.CleanupInstanceByValue(cmd.Context(), c.TSSDBConnectionPool)
			di.CleanupInstanceByKey(cmd.Context(), di.AdminDBConnectionPoolInstanceName)
		},
	}
	err := configOpts.Init(channelAccountsCmd)
	if err != nil {
		log.Ctx(channelAccountsCmd.Context()).Fatalf("Error initializing %s command: %v", channelAccountsCmd.Name(), err)
	}

	channelAccountsCmd.AddCommand(c.ViewCommand(cmdService))
	channelAccountsCmd.AddCommand(c.CreateCommand(cmdService))
	channelAccountsCmd.AddCommand(c.DeleteCommand(cmdService))
	channelAccountsCmd.AddCommand(c.EnsureCommand(cmdService))
	channelAccountsCmd.AddCommand(c.VerifyCommand(cmdService))

	return channelAccountsCmd
}

func (c *ChannelAccountsCommand) CreateCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	chAccService := txSubSvc.ChannelAccountsService{}
	txSubmitterOpts := di.TxSubmitterEngineOptions{}
	configOpts := cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)

	createCmd := &cobra.Command{
		Use:   "create [count]",
		Short: "Create channel accounts",
		Args:  cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &chAccService, &txSubmitterOpts); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error running persistent pre run: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			count, err := strconv.Atoi(args[0])
			if err != nil {
				log.Ctx(ctx).Fatalf("Invalid [count] argument: %s", args[0])
			}

			if err = cmdService.CreateChannelAccounts(ctx, chAccService, count); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts create crash")
				log.Ctx(ctx).Fatalf("Error creating channel accounts: %v", err)
			}
		},
	}
	if err := configOpts.Init(createCmd); err != nil {
		log.Ctx(createCmd.Context()).Fatalf("Error initializing %s command: %v", createCmd.Name(), err)
	}

	return createCmd
}

func (c *ChannelAccountsCommand) VerifyCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	chAccService := txSubSvc.ChannelAccountsService{}
	txSubmitterOpts := di.TxSubmitterEngineOptions{}
	configOpts := cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)

	var deleteInvalidAccounts bool
	configOpts = append(configOpts, &config.ConfigOption{
		Name:        "delete-invalid-accounts",
		Usage:       "Delete channel accounts from storage that are verified to be invalid on the network",
		OptType:     types.Bool,
		ConfigKey:   &deleteInvalidAccounts,
		FlagDefault: false,
		Required:    false,
	})

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify that all the channel accounts in the database exist on the Stellar newtwork",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &chAccService, &txSubmitterOpts); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error running persistent pre run: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := cmdService.VerifyChannelAccounts(ctx, chAccService, deleteInvalidAccounts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts create crash")
				log.Ctx(ctx).Fatalf("Error creating channel accounts: %v", err)
			}
		},
	}
	if err := configOpts.Init(verifyCmd); err != nil {
		log.Ctx(verifyCmd.Context()).Fatalf("Error initializing %s command: %v", verifyCmd.Name(), err)
	}

	return verifyCmd
}

func (c *ChannelAccountsCommand) EnsureCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	chAccService := txSubSvc.ChannelAccountsService{}
	txSubmitterOpts := di.TxSubmitterEngineOptions{}
	configOpts := cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)

	ensureCmd := &cobra.Command{
		Use: "ensure",
		Short: "Ensure we are managing exactly the number of channel accounts " +
			"equal to some specified count by dynamically increasing or decreasing the number of managed " +
			"channel accounts in storage and onchain",
		Args: cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &chAccService, &txSubmitterOpts); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error running persistent pre run: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			count, err := strconv.Atoi(args[0])
			if err != nil {
				log.Ctx(ctx).Fatalf("Invalid [count] argument: %s", args[0])
			}

			if err = cmdService.EnsureChannelAccountsCount(ctx, chAccService, count); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts ensure crash")
				log.Ctx(ctx).Fatalf("Error ensuring count for channel accounts: %v", err)
			}
		},
	}
	if err := configOpts.Init(ensureCmd); err != nil {
		log.Ctx(ensureCmd.Context()).Fatalf("Error initializing %s command: %v", ensureCmd.Name(), err)
	}

	return ensureCmd
}

func (c *ChannelAccountsCommand) DeleteCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	chAccService := txSubSvc.ChannelAccountsService{}
	txSubmitterOpts := di.TxSubmitterEngineOptions{}
	configOpts := cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)

	deleteChAccOpts := txSubSvc.DeleteChannelAccountsOptions{}
	configOpts = append(configOpts, []*config.ConfigOption{
		{
			Name:      "channel-account-id",
			Usage:     "The ID of the channel account to delete",
			OptType:   types.String,
			ConfigKey: &deleteChAccOpts.ChannelAccountID,
			Required:  false,
		},
		{
			Name:        "delete-all-accounts",
			Usage:       "Delete all managed channel accoounts in the database and on the network",
			OptType:     types.Bool,
			ConfigKey:   &deleteChAccOpts.DeleteAllAccounts,
			FlagDefault: false,
			Required:    false,
		},
	}...)

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a specified channel account from storage and on the network",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &chAccService, &txSubmitterOpts); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error running persistent pre run: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := cmdService.DeleteChannelAccount(ctx, chAccService, deleteChAccOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts delete crash")
				log.Ctx(ctx).Fatalf("Error deleting channel account: %v", err)
			}
		},
	}
	if err := configOpts.Init(deleteCmd); err != nil {
		log.Ctx(deleteCmd.Context()).Fatalf("Error initializing %s command: %v", deleteCmd.Name(), err)
	}

	deleteCmd.MarkFlagsMutuallyExclusive("channel-account-id", "delete-all-accounts")

	return deleteCmd
}

func (c *ChannelAccountsCommand) ViewCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "view",
		Short: "List public keys of all channel accounts currently stored in the database",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmdUtils.PropagatePersistentPreRun(cmd, args)
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := cmdService.ViewChannelAccounts(ctx, c.TSSDBConnectionPool); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts view crash")
				log.Ctx(ctx).Fatalf("Error viewing channel accounts: %v", err)
			}
		},
	}

	return listCmd
}

// chAccServicePersistentPreRun runs the persistent pre run for the channel accounts service to be used when the
// signature service is needed.
func (c *ChannelAccountsCommand) chAccServicePersistentPreRun(
	cmd *cobra.Command,
	args []string,
	configOpts config.ConfigOptions,
	chAccService *txSubSvc.ChannelAccountsService,
	txSubmitterOpts *di.TxSubmitterEngineOptions,
) error {
	ctx := cmd.Context()
	cmd.Parent().PersistentPreRun(cmd.Parent(), args)

	// Validate & ingest input parameters
	configOpts.Require()
	if err := configOpts.SetValues(); err != nil {
		return fmt.Errorf("setting values of config options in %s: %w", cmd.Name(), err)
	}

	// Prepare the signature service
	txSubmitterOpts.SignatureServiceOptions.DBConnectionPool = c.TSSDBConnectionPool
	txSubmitterOpts.SignatureServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
	txSubmitterOpts.SignatureServiceOptions.DistributionAccountResolver = c.DistAccResolver
	submitterEngine, err := di.NewTxSubmitterEngine(ctx, *txSubmitterOpts)
	if err != nil {
		log.Ctx(ctx).Fatalf("error creating submitter engine: %v", err)
	}

	// Inject channel account service dependencies
	chAccService.SubmitterEngine = submitterEngine
	chAccService.TSSDBConnectionPool = c.TSSDBConnectionPool

	return nil
}
