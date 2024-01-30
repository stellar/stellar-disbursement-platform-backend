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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
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
}

func (c *ChannelAccountsCommand) Command(cmdService ChAccCmdServiceInterface) *cobra.Command {
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}

	configOpts := config.ConfigOptions{
		cmdUtils.CrashTrackerTypeConfigOption(&crashTrackerOptions.CrashTrackerType),
	}
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

			c.TSSDBConnectionPool, err = di.NewTSSDBConnectionPool(ctx, di.TSSDBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL})
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating TSS DB connection pool: %v", err)
			}

			// Inject crash tracker options dependencies
			globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)
			c.CrashTrackerClient, err = di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating crash tracker client: %v", err)
			}
		},
	}
	err := configOpts.Init(channelAccountsCmd)
	if err != nil {
		log.Fatalf("Error initializing %s command: %v", channelAccountsCmd.Name(), err)
	}

	channelAccountsCmd.AddCommand(c.ViewCommand(cmdService))
	channelAccountsCmd.AddCommand(c.CreateCommand(cmdService))
	channelAccountsCmd.AddCommand(c.DeleteCommand(cmdService))
	channelAccountsCmd.AddCommand(c.EnsureCommand(cmdService))
	channelAccountsCmd.AddCommand(c.VerifyCommand(cmdService))

	return channelAccountsCmd
}

func (c *ChannelAccountsCommand) CreateCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	var horizonURL string
	chAccService := txSubSvc.ChannelAccountsService{}
	sigServiceOptions := engine.SignatureServiceOptions{}
	configOpts := chAccServiceConfigOptions(&horizonURL, &chAccService, &sigServiceOptions)

	createCmd := &cobra.Command{
		Use:   "create [count]",
		Short: "Create channel accounts",
		Args:  cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &horizonURL, &chAccService, &sigServiceOptions); err != nil {
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
		log.Fatalf("Error initializing %s command: %v", createCmd.Name(), err)
	}

	return createCmd
}

func (c *ChannelAccountsCommand) VerifyCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	var horizonURL string
	chAccService := txSubSvc.ChannelAccountsService{}
	sigServiceOptions := engine.SignatureServiceOptions{}
	configOpts := chAccServiceConfigOptions(&horizonURL, &chAccService, &sigServiceOptions)

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
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &horizonURL, &chAccService, &sigServiceOptions); err != nil {
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
		log.Fatalf("Error initializing %s command: %v", verifyCmd.Name(), err)
	}

	return verifyCmd
}

func (c *ChannelAccountsCommand) EnsureCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	var horizonURL string
	chAccService := txSubSvc.ChannelAccountsService{}
	sigServiceOptions := engine.SignatureServiceOptions{}
	configOpts := chAccServiceConfigOptions(&horizonURL, &chAccService, &sigServiceOptions)

	ensureCmd := &cobra.Command{
		Use: "ensure",
		Short: "Ensure we are managing exactly the number of channel accounts " +
			"equal to some specified count by dynamically increasing or decreasing the number of managed " +
			"channel accounts in storage and onchain",
		Args: cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &horizonURL, &chAccService, &sigServiceOptions); err != nil {
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
		log.Fatalf("Error initializing %s command: %v", ensureCmd.Name(), err)
	}

	return ensureCmd
}

func (c *ChannelAccountsCommand) DeleteCommand(cmdService ChAccCmdServiceInterface) *cobra.Command {
	var horizonURL string
	chAccService := txSubSvc.ChannelAccountsService{}
	sigServiceOptions := engine.SignatureServiceOptions{}
	configOpts := chAccServiceConfigOptions(&horizonURL, &chAccService, &sigServiceOptions)

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
			if err := c.chAccServicePersistentPreRun(cmd, args, configOpts, &horizonURL, &chAccService, &sigServiceOptions); err != nil {
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
		log.Fatalf("Error initializing %s command: %v", deleteCmd.Name(), err)
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

// chAccServiceConfigOptions returns the config options for the channel accounts service to be used when the signature
// service is needed.
func chAccServiceConfigOptions(
	horizonURL *string,
	chAccService *txSubSvc.ChannelAccountsService,
	sigServiceOptions *engine.SignatureServiceOptions,
) config.ConfigOptions {
	return append(
		// signature service options:
		cmdUtils.BaseSignatureServiceConfigOptions(sigServiceOptions),
		cmdUtils.DistributionSeed(&sigServiceOptions.DistributionPrivateKey),
		// other shared options:
		cmdUtils.HorizonURLConfigOption(horizonURL),
		cmdUtils.MaxBaseFee(&chAccService.MaxBaseFee),
	)
}

// chAccServicePersistentPreRun runs the persistent pre run for the channel accounts service to be used when the
// signature service is needed.
func (c *ChannelAccountsCommand) chAccServicePersistentPreRun(
	cmd *cobra.Command,
	args []string,
	configOpts config.ConfigOptions,
	horizonURL *string,
	chAccService *txSubSvc.ChannelAccountsService,
	sigServiceOptions *engine.SignatureServiceOptions,
) error {
	ctx := cmd.Context()
	cmd.Parent().PersistentPreRun(cmd.Parent(), args)

	// Validate & ingest input parameters
	configOpts.Require()
	if err := configOpts.SetValues(); err != nil {
		return fmt.Errorf("setting values of config options in %s: %w", cmd.Name(), err)
	}

	// Prepare horizonClient
	horizonClient, err := di.NewHorizonClient(ctx, *horizonURL)
	if err != nil {
		return fmt.Errorf("retrieving horizon client through the dependency injector in %s: %w", cmd.Name(), err)
	}

	// Prepare the signature service
	sigServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
	sigServiceOptions.DBConnectionPool = c.TSSDBConnectionPool
	sigService, err := di.NewSignatureService(ctx, *sigServiceOptions)
	if err != nil {
		return fmt.Errorf("retrieving signing service through the dependency injector in %s: %w", cmd.Name(), err)
	}

	// Inject channel account service dependencies
	chAccService.SigningService = sigService
	chAccService.TSSDBConnectionPool = c.TSSDBConnectionPool
	chAccService.HorizonClient = horizonClient

	return nil
}
