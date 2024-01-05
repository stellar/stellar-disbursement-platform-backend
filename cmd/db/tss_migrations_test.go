package db

import (
	"context"
	"io/fs"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	tssmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/tss-migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewTSSDatabaseMigrationManager(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	t.Run("returns a proper dbConnectionPool", func(t *testing.T) {
		manager, err := NewTSSDatabaseMigrationManager(dbt.DSN)
		require.NoError(t, err)

		assert.Equal(t, dbt.DSN, manager.RootDatabaseDSN)
		require.NotNil(t, manager.RootDBConnectionPool)
		defer manager.RootDBConnectionPool.Close()
	})

	t.Run("returns an error if the database DSN is invalid", func(t *testing.T) {
		manager, err := NewTSSDatabaseMigrationManager("invalid-dsn")
		require.Error(t, err)
		assert.Nil(t, manager)
	})
}

func Test_TSSDatabaseMigrationManager_createTSSSchemaIfNeeded(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	manager, err := NewTSSDatabaseMigrationManager(dbt.DSN)
	require.NoError(t, err)
	defer manager.RootDBConnectionPool.Close()

	ctx := context.Background()

	// Checks that the tss schema does not exist.
	query := `
		SELECT COUNT(*) > 0
		FROM information_schema.schemata
		WHERE schema_name = 'tss'
	`
	var exists bool
	err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query)
	require.NoError(t, err)
	assert.False(t, exists)

	// Creates the tss schema.
	err = manager.createTSSSchemaIfNeeded(ctx)
	require.NoError(t, err)

	// Checks that the tss schema exists.
	err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query)
	require.NoError(t, err)
	assert.True(t, exists)

	// Runs the createTSSSchemaIfNeeded function again, it should be a no-op.
	err = manager.createTSSSchemaIfNeeded(ctx)
	require.NoError(t, err)
	err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query)
	require.NoError(t, err)
	assert.True(t, exists)
}

func Test_TSSDatabaseMigrationManager_deleteTSSSchemaIfNeeded(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	manager, err := NewTSSDatabaseMigrationManager(dbt.DSN)
	require.NoError(t, err)
	defer manager.RootDBConnectionPool.Close()

	ctx := context.Background()

	// Creates the tss schema.
	err = manager.createTSSSchemaIfNeeded(ctx)
	require.NoError(t, err)

	// Checks that the tss schema exists.
	query := `
		SELECT COUNT(*) > 0
		FROM information_schema.schemata
		WHERE schema_name = 'tss'
	`
	var exists bool
	err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query)
	require.NoError(t, err)
	assert.True(t, exists)

	// Deletes the tss schema.
	err = manager.deleteTSSSchemaIfNeeded(ctx)
	require.NoError(t, err)

	// Checks that the tss schema does not exist.
	err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query)
	require.NoError(t, err)
	assert.False(t, exists)

	// Runs the deleteTSSSchemaIfNeeded function again, it should be a no-op.
	err = manager.deleteTSSSchemaIfNeeded(ctx)
	require.NoError(t, err)
	err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query)
	require.NoError(t, err)
	assert.False(t, exists)
}

func Test_runTSSMigrations(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Get number of files in the migrations directory:
	var count int
	err = fs.WalkDir(tssmigrations.FS, ".", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			count++
		}
		return nil
	})
	require.NoError(t, err)

	err = runTSSMigrations(ctx, dbt.DSN, migrate.Up, count)
	require.NoError(t, err)

	err = runTSSMigrations(ctx, dbt.DSN, migrate.Down, count)
	require.NoError(t, err)

	err = runTSSMigrations(ctx, dbt.DSN, migrate.Up, count)
	require.NoError(t, err)

	err = runTSSMigrations(ctx, dbt.DSN, migrate.Down, count)
	require.NoError(t, err)
}
