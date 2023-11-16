package db

import (
	"context"
	"embed"
	"fmt"
	"net/http"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

type MigrationTableName string

const (
	StellarMultiTenantMigrationsTableName MigrationTableName = "migrations"
	StellarSDPMigrationsTableName         MigrationTableName = "gorp_migrations"
	StellarAuthMigrationsTableName        MigrationTableName = "auth_migrations"
)

func Migrate(dbURL string, dir migrate.MigrationDirection, count int, migrationFiles embed.FS, tableName MigrationTableName) (int, error) {
	dbConnectionPool, err := OpenDBConnectionPool(dbURL)
	if err != nil {
		return 0, fmt.Errorf("database URL '%s': %w", utils.TruncateString(dbURL, len(dbURL)/4), err)
	}
	defer dbConnectionPool.Close()

	ms := migrate.MigrationSet{
		TableName: string(tableName),
	}

	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(migrationFiles)}
	ctx := context.Background()
	db, err := dbConnectionPool.SqlDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetching sql.DB: %w", err)
	}
	return ms.ExecMax(db, dbConnectionPool.DriverName(), m, dir, count)
}
