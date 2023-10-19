package cmd

import (
	"go/types"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

type globalOptionsType struct {
	logLevel          logrus.Level
	sentryDSN         string
	environment       string
	version           string
	gitCommit         string
	databaseURL       string
	baseURL           string
	networkPassphrase string
}

// populateConfigOptions populates the CrastTrackerOptions from the global options.
func (g globalOptionsType) populateCrashTrackerOptions(crashTrackerOptions *crashtracker.CrashTrackerOptions) {
	if crashTrackerOptions.CrashTrackerType == crashtracker.CrashTrackerTypeSentry {
		crashTrackerOptions.SentryDSN = g.sentryDSN
	}
	crashTrackerOptions.Environment = g.environment
	crashTrackerOptions.GitCommit = g.gitCommit
}

// globalOptions is a variable that holds the global CLI options that can be
// applied to any command or subcommand.
var globalOptions globalOptionsType

const dbConfigOptionFlagName = "database-url"

func rootCmd() *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:           "log-level",
			Usage:          `The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC".`,
			OptType:        types.String,
			FlagDefault:    "TRACE",
			ConfigKey:      &globalOptions.logLevel,
			CustomSetValue: cmdUtils.SetConfigOptionLogLevel,
			Required:       true,
		},
		{
			Name:      "sentry-dsn",
			Usage:     "The DSN (client key) of the Sentry project. If not provided, Sentry will not be used.",
			OptType:   types.String,
			ConfigKey: &globalOptions.sentryDSN,
			Required:  false,
		},
		{
			Name:        "environment",
			Usage:       `The environment where the application is running. Example: "development", "staging", "production".`,
			OptType:     types.String,
			FlagDefault: "development",
			ConfigKey:   &globalOptions.environment,
			Required:    true,
		},
		{
			Name:        dbConfigOptionFlagName,
			Usage:       `Postgres DB URL`,
			OptType:     types.String,
			FlagDefault: "postgres://localhost:5432/sdp?sslmode=disable",
			ConfigKey:   &globalOptions.databaseURL,
			Required:    true,
		},
		{
			Name:        "base-url",
			Usage:       "The SDP backend server's base URL.",
			OptType:     types.String,
			ConfigKey:   &globalOptions.baseURL,
			FlagDefault: "http://localhost:8000",
			Required:    true,
		},
		{
			Name:        "network-passphrase",
			Usage:       "The Stellar network passphrase",
			OptType:     types.String,
			ConfigKey:   &globalOptions.networkPassphrase,
			FlagDefault: network.TestNetworkPassphrase,
			Required:    true,
		},
	}

	rootCmd := &cobra.Command{
		Use:     "stellar-disbursement-platform",
		Short:   "Stellar Disbursement Platform",
		Long:    "The Stellar Disbursement Platform (SDP) enables organizations to disburse bulk payments to recipients using Stellar.",
		Version: globalOptions.version,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}
			log.Info("Version: ", globalOptions.version)
			log.Info("GitCommit: ", globalOptions.gitCommit)
		},
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}

	err := configOpts.Init(rootCmd)
	if err != nil {
		log.Fatalf("Error initializing a config option: %s", err.Error())
	}

	return rootCmd
}

// SetupCLI sets up the CLI and returns the root command with the subcommands
// attached.
func SetupCLI(version, gitCommit string) *cobra.Command {
	globalOptions.version = version
	globalOptions.gitCommit = gitCommit
	rootCmd := rootCmd()

	// Add subcommands
	rootCmd.AddCommand((&ServeCommand{}).Command(&ServerService{}, &monitor.MonitorService{}))
	rootCmd.AddCommand((&DatabaseCommand{}).Command())
	rootCmd.AddCommand((&MessageCommand{}).Command(&MessengerService{}))
	rootCmd.AddCommand((&TxSubmitterCommand{}).Command(&TxSubmitterService{}))
	rootCmd.AddCommand((&ChannelAccountsCommand{}).Command())
	rootCmd.AddCommand((&IntegrationTestsCommand{}).Command())
	rootCmd.AddCommand((&AuthCommand{}).Command())

	return rootCmd
}
