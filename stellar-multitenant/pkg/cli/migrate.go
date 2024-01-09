package cli

import (
	"context"

	cmdDB "github.com/stellar/stellar-disbursement-platform-backend/cmd/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	adminMigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/admin-migrations"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func MigrateCmd(databaseFlagName string) *cobra.Command {
	executeMigrationsFn := func(ctx context.Context, dir migrate.MigrationDirection, count int) error {
		dbURL := globalOptions.multitenantDbURL
		if globalOptions.multitenantDbURL == "" {
			dbURL = viper.GetString(databaseFlagName)
		}

		return cmdDB.ExecuteMigrations(context.Background(), dbURL, dir, count, adminMigrations.FS, db.StellarAdminMigrationsTableName)
	}

	return cmdDB.MigrateCmd(context.Background(), executeMigrationsFn)
}
