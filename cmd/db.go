package cmd

import (
	"context"
	"embed"
	"fmt"
	"go/types"
	"strconv"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	tenantcli "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type databaseCommandConfigOptions struct {
	All      bool
	TenantID string
}

type DatabaseCommand struct{}

func (c *DatabaseCommand) Command() *cobra.Command {
	opts := databaseCommandConfigOptions{}
	configOptions := config.ConfigOptions{
		{
			Name:        "all",
			Usage:       "Apply the migrations to all tenants. It's ignored when '--tenant-id' is set.",
			OptType:     types.Bool,
			FlagDefault: false,
			ConfigKey:   &opts.All,
			Required:    false,
		},
		{
			Name:      "tenant-id",
			Usage:     "The tenant ID where the migrations will be applied. When set, the '--all' option will be ignored.",
			OptType:   types.String,
			ConfigKey: &opts.TenantID,
			Required:  false,
		},
	}

	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database related commands",

		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			}
			configOptions.Require()
			if err := configOptions.SetValues(); err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			err := cmd.Help()
			if err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}

	// setup-for-network CMD
	cmd.AddCommand(c.setupForNetwork(cmd.Context(), opts))

	// sdp migrate up|down CMD
	// It will run the migrations from the `sdp-migrations` folder and track migrated files in the `gorp_migrations` table.
	sdpCmd := &cobra.Command{
		Use:   "sdp",
		Short: "Stellar Disbursement Platform's per-tenant schema migration helpers. Will execute the migrations of the `sdp-migrations` folder on the desired tenant , according with the --all or --tenant-id configs. The migrations are tracked in the table `gorp_migrations`.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}
	sdpMigrateCmd := c.migrateCmd()
	sdpMigrateCmd.AddCommand(c.migrateUpCmd(&opts))
	sdpMigrateCmd.AddCommand(c.migrateDownCmd(&opts))
	sdpCmd.AddCommand(sdpMigrateCmd)
	cmd.AddCommand(sdpCmd)

	// auth migrate up|down CMD
	// It will run the migrations from the `auth-migrations` folder and track migrated files in the `auth_migrations` table.
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication's per-tenant schema migration helpers. Will execute the migrations of the `auth-migrations` folder on the desired tenant, according with the --all or --tenant-id configs. The migrations are tracked in the table `auth_migrations`.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}
	authMigrateCmd := c.migrateCmd()
	authMigrateCmd.AddCommand(c.migrateUpCmd(&opts))
	authMigrateCmd.AddCommand(c.migrateDownCmd(&opts))
	authCmd.AddCommand(authMigrateCmd)
	cmd.AddCommand(authCmd)

	// admin migrate up|down CMD
	// It will run the migrations from the `admin-migrations` folder and track migrated files in the `admin_migrations` table.
	adminCmd := &cobra.Command{
		Use:   "admin",
		Short: "Admin migrations used to configure the multi-tenant module that manages the tenants.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}
	adminCmd.AddCommand(tenantcli.MigrateCmd(dbConfigOptionFlagName))
	cmd.AddCommand(adminCmd)

	if err := configOptions.Init(cmd); err != nil {
		log.Fatalf("initializing config options: %v", err)
	}

	return cmd
}

func (c *DatabaseCommand) migrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Schema migration helpers",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			err := cmd.Help()
			if err != nil {
				log.Fatalf("Error calling help command: %s", err.Error())
			}
		},
	}
}

func (c *DatabaseCommand) migrateUpCmd(opts *databaseCommandConfigOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Migrates database up [count] migrations",
		Args:  cobra.MaximumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			var count int
			if len(args) > 0 {
				var err error
				count, err = strconv.Atoi(args[0])
				if err != nil {
					log.Fatalf("Invalid [count] argument: %s", args[0])
				}
			}

			migrationFiles := sdpmigrations.FS
			migrationTableName := db.StellarSDPMigrationsTableName

			migrateCmd := cmd.Parent()
			if migrateCmd.Parent().Name() == "auth" {
				migrationFiles = authmigrations.FS
				migrationTableName = db.StellarAuthMigrationsTableName
			}

			if err := c.executeMigrate(cmd.Context(), opts, migrate.Up, count, migrationFiles, migrationTableName); err != nil {
				log.Fatalf("Error executing migrate up: %v", err)
			}
		},
	}
}

func (c *DatabaseCommand) migrateDownCmd(opts *databaseCommandConfigOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "down [count]",
		Short: "Migrates database down [count] migrations",
		Args:  cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			count, err := strconv.Atoi(args[0])
			if err != nil {
				log.Fatalf("Invalid [count] argument: %s", args[0])
			}

			migrationFiles := sdpmigrations.FS
			migrationTableName := db.StellarSDPMigrationsTableName

			migrateCmd := cmd.Parent()
			if migrateCmd.Parent().Name() == "auth" {
				migrationFiles = authmigrations.FS
				migrationTableName = db.StellarAuthMigrationsTableName
			}

			if err := c.executeMigrate(cmd.Context(), opts, migrate.Down, count, migrationFiles, migrationTableName); err != nil {
				log.Fatalf("Error executing migrate down: %v", err)
			}
		},
	}
}

func (c *DatabaseCommand) setupForNetwork(ctx context.Context, opts databaseCommandConfigOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "setup-for-network",
		Short: "Set up the assets and wallets registered in the database based on the network passphrase.",
		Long:  "Set up the assets and wallets registered in the database based on the network passphrase. It inserts or updates the entries of these tables according with the configured Network Passphrase.",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			if err := c.validateFlags(&opts); err != nil {
				log.Ctx(ctx).Fatal(err.Error())
			}

			tenantsDSNMap, err := c.getTenantsDSN(ctx, globalOptions.databaseURL)
			if err != nil {
				log.Ctx(ctx).Fatalf("getting tenants schemas: %s", err.Error())
			}

			if opts.TenantID != "" {
				if dsn, ok := tenantsDSNMap[opts.TenantID]; ok {
					tenantsDSNMap = map[string]string{opts.TenantID: dsn}
				} else {
					log.Fatalf("tenant ID %s does not exist", opts.TenantID)
				}
			}

			for tenantID, dsn := range tenantsDSNMap {
				log.Infof("running for tenant ID %s", tenantID)
				dbConnectionPool, err := db.OpenDBConnectionPool(dsn)
				if err != nil {
					log.Ctx(ctx).Fatalf("error connection to the database: %s", err.Error())
				}
				defer dbConnectionPool.Close()

				networkType, err := utils.GetNetworkTypeFromNetworkPassphrase(globalOptions.networkPassphrase)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting network type: %s", err.Error())
				}

				if err := services.SetupAssetsForProperNetwork(ctx, dbConnectionPool, networkType, services.DefaultAssetsNetworkMap); err != nil {
					log.Ctx(ctx).Fatalf("error upserting assets for proper network: %s", err.Error())
				}

				if err := services.SetupWalletsForProperNetwork(ctx, dbConnectionPool, networkType, services.DefaultWalletsNetworkMap); err != nil {
					log.Ctx(ctx).Fatalf("error upserting wallets for proper network: %s", err.Error())
				}
			}
		},
	}
}

func (c *DatabaseCommand) executeMigrate(ctx context.Context, opts *databaseCommandConfigOptions, dir migrate.MigrationDirection, count int, migrationFiles embed.FS, tableName db.MigrationTableName) error {
	if err := c.validateFlags(opts); err != nil {
		log.Ctx(ctx).Fatal(err.Error())
	}

	tenantsDSNMap, err := c.getTenantsDSN(ctx, globalOptions.databaseURL)
	if err != nil {
		return fmt.Errorf("getting tenants schemas: %w", err)
	}

	if opts.TenantID != "" {
		if dsn, ok := tenantsDSNMap[opts.TenantID]; ok {
			tenantsDSNMap = map[string]string{opts.TenantID: dsn}
		} else {
			log.Ctx(ctx).Fatalf("tenant ID %s does not exist", opts.TenantID)
		}
	}

	for tenantID, dsn := range tenantsDSNMap {
		log.Ctx(ctx).Infof("Applying migrations on tenant ID %s", tenantID)
		err = c.applyMigrations(dsn, dir, count, migrationFiles, tableName)
		if err != nil {
			log.Ctx(ctx).Fatalf("Error migrating database Up: %s", err.Error())
		}
	}

	return nil
}

func (c *DatabaseCommand) applyMigrations(dbURL string, dir migrate.MigrationDirection, count int, migrationFiles embed.FS, tableName db.MigrationTableName) error {
	numMigrationsRun, err := db.Migrate(dbURL, dir, count, migrationFiles, tableName)
	if err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}

	if numMigrationsRun == 0 {
		log.Info("No migrations applied.")
	} else {
		log.Infof("Successfully applied %d migrations %s.", numMigrationsRun, migrationDirectionStr(dir))
	}
	return nil
}

func migrationDirectionStr(dir migrate.MigrationDirection) string {
	if dir == migrate.Up {
		return "up"
	}
	return "down"
}

func (c *DatabaseCommand) validateFlags(opts *databaseCommandConfigOptions) error {
	if !opts.All && opts.TenantID == "" {
		return fmt.Errorf(
			"invalid config. Please specify --all to run the migrations for all tenants " +
				"or specify --tenant-id to run the migrations to a specific tenant",
		)
	}
	return nil
}

func (c *DatabaseCommand) getTenantsDSN(ctx context.Context, dbURL string) (map[string]string, error) {
	dbConnectionPool, err := db.OpenDBConnectionPool(dbURL)
	if err != nil {
		return nil, fmt.Errorf("opening database connection pool: %w", err)
	}
	defer dbConnectionPool.Close()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	tenants, err := m.GetAllTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting all tenants: %w", err)
	}

	tenantsDSNMap := make(map[string]string, len(tenants))
	for _, tnt := range tenants {
		dsn, err := m.GetDSNForTenant(ctx, tnt.Name)
		if err != nil {
			return nil, fmt.Errorf("getting DSN for tenant %s: %w", tnt.Name, err)
		}
		tenantsDSNMap[tnt.ID] = dsn
	}

	return tenantsDSNMap, nil
}
