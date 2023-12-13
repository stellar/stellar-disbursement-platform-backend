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
)

func OpenWithoutMigrations(t *testing.T) *dbtest.DB {
	db := dbtest.Postgres(t)
	return db
}

func Open(t *testing.T) *dbtest.DB {
	db := OpenWithoutMigrations(t)

	conn := db.Open()
	defer conn.Close()

	// Tenant migrations
	ms := migrate.MigrationSet{TableName: "migrations"}
	migrateDirection := migrate.Up
	m := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(adminmigrations.FS)}
	_, err := ms.ExecMax(conn.DB, "postgres", m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}

	// SDP migrations
	ms = migrate.MigrationSet{}
	m = migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(sdpmigrations.FS)}
	_, err = ms.ExecMax(conn.DB, "postgres", m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Auth migrations
	ms = migrate.MigrationSet{TableName: "auth_migrations"}
	m = migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(authmigrations.FS)}
	_, err = ms.ExecMax(conn.DB, "postgres", m, migrateDirection, 0)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func OpenWithTenantMigrationsOnly(t *testing.T) *dbtest.DB {
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
