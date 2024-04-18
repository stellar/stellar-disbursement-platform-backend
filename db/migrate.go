package db

import (
	"context"
	"fmt"
	"net/http"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

type MigrationTableName string

func Migrate(dbURL string, dir migrate.MigrationDirection, count int, migrationRouter migrations.MigrationRouter) (int, error) {
	dbConnectionPool, err := OpenDBConnectionPool(dbURL)
	if err != nil {
		return 0, fmt.Errorf("database URL '%s': %w", utils.TruncateString(dbURL, len(dbURL)/4), err)
	}
	defer dbConnectionPool.Close()

	ms := migrate.MigrationSet{
		TableName: string(migrationRouter.TableName),
	}

	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(migrationRouter.FS)}
	ctx := context.Background()
	db, err := dbConnectionPool.SqlDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetching sql.DB: %w", err)
	}
	return ms.ExecMax(db, dbConnectionPool.DriverName(), m, dir, count)
}
