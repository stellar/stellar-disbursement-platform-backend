package dbtest

import (
	"net/http"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/db/dbtest"
	"github.com/stellar/go/support/db/schema"

	adminmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/admin-migrations"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	tssmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/tss-migrations"
)

func OpenWithoutMigrations(t *testing.T) *dbtest.DB {
	db := dbtest.Postgres(t)
	return db
}

func Open(t *testing.T) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	// Admin migrations
	ms := migrate.MigrationSet{TableName: "admin_migrations"}
	migrateDirection := migrate.Up
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(adminmigrations.FS)}
	_, err := ms.ExecMax(conn.DB, "postgres", m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Per-tenant SDP migrations
	ms = migrate.MigrationSet{TableName: "sdp_migrations"}
	m = migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(sdpmigrations.FS)}
	_, err = ms.ExecMax(conn.DB, "postgres", m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Per-tenant Auth migrations
	ms = migrate.MigrationSet{TableName: "auth_migrations"}
	m = migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(authmigrations.FS)}
	_, err = ms.ExecMax(conn.DB, "postgres", m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}

	// TSS migrations
	ms = migrate.MigrationSet{TableName: "tss_migrations"}
	m = migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(tssmigrations.FS)}
	_, err = ms.ExecMax(conn.DB, "postgres", m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func OpenWithAdminMigrationsOnly(t *testing.T) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	migrateDirection := schema.MigrateUp
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(adminmigrations.FS)}
	_, err := schema.Migrate(conn.DB, m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func OpenWithSDPMigrationsOnly(t *testing.T) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	migrateDirection := schema.MigrateUp
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(sdpmigrations.FS)}
	_, err := schema.Migrate(conn.DB, m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func OpenWithAuthMigrationsOnly(t *testing.T) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	migrateDirection := schema.MigrateUp
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(authmigrations.FS)}
	_, err := schema.Migrate(conn.DB, m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func OpenWithTSSMigrationsOnly(t *testing.T) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	migrateDirection := schema.MigrateUp
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(tssmigrations.FS)}
	_, err := schema.Migrate(conn.DB, m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
