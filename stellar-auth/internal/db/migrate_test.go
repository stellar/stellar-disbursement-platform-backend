package db

import (
	"context"
	"fmt"
	"io/fs"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate_upApplyOne(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", StellarAuthMigrationsTableName))
	require.NoError(t, err)
	wantIDs := []string{"2023-02-09.0.add-users-table.sql"}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_downApplyOne(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	n, err := Migrate(db.DSN, migrate.Up, 2)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = Migrate(db.DSN, migrate.Down, 1)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	ids := []string{}
	err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", StellarAuthMigrationsTableName))
	require.NoError(t, err)
	wantIDs := []string{"2023-02-09.0.add-users-table.sql"}
	assert.Equal(t, wantIDs, ids)
}

func TestMigrate_upAndDownAllTheWayTwice(t *testing.T) {
	db := dbtest.OpenWithoutMigrations(t)
	defer db.Close()
	dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	// Get number of files in the migrations directory:
	var count int
	err = fs.WalkDir(migrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			count++
		}
		return nil
	})
	require.NoError(t, err)

	n, err := Migrate(db.DSN, migrate.Up, count)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Up, count)
	require.NoError(t, err)
	require.Equal(t, count, n)

	n, err = Migrate(db.DSN, migrate.Down, count)
	require.NoError(t, err)
	require.Equal(t, count, n)
}
