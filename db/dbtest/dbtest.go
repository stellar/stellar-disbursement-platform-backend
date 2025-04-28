package dbtest

import (
	"net/http"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/db/dbtest"

	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
)

func OpenWithoutMigrations(t *testing.T) *dbtest.DB {
	t.Helper()

	db := dbtest.Postgres(t)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func openWithMigrations(t *testing.T, configs ...migrations.MigrationRouter) *dbtest.DB {
	t.Helper()

	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	for _, config := range configs {
		ms := migrate.MigrationSet{TableName: config.TableName}
		m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(config.FS)}
		_, err := ms.ExecMax(conn.DB, "postgres", m, migrate.Up, 0)
		if err != nil {
			t.Fatal(err)
		}
	}

	return db
}

func Open(t *testing.T) *dbtest.DB {
	t.Helper()

	return openWithMigrations(t,
		migrations.AdminMigrationRouter,
		migrations.SDPMigrationRouter,
		migrations.AuthMigrationRouter,
		migrations.TSSMigrationRouter,
	)
}

func OpenWithAdminMigrationsOnly(t *testing.T) *dbtest.DB {
	t.Helper()

	return openWithMigrations(t, migrations.AdminMigrationRouter)
}

func OpenWithTSSMigrationsOnly(t *testing.T) *dbtest.DB {
	t.Helper()

	return openWithMigrations(t, migrations.TSSMigrationRouter)
}
