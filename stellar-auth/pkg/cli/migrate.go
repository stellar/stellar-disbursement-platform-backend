package cli

import (
	"fmt"
	"strconv"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	migrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
)

func MigrateCmd(databaseFlagName string) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply Stellar Auth database migrations",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}

	migrateUp := &cobra.Command{
		Use:   "up [count]",
		Short: "Migrates database up [count] migrations",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			var count int
			if len(args) > 0 {
				var err error
				count, err = strconv.Atoi(args[0])
				if err != nil {
					log.Ctx(ctx).Fatalf("Invalid [count] argument: %s", args[0])
				}
			}

			// TODO: in SDP-1074, use --tenant-id flag to cast the right dbConnectionPool, or delete this file.
			dbURL := globalOptions.databaseURL
			if globalOptions.databaseURL == "" {
				dbURL = viper.GetString(databaseFlagName)
			}

			err := runMigration(dbURL, migrate.Up, count)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error migrating database Up: %s", err.Error())
			}
		},
	}
	migrateCmd.AddCommand(migrateUp)

	migrateDown := &cobra.Command{
		Use:   "down [count]",
		Short: "Migrates database down [count] migrations",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			count, err := strconv.Atoi(args[0])
			if err != nil {
				log.Ctx(ctx).Fatalf("Invalid [count] argument: %s", args[0])
			}

			dbURL := globalOptions.databaseURL
			if globalOptions.databaseURL == "" {
				dbURL = viper.GetString(databaseFlagName)
			}

			err = runMigration(dbURL, migrate.Down, count)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error migrating database Down: %s", err.Error())
			}
		},
	}
	migrateCmd.AddCommand(migrateDown)

	return migrateCmd
}

func runMigration(databaseURL string, dir migrate.MigrationDirection, count int) error {
	numMigrationsRun, err := db.Migrate(databaseURL, dir, count, migrations.FS, db.StellarPerTenantAuthMigrationsTableName)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	if numMigrationsRun == 0 {
		log.Info("No migrations applied.")
	} else {
		log.Infof("Successfully applied %d migrations %s.", numMigrationsRun, migrationDirectionStr(dir))
	}

	return nil
}

func migrationDirectionStr(dir migrate.MigrationDirection) string {
	if dir == migrate.Up {
		return "up"
	}
	return "down"
}
