package db

import (
	"context"
	"fmt"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const DBConfigOptionFlagName = "database-url"

type DatabaseCommand struct {
	adminDBConnectionPool db.DBConnectionPool
}

func (c *DatabaseCommand) Command(globalOptions *utils.GlobalOptionsType) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database related commands",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			utils.PropagatePersistentPreRun(cmd, args)

			adminDBConnectionPool, err := di.NewAdminDBConnectionPool(cmd.Context(), di.DBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL})
			if err != nil {
				log.Ctx(cmd.Context()).Fatalf("getting Admin database connection pool: %v", err)
			}
			c.adminDBConnectionPool = adminDBConnectionPool
		},
		RunE: utils.CallHelpCommand,
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			di.CleanupInstanceByValue(cmd.Context(), c.adminDBConnectionPool)
		},
	}

	// ADD SUBCOMMANDs:
	// The following commands use --all and --tenant-id flags.
	cmd.AddCommand(c.setupForNetworkCmd(globalOptions))         // 'setup-for-network'
	cmd.AddCommand(c.sdpPerTenantMigrationsCmd(cmd.Context()))  // 'sdp migrate up|down'
	cmd.AddCommand(c.authPerTenantMigrationsCmd(cmd.Context())) // 'auth migrate up|down'

	// The following command does NOT use --all and --tenant-id flags.
	cmd.AddCommand(c.adminMigrationsCmd(cmd.Context(), globalOptions)) // 'admin migrate up|down'
	cmd.AddCommand(c.tssMigrationsCmd(cmd.Context(), globalOptions))   // 'tss migrate up|down'

	return cmd
}

// setupForNetworkCmd returns a cobra.Command responsible for setting up the assets and wallets registered in the
// database based on the network passphrase.
func (c *DatabaseCommand) setupForNetworkCmd(globalOptions *utils.GlobalOptionsType) *cobra.Command {
	opts := utils.TenantRoutingOptions{}
	var configOptions config.ConfigOptions = utils.TenantRoutingConfigOptions(&opts)

	cmd := &cobra.Command{
		Use:   "setup-for-network",
		Short: "Set up the assets and wallets registered in the database based on the network passphrase.",
		Long:  "Set up the assets and wallets registered in the database based on the network passphrase. It inserts or updates the entries of these tables according with the configured Network Passphrase.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			utils.PropagatePersistentPreRun(cmd, args)
			configOptions.Require()
			if err := configOptions.SetValues(); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error setting values of config options: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			if err := opts.ValidateFlags(); err != nil {
				log.Ctx(ctx).Fatal(err.Error())
			}

			tenantIDToDNSMap, err := getTenantIDToDSNMapping(ctx, c.adminDBConnectionPool)
			if err != nil {
				log.Ctx(ctx).Fatalf("getting tenants schemas: %s", err.Error())
			}

			if opts.TenantID != "" {
				if dsn, ok := tenantIDToDNSMap[opts.TenantID]; ok {
					tenantIDToDNSMap = map[string]string{opts.TenantID: dsn}
				} else {
					log.Ctx(ctx).Fatalf("tenant ID %s does not exist", opts.TenantID)
				}
			}

			for tenantID, dsn := range tenantIDToDNSMap {
				log.Ctx(ctx).Infof("running for tenant ID %s", tenantID)
				tenantDBConnectionPool, err := db.OpenDBConnectionPool(dsn)
				if err != nil {
					log.Ctx(ctx).Fatalf("error connection to the database: %s", err.Error())
				}
				defer tenantDBConnectionPool.Close()

				networkType, err := sdpUtils.GetNetworkTypeFromNetworkPassphrase(globalOptions.NetworkPassphrase)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting network type: %s", err.Error())
				}

				if err := services.SetupAssetsForProperNetwork(ctx, tenantDBConnectionPool, networkType, services.DefaultAssetsNetworkMap); err != nil {
					log.Ctx(ctx).Fatalf("error upserting assets for proper network: %s", err.Error())
				}

				if err := services.SetupWalletsForProperNetwork(ctx, tenantDBConnectionPool, networkType, services.DefaultWalletsNetworkMap); err != nil {
					log.Ctx(ctx).Fatalf("error upserting wallets for proper network: %s", err.Error())
				}
			}
		},
	}

	if err := configOptions.Init(cmd); err != nil {
		log.Ctx(cmd.Context()).Fatalf("initializing config options: %v", err)
	}

	return cmd
}

// sdpPerTenantMigrationsCmd returns a cobra.Command responsible for running the migrations of the `sdp-migrations`
// folder on the desired tenant(s).
func (c *DatabaseCommand) sdpPerTenantMigrationsCmd(ctx context.Context) *cobra.Command {
	opts := utils.TenantRoutingOptions{}
	var configOptions config.ConfigOptions = utils.TenantRoutingConfigOptions(&opts)

	sdpCmd := &cobra.Command{
		Use:   "sdp",
		Short: "Stellar Disbursement Platform's per-tenant schema migration helpers. Will execute the migrations of the `sdp-migrations` folder on the desired tenant, according with the --all or --tenant-id configs. The migrations are tracked in the table `sdp_migrations`.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			utils.PropagatePersistentPreRun(cmd, args)
			configOptions.Require()
			if err := configOptions.SetValues(); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error setting values of config options: %v", err)
			}
		},
		RunE: utils.CallHelpCommand,
	}

	executeMigrationsFn := func(ctx context.Context, dir migrate.MigrationDirection, count int) error {
		if err := executeMigrationsPerTenant(ctx, c.adminDBConnectionPool, opts, dir, count, migrations.SDPMigrationRouter); err != nil {
			return fmt.Errorf("executing migrations for %s: %w", sdpCmd.Name(), err)
		}
		return nil
	}
	sdpCmd.AddCommand(MigrateCmd(ctx, executeMigrationsFn))

	if err := configOptions.Init(sdpCmd); err != nil {
		log.Ctx(sdpCmd.Context()).Fatalf("initializing config options: %v", err)
	}

	return sdpCmd
}

// authPerTenantMigrationsCmd returns a cobra.Command responsible for running the migrations of the `auth-migrations`
// folder on the desired tenant(s).
func (c *DatabaseCommand) authPerTenantMigrationsCmd(ctx context.Context) *cobra.Command {
	opts := utils.TenantRoutingOptions{}
	var configOptions config.ConfigOptions = utils.TenantRoutingConfigOptions(&opts)

	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication's per-tenant schema migration helpers. Will execute the migrations of the `auth-migrations` folder on the desired tenant, according with the --all or --tenant-id configs. The migrations are tracked in the table `auth_migrations`.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			utils.PropagatePersistentPreRun(cmd, args)
			configOptions.Require()
			if err := configOptions.SetValues(); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error setting values of config options: %v", err)
			}
		},
		RunE: utils.CallHelpCommand,
	}

	executeMigrationsFn := func(ctx context.Context, dir migrate.MigrationDirection, count int) error {
		if err := executeMigrationsPerTenant(ctx, c.adminDBConnectionPool, opts, dir, count, migrations.AuthMigrationRouter); err != nil {
			return fmt.Errorf("executing migrations for %s: %w", authCmd.Name(), err)
		}
		return nil
	}
	authCmd.AddCommand(MigrateCmd(ctx, executeMigrationsFn))

	if err := configOptions.Init(authCmd); err != nil {
		log.Ctx(authCmd.Context()).Fatalf("initializing config options: %v", err)
	}

	return authCmd
}

// adminMigrationsCmd returns a cobra.Command responsible for running the migrations of the `admin-migrations`
// folder, that are used to configure the multi-tenant module that manages the tenants.
func (c *DatabaseCommand) adminMigrationsCmd(ctx context.Context, globalOptions *utils.GlobalOptionsType) *cobra.Command {
	adminCmd := &cobra.Command{
		Use:              "admin",
		Short:            "Admin migrations used to configure the multi-tenant module that manages the tenants. Will execute the migrations of the `admin-migrations` and the migrations are tracked in the table `admin_migrations`.",
		PersistentPreRun: utils.PropagatePersistentPreRun,
		RunE:             utils.CallHelpCommand,
	}

	executeMigrationsFn := func(ctx context.Context, dir migrate.MigrationDirection, count int) error {
		if err := ExecuteMigrations(ctx, globalOptions.DatabaseURL, dir, count, migrations.AdminMigrationRouter); err != nil {
			return fmt.Errorf("executing migrations for %s: %w", adminCmd.Name(), err)
		}
		return nil
	}
	adminCmd.AddCommand(MigrateCmd(ctx, executeMigrationsFn))

	return adminCmd
}

// tssMigrationsCmd returns a cobra.Command responsible for running the migrations of the `tss-migrations` folder, that
// are used to configure the Transaction Submission Service (TSS) module that submits transactions to the Stellar
// network.
func (c *DatabaseCommand) tssMigrationsCmd(ctx context.Context, globalOptions *utils.GlobalOptionsType) *cobra.Command {
	tssCmd := &cobra.Command{
		Use:              "tss",
		Short:            "TSS migrations used to configure the Transaction Submission Service (TSS) module that submits transactions to the Stellar network. Will execute the migrations of the `tss-migrations` and the migrations are tracked in the table `tss_migrations`.",
		PersistentPreRun: utils.PropagatePersistentPreRun,
		RunE:             utils.CallHelpCommand,
	}

	executeMigrationsFn := func(ctx context.Context, dir migrate.MigrationDirection, count int) error {
		dbURL, err := router.GetDNSForTSS(globalOptions.DatabaseURL)
		if err != nil {
			return fmt.Errorf("getting the TSS database DSN: %w", err)
		}

		tssMigrationsManager, err := NewSchemaMigrationManager(c.adminDBConnectionPool, migrations.TSSMigrationRouter, router.TSSSchemaName, dbURL)
		if err != nil {
			return fmt.Errorf("creating TSS database migration manager: %w", err)
		}

		if err = tssMigrationsManager.OrchestrateSchemaMigrations(ctx, dbURL, dir, count); err != nil {
			return fmt.Errorf("running TSS migrations: %w", err)
		}

		return nil
	}
	tssCmd.AddCommand(MigrateCmd(ctx, executeMigrationsFn))

	return tssCmd
}
