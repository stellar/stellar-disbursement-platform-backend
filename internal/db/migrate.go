package db

import (
	"fmt"
	"net/http"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Migrate(dbURL string, dir migrate.MigrationDirection, count int) (int, error) {
	dbConnectionPool, err := OpenDBConnectionPool(dbURL)
	if err != nil {
		return 0, fmt.Errorf("database URL '%s': %w", utils.TruncateString(dbURL, len(dbURL)/4), err)
	}
	defer dbConnectionPool.Close()

	ms := migrate.MigrationSet{}
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(migrations.FS)}
	return ms.ExecMax(dbConnectionPool.SqlDB(), dbConnectionPool.DriverName(), m, dir, count)
}
