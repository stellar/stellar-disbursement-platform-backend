package cmd

import (
	"context"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
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
		{
			Name:        "horizon-url",
			Usage:       `Horizon URL"`,
			OptType:     types.String,
			ConfigKey:   &svcOpts.HorizonUrl,
			FlagDefault: horizonclient.DefaultTestNetClient.HorizonURL,
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
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}

			// Inject server dependencies
			svcOpts.DatabaseDSN = globalOptions.databaseURL
			svcOpts.NetworkPassphrase = globalOptions.networkPassphrase
			c.Service, err = txSubSvc.NewChannelAccountService(ctx, *svcOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating channel account service: %s", err.Error())
			}

			// Inject crash tracker options dependencies
			globalOptions.populateCrashTrackerOptions(&crashTrackerOptions)

			// Setup default Crash Tracker client
			crashTrackerClient, err := di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating crash tracker client: %s", err.Error())
			}
			c.CrashTrackerClient = crashTrackerClient
		},
	}
	err := configOpts.Init(channelAccountsCmd)
	if err != nil {
		log.Fatalf("Error initializing channelAccountsCmd config option: %s", err.Error())
	}

	createCmd := c.CreateCommand(svcOpts)
	deleteCmd := c.DeleteCommand(svcOpts)
	ensureCmd := c.EnsureCommand(svcOpts)
	verifyCmd := c.VerifyCommand(svcOpts)
	viewCmd := c.ViewCommand()
	channelAccountsCmd.AddCommand(createCmd, deleteCmd, ensureCmd, verifyCmd, viewCmd)

	return channelAccountsCmd
}

func (c *ChannelAccountsCommand) CreateCommand(toolOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account that will be used to sponsor the channel accounts",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &toolOpts.RootSeed,
			Required:       true,
		},
		{
			Name:        "num-channel-accounts-create",
			Usage:       "The desired number of channel accounts to be created",
			OptType:     types.Int,
			ConfigKey:   &toolOpts.NumChannelAccounts,
			FlagDefault: 1,
			Required:    true,
		},
		{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &toolOpts.MaxBaseFee,
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
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			// entrypoint into the main logic for creating channel accounts
			if err := c.Service.CreateChannelAccountsOnChain(ctx, *toolOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts create crash")
				log.Ctx(ctx).Fatalf("Error creating channel accounts: %s", err.Error())
			}
		},
	}
	err := configOpts.Init(createCmd)
	if err != nil {
		log.Fatalf("Error initializing createCmd: %s", err.Error())
	}

	return createCmd
}

func (c *ChannelAccountsCommand) VerifyCommand(toolOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:        "delete-invalid-accounts",
			Usage:       "Delete channel accounts from storage that are verified to be invalid on the network",
			OptType:     types.Bool,
			ConfigKey:   &toolOpts.DeleteInvalidAcccounts,
			FlagDefault: false,
			Required:    false,
		},
	}

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the existence of all channel accounts in the database on the Stellar newtwork",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := c.Service.VerifyChannelAccounts(ctx, *toolOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts verify crash")
				log.Ctx(ctx).Fatalf("Error verifying channel accounts: %s", err.Error())
			}
		},
	}
	err := configOpts.Init(verifyCmd)
	if err != nil {
		log.Fatalf("Error initializing verifyCmd: %s", err.Error())
	}

	return verifyCmd
}

func (c *ChannelAccountsCommand) EnsureCommand(toolOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account used to sponsor existing channel accounts",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &toolOpts.RootSeed,
			Required:       true,
		},
		{
			Name:        "num-channel-accounts-ensure",
			Usage:       "The desired number of channel accounts to manage",
			OptType:     types.Int,
			ConfigKey:   &toolOpts.NumChannelAccounts,
			FlagDefault: 1,
			Required:    true,
		},
		{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &toolOpts.MaxBaseFee,
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
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := c.Service.EnsureChannelAccountsCount(ctx, *toolOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts ensure crash")
				log.Ctx(ctx).Fatalf("Error ensuring count for channel accounts: %s", err.Error())
			}
		},
	}

	err := configOpts.Init(ensureCmd)
	if err != nil {
		log.Fatalf("Error initializing ensureCmd: %s", err.Error())
	}

	return ensureCmd
}

func (c *ChannelAccountsCommand) DeleteCommand(toolOpts *txSubSvc.ChannelAccountServiceOptions) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar account used to sponsor the channel account specified",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &toolOpts.RootSeed,
			Required:       true,
		},
		{
			Name:      "channel-account-id",
			Usage:     "The ID of the channel account to delete",
			OptType:   types.String,
			ConfigKey: &toolOpts.ChannelAccountID,
			Required:  false,
		},
		{
			Name:        "delete-all-accounts",
			Usage:       "Delete all managed channel accoounts in the database and on the network",
			OptType:     types.Bool,
			ConfigKey:   &toolOpts.DeleteAllAccounts,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &toolOpts.MaxBaseFee,
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
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			if err := c.Service.DeleteChannelAccount(ctx, *toolOpts); err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd channel-accounts delete crash")
				log.Ctx(ctx).Fatalf("Error deleting channel account: %s", err.Error())
			}
		},
	}

	err := configOpts.Init(deleteCmd)
	if err != nil {
		log.Fatalf("Error initializing deleteCmd: %s", err.Error())
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
				log.Ctx(ctx).Fatalf("Error viewing channel accounts: %s", err.Error())
			}
		},
	}

	return viewCmd
}
