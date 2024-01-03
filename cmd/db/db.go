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
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	tenantcli "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli"
)

const DBConfigOptionFlagName = "database-url"

type DatabaseCommand struct{}

func (c *DatabaseCommand) Command(globalOptions *utils.GlobalOptionsType) *cobra.Command {
	cmd := &cobra.Command{
		Use:              "db",
		Short:            "Database related commands",
		PersistentPreRun: utils.PropagatePersistentPreRun,
		RunE:             utils.CallHelpCommand,
	}

	// ADD COMMANDs:
	// The following commands use --all and --tenant-id flags.
	cmd.AddCommand(c.setupForNetworkCmd(cmd.Context(), globalOptions))         // 'setup-for-network'
	cmd.AddCommand(c.sdpPerTenantMigrationsCmd(cmd.Context(), globalOptions))  // 'sdp migrate up|down'
	cmd.AddCommand(c.authPerTenantMigrationsCmd(cmd.Context(), globalOptions)) // 'auth migrate up|down'

	// The following command does NOT use --all and --tenant-id flags.
	cmd.AddCommand(c.adminMigrationsCmd(cmd.Context())) // 'admin migrate up|down'

	return cmd
}

// setupForNetworkCmd returns a cobra.Command responsible for setting up the assets and wallets registered in the
// database based on the network passphrase.
func (c *DatabaseCommand) setupForNetworkCmd(ctx context.Context, globalOptions *utils.GlobalOptionsType) *cobra.Command {
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

			tenantIDToDNSMap, err := getTenantIDToDSNMapping(ctx, globalOptions.DatabaseURL)
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
				dbConnectionPool, err := db.OpenDBConnectionPool(dsn)
				if err != nil {
					log.Ctx(ctx).Fatalf("error connection to the database: %s", err.Error())
				}
				defer dbConnectionPool.Close()

				networkType, err := sdpUtils.GetNetworkTypeFromNetworkPassphrase(globalOptions.NetworkPassphrase)
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

	if err := configOptions.Init(cmd); err != nil {
		log.Ctx(cmd.Context()).Fatalf("initializing config options: %v", err)
	}

	return cmd
}

// sdpPerTenantMigrationsCmd returns a cobra.Command responsible for running the migrations of the `sdp-migrations`
// folder on the desired tenant(s).
func (c *DatabaseCommand) sdpPerTenantMigrationsCmd(ctx context.Context, globalOptions *utils.GlobalOptionsType) *cobra.Command {
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
		if err := executeMigrationsPerTenant(ctx, globalOptions.DatabaseURL, opts, dir, count, sdpmigrations.FS, db.StellarPerTenantSDPMigrationsTableName); err != nil {
			return fmt.Errorf("executing migrations for %s: %w", sdpCmd.Name(), err)
		}
		return nil
	}
	sdpCmd.AddCommand(c.migrateCmd(ctx, executeMigrationsFn))

	if err := configOptions.Init(sdpCmd); err != nil {
		log.Ctx(sdpCmd.Context()).Fatalf("initializing config options: %v", err)
	}

	return sdpCmd
}

// authPerTenantMigrationsCmd returns a cobra.Command responsible for running the migrations of the `auth-migrations`
// folder on the desired tenant(s).
func (c *DatabaseCommand) authPerTenantMigrationsCmd(ctx context.Context, globalOptions *utils.GlobalOptionsType) *cobra.Command {
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
		if err := executeMigrationsPerTenant(ctx, globalOptions.DatabaseURL, opts, dir, count, authmigrations.FS, db.StellarPerTenantAuthMigrationsTableName); err != nil {
			return fmt.Errorf("executing migrations for %s: %w", authCmd.Name(), err)
		}
		return nil
	}
	authCmd.AddCommand(c.migrateCmd(ctx, executeMigrationsFn))

	if err := configOptions.Init(authCmd); err != nil {
		log.Ctx(authCmd.Context()).Fatalf("initializing config options: %v", err)
	}

	return authCmd
}

// adminMigrationsCmd returns a cobra.Command responsible for running the migrations of the `admin-migrations`
// folder, that are used to configure the multi-tenant module that manages the tenants.
func (c *DatabaseCommand) adminMigrationsCmd(ctx context.Context) *cobra.Command {
	adminCmd := &cobra.Command{
		Use:              "admin",
		Short:            "Admin migrations used to configure the multi-tenant module that manages the tenants. Will execute the migrations of the `admin-migrations` and the migrations are tracked in the table `admin_migrations`.",
		PersistentPreRun: utils.PropagatePersistentPreRun,
		RunE:             utils.CallHelpCommand,
	}
	adminCmd.AddCommand(tenantcli.MigrateCmd(DBConfigOptionFlagName))
	return adminCmd
}
