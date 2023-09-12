package cmd

import (
	"fmt"
	"strconv"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/cli"
)

type DatabaseCommand struct{}

func (c *DatabaseCommand) Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database related commands",
		Run: func(cmd *cobra.Command, _ []string) {
			err := cmd.Help()
			if err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Schema migration helpers",
		Run: func(cmd *cobra.Command, _ []string) {
			err := cmd.Help()
			if err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}
	cmd.AddCommand(migrateCmd)

	migrateUp := &cobra.Command{
		Use:   "up",
		Short: "Migrates database up [count]",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var count int
			if len(args) > 0 {
				var err error
				count, err = strconv.Atoi(args[0])
				if err != nil {
					log.Fatalf("Invalid [count] argument: %s", args[0])
				}
			}

			err := c.migrate(migrate.Up, count)
			if err != nil {
				log.Fatalf("Error migrating database Up: %s", err.Error())
			}
		},
	}
	migrateCmd.AddCommand(migrateUp)

	migrateDown := &cobra.Command{
		Use:   "down [count]",
		Short: "Migrates database down [count] migrations",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			count, err := strconv.Atoi(args[0])
			if err != nil {
				log.Fatalf("Invalid [count] argument: %s", args[0])
			}

			err = c.migrate(migrate.Down, count)
			if err != nil {
				log.Fatalf("Error migrating database Down: %s", err.Error())
			}
		},
	}
	migrateCmd.AddCommand(migrateDown)

	setupForNetwork := &cobra.Command{
		Use:   "setup-for-network",
		Short: "Set up the assets and wallets registered in the database based on the network passphrase.",
		Long:  "Set up the assets and wallets registered in the database based on the network passphrase. It inserts or updates the entries of these tables according with the configured Network Passphrase.",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			dbConnectionPool, err := db.OpenDBConnectionPool(globalOptions.databaseURL)
			if err != nil {
				log.Ctx(ctx).Fatalf("error connection to the database: %s", err.Error())
			}
			defer dbConnectionPool.Close()

			networkType, err := utils.GetNetworkTypeFromNetworkPassphrase(globalOptions.networkPassphrase)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting network type: %s", err.Error())
			}

			if err := services.SetupAssetsForProperNetwork(ctx, dbConnectionPool, networkType, services.DefaultAssetsNetworkMap); err != nil {
				log.Ctx(ctx).Fatalf("error upserting assets for proper network: %s", err.Error())
			}

			if err := services.SetupWalletsForProperNetwork(ctx, dbConnectionPool, networkType, services.DefaultWalletsNetworkMap); err != nil {
				log.Ctx(ctx).Fatalf("error upserting wallets for proper network: %s", err.Error())
			}
		},
	}
	cmd.AddCommand(setupForNetwork)

	stellarAuthMigrateCmd := &cobra.Command{
		Use:     "auth",
		Short:   "Stellar Auth schema migration helpers",
		Example: "stellarauth migrate [direction] [count]",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}
	stellarAuthMigrateCmd.AddCommand(cli.MigrateCmd(dbConfigOptionFlagName))

	// Add `auth` as a sub-command to `db`. Usage: db auth migrate up
	cmd.AddCommand(stellarAuthMigrateCmd)

	return cmd
}

func (c *DatabaseCommand) migrate(dir migrate.MigrationDirection, count int) error {
	numMigrationsRun, err := db.Migrate(globalOptions.databaseURL, dir, count)
	if err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}

	if numMigrationsRun == 0 {
		log.Info("No migrations applied.")
	} else {
		log.Infof("Successfully applied %d migrations.", numMigrationsRun)
	}
	return nil
}
