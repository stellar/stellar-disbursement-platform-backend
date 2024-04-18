package db

import (
	"context"
	"fmt"
	"io/fs"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
)

var testCases = []struct {
	MigrationRouter migrations.MigrationRouter
	SchemaName      string
	getDatabaseDNS  func(dataSourceName string) (string, error)
}{
	// {
	// 	MigrationRouter: migrations.AdminMigrationRouter,
	// 	SchemaName:      "admin",
	// 	getDatabaseDNS:  router.GetDNSForAdmin,
	// },
	{
		MigrationRouter: migrations.TSSMigrationRouter,
		SchemaName:      "tss",
		getDatabaseDNS:  router.GetDNSForTSS,
	},
}

func Test_NewSchemaMigrationManager(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.SchemaName, func(t *testing.T) {
			dbt := dbtest.Open(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			t.Run("rootDBConnectionPool cannot be nil", func(t *testing.T) {
				manager, err := NewSchemaMigrationManager(nil, migrations.MigrationRouter{}, "", "")
				assert.Nil(t, manager)
				assert.EqualError(t, err, "rootDBConnectionPool cannot be nil")
			})

			t.Run("migrationRouter cannot be empty", func(t *testing.T) {
				manager, err := NewSchemaMigrationManager(dbConnectionPool, migrations.MigrationRouter{}, "", "")
				assert.Nil(t, manager)
				assert.EqualError(t, err, "migrationRouter cannot be empty")
			})

			t.Run("schemaName cannot be empty", func(t *testing.T) {
				manager, err := NewSchemaMigrationManager(dbConnectionPool, migrations.SDPMigrationRouter, "", "")
				assert.Nil(t, manager)
				assert.EqualError(t, err, "schemaName cannot be empty")
			})

			t.Run("schemaDatabaseDNS cannot be empty", func(t *testing.T) {
				manager, err := NewSchemaMigrationManager(dbConnectionPool, migrations.SDPMigrationRouter, tc.SchemaName, "")
				assert.Nil(t, manager)
				assert.EqualError(t, err, "schemaDatabaseDNS cannot be empty")
			})

			t.Run("🎉 successfully constructs the instance", func(t *testing.T) {
				schemaDatabaseDNS, err := tc.getDatabaseDNS(dbt.DSN)
				require.NoError(t, err)

				manager, err := NewSchemaMigrationManager(dbConnectionPool, tc.MigrationRouter, tc.SchemaName, schemaDatabaseDNS)
				require.NoError(t, err)
				wantManager := &SchemaMigrationManager{
					RootDBConnectionPool: dbConnectionPool,
					MigrationRouter:      tc.MigrationRouter,
					SchemaName:           tc.SchemaName,
				}
				assert.Equal(t, wantManager, manager)
			})
		})
	}
}

func Test_SchemaMigrationManager_createSchemaIfNeeded(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.SchemaName, func(t *testing.T) {
			dbt := dbtest.Open(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			ctx := context.Background()

			schemaDatabaseDNS, err := tc.getDatabaseDNS(dbt.DSN)
			require.NoError(t, err)

			manager, err := NewSchemaMigrationManager(dbConnectionPool, tc.MigrationRouter, tc.SchemaName, schemaDatabaseDNS)
			require.NoError(t, err)

			// Checks that the schema does not exist.
			query := `
                SELECT COUNT(*) > 0
                FROM information_schema.schemata
                WHERE schema_name = $1
                `
			var exists bool
			err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query, tc.SchemaName)
			require.NoError(t, err)
			assert.False(t, exists)

			// Creates the schema.
			err = manager.createSchemaIfNeeded(ctx)
			require.NoError(t, err)

			// Checks that the schema exists.
			err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query, tc.SchemaName)
			require.NoError(t, err)
			assert.True(t, exists)

			// Runs the CreateSchemaIfNeeded function again, it should be a no-op.
			err = manager.createSchemaIfNeeded(ctx)
			require.NoError(t, err)
			err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query, tc.SchemaName)
			require.NoError(t, err)
			assert.True(t, exists)
		})
	}
}

func Test_SchemaMigrationManager_deleteSchemaIfNeeded(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.SchemaName, func(t *testing.T) {
			dbt := dbtest.Open(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			ctx := context.Background()

			schemaDatabaseDNS, err := tc.getDatabaseDNS(dbt.DSN)
			require.NoError(t, err)

			manager, err := NewSchemaMigrationManager(dbConnectionPool, tc.MigrationRouter, tc.SchemaName, schemaDatabaseDNS)
			require.NoError(t, err)

			// Creates the schema.
			err = manager.createSchemaIfNeeded(ctx)
			require.NoError(t, err)

			// Checks that the schema exists.
			query := `
                SELECT COUNT(*) > 0
                FROM information_schema.schemata
                WHERE schema_name = $1
			`
			var exists bool
			err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query, tc.SchemaName)
			require.NoError(t, err)
			assert.True(t, exists)

			// Deletes the schema.
			err = manager.deleteSchemaIfNeeded(ctx)
			require.NoError(t, err)

			// Checks that the schema does not exist.
			err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query, tc.SchemaName)
			require.NoError(t, err)
			assert.False(t, exists)

			// Runs the deleteSchemaIfNeeded function again, it should be a no-op.
			err = manager.deleteSchemaIfNeeded(ctx)
			require.NoError(t, err)
			err = manager.RootDBConnectionPool.GetContext(ctx, &exists, query, tc.SchemaName)
			require.NoError(t, err)
			assert.False(t, exists)
		})
	}
}

func Test_SchemaMigrationManager_executeMigrations(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.SchemaName, func(t *testing.T) {
			dbt := dbtest.Open(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			ctx := context.Background()

			schemaDatabaseDNS, err := tc.getDatabaseDNS(dbt.DSN)
			require.NoError(t, err)

			manager, err := NewSchemaMigrationManager(dbConnectionPool, tc.MigrationRouter, tc.SchemaName, schemaDatabaseDNS)
			require.NoError(t, err)

			// Get number of files in the migrations directory:
			var count int
			err = fs.WalkDir(manager.MigrationRouter.FS, ".", func(path string, d fs.DirEntry, err error) error {
				require.NoError(t, err)
				if !d.IsDir() {
					count++
				}
				return nil
			})
			require.NoError(t, err)

			err = manager.executeMigrations(ctx, dbt.DSN, migrate.Up, count)
			require.NoError(t, err)

			// Checks if the amount iof migrations is correct
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tc.MigrationRouter.TableName)
			var numberOfMigrations int
			err = manager.RootDBConnectionPool.GetContext(ctx, &numberOfMigrations, query)
			require.NoError(t, err)
			assert.Equal(t, count, numberOfMigrations)

			err = manager.executeMigrations(ctx, dbt.DSN, migrate.Down, count)
			require.NoError(t, err)

			// Checks if the amount iof migrations is correct
			err = manager.RootDBConnectionPool.GetContext(ctx, &numberOfMigrations, query)
			require.NoError(t, err)
			assert.Equal(t, 0, numberOfMigrations)

			err = manager.executeMigrations(ctx, dbt.DSN, migrate.Up, count)
			require.NoError(t, err)
			err = manager.RootDBConnectionPool.GetContext(ctx, &numberOfMigrations, query)
			require.NoError(t, err)
			assert.Equal(t, count, numberOfMigrations)

			err = manager.executeMigrations(ctx, dbt.DSN, migrate.Down, count)
			require.NoError(t, err)
			err = manager.RootDBConnectionPool.GetContext(ctx, &numberOfMigrations, query)
			require.NoError(t, err)
			assert.Equal(t, 0, numberOfMigrations)
		})
	}
}

func Test_SchemaMigrationManager_OrchestrateSchemaMigrations(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.SchemaName, func(t *testing.T) {
			dbt := dbtest.Open(t)
			defer dbt.Close()
			dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			ctx := context.Background()

			schemaDatabaseDNS, err := tc.getDatabaseDNS(dbt.DSN)
			require.NoError(t, err)

			manager, err := NewSchemaMigrationManager(dbConnectionPool, tc.MigrationRouter, tc.SchemaName, schemaDatabaseDNS)
			require.NoError(t, err)

			// Get number of files in the migrations directory:
			var count int
			err = fs.WalkDir(manager.MigrationRouter.FS, ".", func(path string, d fs.DirEntry, err error) error {
				require.NoError(t, err)
				if !d.IsDir() {
					count++
				}
				return nil
			})
			require.NoError(t, err)

			err = manager.OrchestrateSchemaMigrations(ctx, schemaDatabaseDNS, migrate.Up, count)
			require.NoError(t, err)

			// Checks if the amount iof migrations is correct
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", tc.SchemaName, tc.MigrationRouter.TableName)
			var numberOfMigrations int
			err = manager.RootDBConnectionPool.GetContext(ctx, &numberOfMigrations, query)
			require.NoError(t, err)
			assert.Equal(t, count, numberOfMigrations)
		})
	}
}
