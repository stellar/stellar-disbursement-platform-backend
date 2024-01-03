package db

import (
	"context"
	"strconv"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
)

// migrateCmd returns a cobra.Command responsible for running the database migrations.
func (c *DatabaseCommand) migrateCmd(ctx context.Context, executeMigrationsFn func(ctx context.Context, dir migrate.MigrationDirection, count int) error) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:              "migrate",
		Short:            "Schema migration helpers",
		PersistentPreRun: utils.PropagatePersistentPreRun,
		RunE:             utils.CallHelpCommand,
	}

	migrateUpCmd := cobra.Command{
		Use:              "up",
		Short:            "Migrates database up [count] migrations",
		Args:             cobra.MaximumNArgs(1),
		PersistentPreRun: utils.PropagatePersistentPreRun,
		Run: func(cmd *cobra.Command, args []string) {
			var count int
			if len(args) > 0 {
				var err error
				count, err = strconv.Atoi(args[0])
				if err != nil {
					log.Ctx(ctx).Fatalf("Invalid [count] argument: %s", args[0])
				}
			}

			if err := executeMigrationsFn(cmd.Context(), migrate.Up, count); err != nil {
				log.Ctx(ctx).Fatalf("Error executing migrate up: %v", err)
			}
		},
	}

	migrateDownCmd := &cobra.Command{
		Use:              "down [count]",
		Short:            "Migrates database down [count] migrations",
		Args:             cobra.ExactArgs(1),
		PersistentPreRun: utils.PropagatePersistentPreRun,
		Run: func(cmd *cobra.Command, args []string) {
			count, err := strconv.Atoi(args[0])
			if err != nil {
				log.Ctx(ctx).Fatalf("Invalid [count] argument: %s", args[0])
			}

			if err := executeMigrationsFn(cmd.Context(), migrate.Down, count); err != nil {
				log.Ctx(ctx).Fatalf("Error executing migrate down: %v", err)
			}
		},
	}

	migrateCmd.AddCommand(&migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	return migrateCmd
}
