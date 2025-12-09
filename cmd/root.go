package cmd

import (
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/support/config"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd/db"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

// globalOptions is a variable that holds the global CLI options that can be
// applied to any command or subcommand.
var globalOptions cmdUtils.GlobalOptionsType

func rootCmd() *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "log-level",
			Usage:          `The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC".`,
			OptType:        types.String,
			FlagDefault:    "TRACE",
			ConfigKey:      &globalOptions.LogLevel,
			CustomSetValue: cmdUtils.SetConfigOptionLogLevel,
			Required:       true,
		},
		{
			Name:      "sentry-dsn",
			Usage:     "The DSN (client key) of the Sentry project. If not provided, Sentry will not be used.",
			OptType:   types.String,
			ConfigKey: &globalOptions.SentryDSN,
			Required:  false,
		},
		{
			Name:        "environment",
			Usage:       `The environment where the application is running. Example: "development", "staging", "production".`,
			OptType:     types.String,
			FlagDefault: "development",
			ConfigKey:   &globalOptions.Environment,
			Required:    true,
		},
		{
			Name:        db.DBConfigOptionFlagName,
			Usage:       `Postgres DB URL`,
			OptType:     types.String,
			FlagDefault: "postgres://localhost:5432/sdp?sslmode=disable",
			ConfigKey:   &globalOptions.DatabaseURL,
			Required:    true,
		},
		{
			Name:        "base-url",
			Usage:       "The SDP backend server's base URL.",
			OptType:     types.String,
			ConfigKey:   &globalOptions.BaseURL,
			FlagDefault: "http://localhost:8000",
			Required:    true,
		},
		{
			Name:        "sdp-ui-base-url",
			Usage:       "The SDP UI server's base URL.",
			OptType:     types.String,
			ConfigKey:   &globalOptions.SDPUIBaseURL,
			FlagDefault: "http://localhost:3000",
			Required:    true,
		},
		// env-file flag is already handled in main.go, but it needs to be also defined here because Cobra doesn't allow unknown flags.
		{
			Name:      "env-file",
			Usage:     "Path to environment file to load (e.g., \"dev/.env.https-testnet\"). Supports absolute and relative paths. Defaults to \".env\" if not specified.",
			OptType:   types.String,
			ConfigKey: &globalOptions.EnvFile,
			Required:  false,
		},
		cmdUtils.NetworkPassphrase(&globalOptions.NetworkPassphrase),
	}

	rootCmd := &cobra.Command{
		Use:     "stellar-disbursement-platform",
		Short:   "Stellar Disbursement Platform",
		Long:    "The Stellar Disbursement Platform (SDP) enables organizations to disburse bulk payments to recipients using Stellar.",
		Version: globalOptions.Version,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}

	err := configOpts.Init(rootCmd)
	if err != nil {
		log.Ctx(rootCmd.Context()).Fatalf("Error initializing a config option: %s", err.Error())
	}

	return rootCmd
}

// SetupCLI sets up the CLI and returns the root command with the subcommands
// attached.
func SetupCLI(version, gitCommit string) *cobra.Command {
	globalOptions.Version = version
	globalOptions.GitCommit = gitCommit
	rootCmd := rootCmd()

	// Add subcommands
	rootCmd.AddCommand((&ServeCommand{}).Command(&ServerService{}, &monitor.MonitorService{}))
	rootCmd.AddCommand((&db.DatabaseCommand{}).Command(&globalOptions))
	rootCmd.AddCommand((&MessageCommand{}).Command(&MessengerService{}))
	rootCmd.AddCommand((&TxSubmitterCommand{}).Command(&TxSubmitterService{}))
	rootCmd.AddCommand((&ChannelAccountsCommand{}).Command(&ChAccCmdService{}))
	rootCmd.AddCommand((&DistributionAccountCommand{}).Command(&DistAccCmdService{}))
	rootCmd.AddCommand((&IntegrationTestsCommand{}).Command())
	rootCmd.AddCommand((&AuthCommand{}).Command())
	rootCmd.AddCommand((&TenantsCommand{}).Command())

	return rootCmd
}
