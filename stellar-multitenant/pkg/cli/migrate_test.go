package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	stellardbtest "github.com/stellar/go/support/db/dbtest"
	"github.com/stellar/go/support/log"
	dbpkg "github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	adminMigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/admin-migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getMigrationsApplied(t *testing.T, ctx context.Context, db *sql.DB) []string {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT id FROM %s", dbpkg.StellarAdminMigrationsTableName))
	require.NoError(t, err)

	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		err := rows.Scan(&id)
		require.NoError(t, err)

		ids = append(ids, id)
	}

	require.NoError(t, rows.Err())

	return ids
}

func Test_MigrateCmd(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		envVars     map[string]string
		expect      string
		expectError string
		preRunFunc  func(*testing.T, *stellardbtest.DB)
		postRunFunc func(*sql.DB)
	}{
		{
			name:   "test help command",
			args:   []string{"migrate", "--help"},
			expect: "Apply admin migrations to configure the multi-tenant module that manages the tenants.\n\nUsage:\n  mtn migrate [flags]\n  mtn migrate [command]\n\nAvailable Commands:\n  down        Migrates database down [count] migrations\n  up          Migrates database up [count] migrations\n\nFlags:\n  -h, --help   help for migrate\n\nGlobal Flags:\n      --multitenant-db-url string   Postgres DB URL (MULTITENANT_DB_URL) (default \"postgres://postgres:postgres@localhost:5432/sdp_main?sslmode=disable\")\n\nUse \"mtn migrate [command] --help\" for more information about a command.\n",
		},
		{
			name:   "test short help command",
			args:   []string{"migrate", "-h"},
			expect: "Apply admin migrations to configure the multi-tenant module that manages the tenants.\n\nUsage:\n  mtn migrate [flags]\n  mtn migrate [command]\n\nAvailable Commands:\n  down        Migrates database down [count] migrations\n  up          Migrates database up [count] migrations\n\nFlags:\n  -h, --help   help for migrate\n\nGlobal Flags:\n      --multitenant-db-url string   Postgres DB URL (MULTITENANT_DB_URL) (default \"postgres://postgres:postgres@localhost:5432/sdp_main?sslmode=disable\")\n\nUse \"mtn migrate [command] --help\" for more information about a command.\n",
		},
		{
			name:   "test migrate up successfully",
			args:   []string{"--multitenant-db-url", "", "migrate", "up", "1"},
			expect: "Successfully applied 1 migrations up.",
			postRunFunc: func(db *sql.DB) {
				ids := getMigrationsApplied(t, context.Background(), db)
				assert.Equal(t, []string{"2023-10-16.0.add-tenants-table.sql"}, ids)
			},
		},
		{
			name:    "test migrate up successfully when using the MULTITENANT_DB_URL env var",
			args:    []string{"migrate", "up", "1"},
			envVars: map[string]string{"MULTITENANT_DB_URL": ""},
			expect:  "Successfully applied 1 migrations up.",
			postRunFunc: func(db *sql.DB) {
				ids := getMigrationsApplied(t, context.Background(), db)
				assert.Equal(t, []string{"2023-10-16.0.add-tenants-table.sql"}, ids)
			},
		},
		{
			name:        "test apply migrations when no number of migration is specified",
			args:        []string{"--multitenant-db-url", "", "migrate", "up"},
			expect:      "Successfully applied",
			expectError: "",
		},
		{
			name:        "test migrate down usage",
			args:        []string{"migrate", "down"},
			expect:      "Usage:\n  mtn migrate down [count] [flags]\n\nFlags:\n  -h, --help   help for down\n\nGlobal Flags:\n      --multitenant-db-url string   Postgres DB URL (MULTITENANT_DB_URL) (default \"postgres://postgres:postgres@localhost:5432/sdp_main?sslmode=disable\")\n\n",
			expectError: "accepts 1 arg(s), received 0",
		},
		{
			name:   "test migrate down successfully",
			args:   []string{"--multitenant-db-url", "", "migrate", "down", "1"},
			expect: "Successfully applied 1 migrations down.",
			preRunFunc: func(t *testing.T, db *stellardbtest.DB) {
				_, err := dbpkg.Migrate(db.DSN, migrate.Up, 1, adminMigrations.FS, dbpkg.StellarAdminMigrationsTableName)
				require.NoError(t, err)

				conn := db.Open()
				defer conn.Close()

				ids := getMigrationsApplied(t, context.Background(), conn.DB)
				assert.Equal(t, []string{"2023-10-16.0.add-tenants-table.sql"}, ids)
			},
			postRunFunc: func(db *sql.DB) {
				ids := getMigrationsApplied(t, context.Background(), db)
				assert.Equal(t, []string{}, ids)
			},
		},
		{
			name:    "test migrate up successfully when using the MULTITENANT_DB_URL env var",
			args:    []string{"migrate", "down", "1"},
			envVars: map[string]string{"MULTITENANT_DB_URL": ""},
			expect:  "Successfully applied 1 migrations down.",
			preRunFunc: func(t *testing.T, db *stellardbtest.DB) {
				_, err := dbpkg.Migrate(db.DSN, migrate.Up, 1, adminMigrations.FS, dbpkg.StellarAdminMigrationsTableName)
				require.NoError(t, err)

				conn := db.Open()
				defer conn.Close()

				ids := getMigrationsApplied(t, context.Background(), conn.DB)
				assert.Equal(t, []string{"2023-10-16.0.add-tenants-table.sql"}, ids)
			},
			postRunFunc: func(db *sql.DB) {
				ids := getMigrationsApplied(t, context.Background(), db)
				assert.Equal(t, []string{}, ids)
			},
		},
	}

	for _, tc := range testCases {
		db := dbtest.OpenWithoutMigrations(t)

		if len(tc.args) >= 3 && tc.args[0] == "--multitenant-db-url" {
			tc.args[1] = db.DSN
		}

		t.Run(tc.name, func(t *testing.T) {
			if tc.preRunFunc != nil {
				tc.preRunFunc(t, db)
			}

			for key, value := range tc.envVars {
				if key == "MULTITENANT_DB_URL" {
					value = db.DSN
				}
				t.Setenv(key, value)
			}

			buf := new(strings.Builder)
			log.DefaultLogger.SetLevel(log.DebugLevel)
			log.DefaultLogger.SetOutput(buf)

			rootCmd := rootCmd()
			rootCmd.SetOut(buf)
			rootCmd.AddCommand(MigrateCmd(""))
			rootCmd.SetArgs(tc.args)

			err := rootCmd.Execute()
			if tc.expectError != "" {
				assert.EqualError(t, err, tc.expectError)
			} else {
				require.NoError(t, err)
			}

			output := buf.String()
			if tc.expect != "" {
				assert.Contains(t, output, tc.expect)
			}

			if tc.postRunFunc != nil {
				conn := db.Open()
				tc.postRunFunc(conn.DB)
				conn.Close()
			}
		})

		db.Close()
	}
}

func Test_MigrateCmd_databaseFlagName(t *testing.T) {
	globalOptions = globalOptionsType{}

	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	testCmd := &cobra.Command{
		Use: "testcmd",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			require.NoError(t, err)
		},
	}

	testCmd.PersistentFlags().String("db-url", dbt.DSN, "")

	err := viper.BindPFlag("db-url", testCmd.PersistentFlags().Lookup("db-url"))
	require.NoError(t, err)

	err = viper.BindEnv("DB_URL", dbt.DSN)
	require.NoError(t, err)

	testCmd.AddCommand(MigrateCmd("db-url"))
	testCmd.SetArgs([]string{"migrate", "up", "1"})

	buf := new(strings.Builder)
	log.DefaultLogger.SetOutput(buf)
	log.DefaultLogger.SetLevel(log.InfoLevel)
	testCmd.SetOut(buf)

	err = testCmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Successfully applied 1 migrations up.")
}
