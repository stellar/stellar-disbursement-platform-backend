package dbtest

import (
	"net/http"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/db/dbtest"

	adminmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/admin-migrations"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	tssmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/tss-migrations"
)

type migrationsConfig struct {
	tableName string
	fs        http.FileSystem
}

var (
	adminMigrationsConfig = migrationsConfig{tableName: "admin_migrations", fs: http.FS(adminmigrations.FS)}
	sdpMigrationsConfig   = migrationsConfig{tableName: "sdp_migrations", fs: http.FS(sdpmigrations.FS)}
	authMigrationsConfig  = migrationsConfig{tableName: "auth_migrations", fs: http.FS(authmigrations.FS)}
	tssMigrationsConfig   = migrationsConfig{tableName: "tss_migrations", fs: http.FS(tssmigrations.FS)}
)

func OpenWithoutMigrations(t *testing.T) *dbtest.DB {
	db := dbtest.Postgres(t)
	return db
}

func openWithMigrations(t *testing.T, configs ...migrationsConfig) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	for _, config := range configs {
		ms := migrate.MigrationSet{TableName: config.tableName}
		m := migrate.HttpFileSystemMigrationSource{FileSystem: config.fs}
		_, err := ms.ExecMax(conn.DB, "postgres", m, migrate.Up, 0)
		if err != nil {
			t.Fatal(err)
		}
	}

	return db
}

func Open(t *testing.T) *dbtest.DB {
	return openWithMigrations(t,
		adminMigrationsConfig,
		sdpMigrationsConfig,
		authMigrationsConfig,
		tssMigrationsConfig,
	)
}

func OpenWithAdminMigrationsOnly(t *testing.T) *dbtest.DB {
	return openWithMigrations(t, adminMigrationsConfig)
}

func OpenWithTSSMigrationsOnly(t *testing.T) *dbtest.DB {
	return openWithMigrations(t, tssMigrationsConfig)
}
