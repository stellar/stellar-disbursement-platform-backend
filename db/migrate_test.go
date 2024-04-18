package db

import (
	"context"
	"fmt"
	"io/fs"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	adminmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/admin-migrations"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate_upApplyOne_SDP_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 1, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", migrations.SDPMigrationRouter.TableName))
	require.NoError(t, err)
	wantIDs := []string{"2023-01-20.0-initial.sql"}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_downApplyOne_SDP_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 2, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = Migrate(db.DSN, migrate.Down, 1, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", migrations.SDPMigrationRouter.TableName))
	require.NoError(t, err)
	wantIDs := []string{"2023-01-20.0-initial.sql"}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_upAndDownAllTheWayTwice_SDP_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Get number of files in the migrations directory:
	var count int
	err = fs.WalkDir(sdpmigrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			count++
		}
		return nil
	})
	require.NoError(t, err)

	n, err := Migrate(db.DSN, migrate.Up, count, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Up, count, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)
}

func TestMigrate_upApplyOne_Tenant_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 1, migrations.AdminMigrationRouter)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", migrations.AdminMigrationRouter.TableName))
	require.NoError(t, err)
	wantIDs := []string{"2023-10-16.0.add-tenants-table.sql"}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_downApplyTwo_Tenant_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 2, migrations.AdminMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = Migrate(db.DSN, migrate.Down, 2, migrations.AdminMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", migrations.AdminMigrationRouter.TableName))
	require.NoError(t, err)
	wantIDs := []string{}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_upAndDownAllTheWayTwice_Tenant_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Get number of files in the migrations directory:
	var count int
	err = fs.WalkDir(adminmigrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			count++
		}
		return nil
	})
	require.NoError(t, err)

	n, err := Migrate(db.DSN, migrate.Up, count, migrations.AdminMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count, migrations.AdminMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Up, count, migrations.AdminMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count, migrations.AdminMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)
}

func TestMigrate_upApplyOne_Auth_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 1, migrations.AuthMigrationRouter)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", migrations.AuthMigrationRouter.TableName))
	require.NoError(t, err)
	wantIDs := []string{"2023-02-09.0.add-users-table.sql"}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_downApplyOne_Auth_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 2, migrations.AuthMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = Migrate(db.DSN, migrate.Down, 1, migrations.AuthMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", migrations.AuthMigrationRouter.TableName))
	require.NoError(t, err)
	wantIDs := []string{"2023-02-09.0.add-users-table.sql"}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_upAndDownAllTheWayTwice_Auth_migrations(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Get number of files in the migrations directory:
	var count int
	err = fs.WalkDir(authmigrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			count++
		}
		return nil
	})
	require.NoError(t, err)

	n, err := Migrate(db.DSN, migrate.Up, count, migrations.AuthMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count, migrations.AuthMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Up, count, migrations.AuthMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count, migrations.AuthMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, count, n)
}
