package cmd

import (
	"context"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	txSubSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
)

type ChannelAccountsCommand struct {
	Service            txSubSvc.ChannelAccountsServiceInterface
	CrashTrackerClient crashtracker.CrashTrackerClient
}

func (c *ChannelAccountsCommand) Command() *cobra.Command {
	svcOpts := &txSubSvc.ChannelAccountServiceOptions{}
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}

	configOpts := config.ConfigOptions{
		cmdUtils.CrashTrackerTypeConfigOption(&crashTrackerOptions.CrashTrackerType),
		cmdUtils.HorizonURLConfigOption(&svcOpts.HorizonUrl),
		cmdUtils.ChannelAccountEncryptionKeyConfigOption(&svcOpts.EncryptionKey),
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

			// Inject server dependencies
			tssDatabaseDSN, err := router.GetDNSForTSS(globalOptions.DatabaseURL)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting TSS database DSN: %v", err)
			}
			svcOpts.DatabaseDSN = tssDatabaseDSN
			svcOpts.NetworkPassphrase = globalOptions.NetworkPassphrase
			c.Service, err = txSubSvc.NewChannelAccountService(ctx, *svcOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating channel account service: %v", err)
			}

			// Inject crash tracker options dependencies
			globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)

			// Setup default Crash Tracker client
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating crash tracker client: %v", err)
			}
			c.CrashTrackerClient = crashTrackerClient
		},
	}
	err := configOpts.Init(channelAccountsCmd)
	if err != nil {
		log.Fatalf("Error initializing channelAccountsCmd config option: %v", err)
	}

	createCmd := c.CreateCommand(svcOpts)
	deleteCmd := c.DeleteCommand(svcOpts)
	ensureCmd := c.EnsureCommand(svcOpts)
	verifyCmd := c.VerifyCommand(svcOpts)
	viewCmd := c.ViewCommand()
	channelAccountsCmd.AddCommand(createCmd, deleteCmd, ensureCmd, verifyCmd, viewCmd)

	return channelAccountsCmd
}

func (c *ChannelAccountsCommand) CreateCommand(chAccOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account that will be used to sponsor the channel accounts",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &chAccOpts.RootSeed,
			Required:       true,
		},
		{
			Name:        "num-channel-accounts-create",
			Usage:       "The desired number of channel accounts to be created",
			OptType:     types.Int,
			ConfigKey:   &chAccOpts.NumChannelAccounts,
			FlagDefault: 1,
			Required:    true,
		},
		{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &chAccOpts.MaxBaseFee,
			FlagDefault: 100 * txnbuild.MinBaseFee,
			Required:    true,
		},
	}
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create channel accounts",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			// entrypoint into the main logic for creating channel accounts
			if err := c.Service.CreateChannelAccountsOnChain(ctx, *chAccOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts create crash")
				log.Ctx(ctx).Fatalf("Error creating channel accounts: %v", err)
			}
		},
	}
	err := configOpts.Init(createCmd)
	if err != nil {
		log.Fatalf("Error initializing createCmd: %v", err)
	}

	return createCmd
}

func (c *ChannelAccountsCommand) VerifyCommand(chAccOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:        "delete-invalid-accounts",
			Usage:       "Delete channel accounts from storage that are verified to be invalid on the network",
			OptType:     types.Bool,
			ConfigKey:   &chAccOpts.DeleteInvalidAcccounts,
			FlagDefault: false,
			Required:    false,
		},
	}

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify that all the channel accounts in the database exist on the Stellar newtwork",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := c.Service.VerifyChannelAccounts(ctx, *chAccOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts verify crash")
				log.Ctx(ctx).Fatalf("Error verifying channel accounts: %v", err)
			}
		},
	}
	err := configOpts.Init(verifyCmd)
	if err != nil {
		log.Fatalf("Error initializing verifyCmd: %v", err)
	}

	return verifyCmd
}

func (c *ChannelAccountsCommand) EnsureCommand(chAccOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account used to sponsor existing channel accounts",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &chAccOpts.RootSeed,
			Required:       true,
		},
		{
			Name:        "num-channel-accounts-ensure",
			Usage:       "The desired number of channel accounts to manage",
			OptType:     types.Int,
			ConfigKey:   &chAccOpts.NumChannelAccounts,
			FlagDefault: 1,
			Required:    true,
		},
		{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &chAccOpts.MaxBaseFee,
			FlagDefault: 100 * txnbuild.MinBaseFee,
			Required:    true,
		},
	}

	ensureCmd := &cobra.Command{
		Use: "ensure",
		Short: "Ensure we are managing exactly the number of channel accounts " +
			"equal to some specified count by dynamically increasing or decreasing the number of managed " +
			"channel accounts in storage and onchain",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := c.Service.EnsureChannelAccountsCount(ctx, *chAccOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts ensure crash")
				log.Ctx(ctx).Fatalf("Error ensuring count for channel accounts: %v", err)
			}
		},
	}

	err := configOpts.Init(ensureCmd)
	if err != nil {
		log.Fatalf("Error initializing ensureCmd: %v", err)
	}

	return ensureCmd
}

func (c *ChannelAccountsCommand) DeleteCommand(chAccOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account used to sponsor the channel account specified",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &chAccOpts.RootSeed,
			Required:       true,
		},
		{
			Name:      "channel-account-id",
			Usage:     "The ID of the channel account to delete",
			OptType:   types.String,
			ConfigKey: &chAccOpts.ChannelAccountID,
			Required:  false,
		},
		{
			Name:        "delete-all-accounts",
			Usage:       "Delete all managed channel accoounts in the database and on the network",
			OptType:     types.Bool,
			ConfigKey:   &chAccOpts.DeleteAllAccounts,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &chAccOpts.MaxBaseFee,
			FlagDefault: 100 * txnbuild.MinBaseFee,
			Required:    true,
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a specified channel account from storage and on the network",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := c.Service.DeleteChannelAccount(ctx, *chAccOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts delete crash")
				log.Ctx(ctx).Fatalf("Error deleting channel account: %v", err)
			}
		},
	}

	err := configOpts.Init(deleteCmd)
	if err != nil {
		log.Fatalf("Error initializing deleteCmd: %v", err)
	}

	deleteCmd.MarkFlagsMutuallyExclusive("channel-account-id", "delete-all-accounts")

	return deleteCmd
}

func (c *ChannelAccountsCommand) ViewCommand() *cobra.Command {
	viewCmd := &cobra.Command{
		Use:   "view",
		Short: "View all channel accounts currently managed in the database",
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			err := c.Service.ViewChannelAccounts(ctx)
			if err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts view crash")
				log.Ctx(ctx).Fatalf("Error viewing channel accounts: %v", err)
			}
		},
	}

	return viewCmd
}
