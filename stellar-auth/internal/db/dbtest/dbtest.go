package dbtest

import (
	"net/http"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/db/dbtest"
	"github.com/stellar/go/support/db/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db/migrations"
)

func OpenWithoutMigrations(t *testing.T) *dbtest.DB {
	db := dbtest.Postgres(t)
	return db
}

func Open(t *testing.T) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	migrateDirection := schema.MigrateUp
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(migrations.FS)}
	_, err := schema.Migrate(conn.DB, m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
