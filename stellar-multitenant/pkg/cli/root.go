package cli

import (
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
)

type globalOptionsType struct {
	version          string
	gitCommit        string
	multitenantDbURL string
}

var globalOptions globalOptionsType

func rootCmd() *cobra.Command {
	configOptions := config.ConfigOptions{
		{
			Name:        "multitenant-db-url",
			Usage:       "Postgres DB URL",
			OptType:     types.String,
			FlagDefault: "postgres://postgres:postgres@localhost:5432/sdp_main?sslmode=disable",
			ConfigKey:   &globalOptions.multitenantDbURL,
			Required:    true,
		},
	}

	cmd := &cobra.Command{
		Use:   "mtn",
		Short: "Stellar Disbursement Platform Multi-tenant Configuration.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			configOptions.Require()
			err := configOptions.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}

			log.Ctx(ctx).Info("Version: ", globalOptions.version)
			log.Ctx(ctx).Info("GitCommit: ", globalOptions.gitCommit)
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}

	if err := configOptions.Init(cmd); err != nil {
		log.Ctx(cmd.Context()).Fatalf("Error initializing a config option: %s", err.Error())
	}

	return cmd
}

func SetupCLI(version, gitCommit string) *cobra.Command {
	globalOptions.version = version
	globalOptions.gitCommit = gitCommit

	cmd := rootCmd()

	cmd.AddCommand(MigrateCmd(""))
	cmd.AddCommand(AddTenantsCmd())
	cmd.AddCommand(ConfigTenantCmd())

	return cmd
}
