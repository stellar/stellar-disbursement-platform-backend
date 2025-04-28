package db

import (
	"context"
	"fmt"
	"io/fs"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
)

func TestMigrate_up_1(t *testing.T) {
	testCases := []struct {
		migrationRouter      migrations.MigrationRouter
		initialMigrationName string
	}{
		{
			migrationRouter:      migrations.SDPMigrationRouter,
			initialMigrationName: "2023-01-20.0-initial.sql",
		},
		{
			migrationRouter:      migrations.AdminMigrationRouter,
			initialMigrationName: "2023-10-16.0.add-tenants-table.sql",
		},
		{
			migrationRouter:      migrations.AuthMigrationRouter,
			initialMigrationName: "2023-02-09.0.add-users-table.sql",
		},
		{
			migrationRouter:      migrations.TSSMigrationRouter,
			initialMigrationName: "2024-01-03.0-add-submitter-transactions-table.sql",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("%s-up-1", tc.migrationRouter.TableName), func(t *testing.T) {
			t.Parallel()
			db := dbtest.OpenWithoutMigrations(t)
			defer db.Close()
			dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			ctx := context.Background()

			n, err := Migrate(db.DSN, migrate.Up, 1, tc.migrationRouter)
			require.NoError(t, err)
			assert.Equal(t, 1, n)

			ids := []string{}
			err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", tc.migrationRouter.TableName))
			require.NoError(t, err)
			wantIDs := []string{tc.initialMigrationName}
			assert.Equal(t, wantIDs, ids)
		})
	}
}

func TestMigrate_up_2_down_1(t *testing.T) {
	testCases := []struct {
		migrationRouter      migrations.MigrationRouter
		initialMigrationName string
	}{
		{
			migrationRouter:      migrations.SDPMigrationRouter,
			initialMigrationName: "2023-01-20.0-initial.sql",
		},
		{
			migrationRouter:      migrations.AdminMigrationRouter,
			initialMigrationName: "2023-10-16.0.add-tenants-table.sql",
		},
		{
			migrationRouter:      migrations.AuthMigrationRouter,
			initialMigrationName: "2023-02-09.0.add-users-table.sql",
		},
		{
			migrationRouter:      migrations.TSSMigrationRouter,
			initialMigrationName: "2024-01-03.0-add-submitter-transactions-table.sql",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("%s-up-2--down-1", tc.migrationRouter.TableName), func(t *testing.T) {
			t.Parallel()
			db := dbtest.OpenWithoutMigrations(t)
			defer db.Close()
			dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			ctx := context.Background()

			n, err := Migrate(db.DSN, migrate.Up, 2, tc.migrationRouter)
			require.NoError(t, err)
			require.Equal(t, 2, n)

			n, err = Migrate(db.DSN, migrate.Down, 1, tc.migrationRouter)
			require.NoError(t, err)
			require.Equal(t, 1, n)

			ids := []string{}
			err = dbConnectionPool.SelectContext(ctx, &ids, fmt.Sprintf("SELECT id FROM %s", tc.migrationRouter.TableName))
			require.NoError(t, err)
			wantIDs := []string{tc.initialMigrationName}
			assert.Equal(t, wantIDs, ids)
		})
	}
}

func TestMigrate_upAndDownAllTheWayTwice(t *testing.T) {
	migrationRouters := []migrations.MigrationRouter{
		migrations.SDPMigrationRouter,
		migrations.AdminMigrationRouter,
		migrations.AuthMigrationRouter,
		migrations.TSSMigrationRouter,
	}

	for _, migrationRouter := range migrationRouters {
		migrationRouter := migrationRouter
		t.Run(fmt.Sprintf("%s-up-and-down-all-the-way-twice", migrationRouter.TableName), func(t *testing.T) {
			t.Parallel()
			db := dbtest.OpenWithoutMigrations(t)
			defer db.Close()
			dbConnectionPool, err := OpenDBConnectionPool(db.DSN)
			require.NoError(t, err)
			defer dbConnectionPool.Close()

			// Get number of files in the migrations directory:
			var count int
			err = fs.WalkDir(migrationRouter.FS, ".", func(path string, d fs.DirEntry, err error) error {
				require.NoError(t, err)
				if !d.IsDir() {
					count++
				}
				return nil
			})
			require.NoError(t, err)

			n, err := Migrate(db.DSN, migrate.Up, count, migrationRouter)
			require.NoError(t, err)
			require.Equal(t, count, n)

			n, err = Migrate(db.DSN, migrate.Down, count, migrationRouter)
			require.NoError(t, err)
			require.Equal(t, count, n)

			n, err = Migrate(db.DSN, migrate.Up, count, migrationRouter)
			require.NoError(t, err)
			require.Equal(t, count, n)

			n, err = Migrate(db.DSN, migrate.Down, count, migrationRouter)
			require.NoError(t, err)
			require.Equal(t, count, n)
		})
	}
}
