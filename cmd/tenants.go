package cmd

import (
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd/db"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant/provisioning"
)

// TenantsCommand for commands related to tenant operations
type TenantsCommand struct{}

type DefaultTenantConfig struct {
	DefaultTenantOwnerEmail              string
	DefaultTenantOwnerFirstName          string
	DefaultTenantOwnerLastName           string
	DefaultTenantDistributionAccountType string
	DistributionPublicKey                string
	HorizonURL                           string
	MaxBaseFee                           int
	DistAccEncryptionPassphrase          string
	ChAccEncryptionPassphrase            string
	DistributionPrivateKey               string
}

// DefaultTenantsService implements TenantsService
type DefaultTenantsService struct{}

// Command returns the cobra command for tenant operations
func (cmd *TenantsCommand) Command() *cobra.Command {
	cfg := DefaultTenantConfig{}
	txSubmitterOpts := di.TxSubmitterEngineOptions{}

	configOpts := config.ConfigOptions{
		{
			Name:      "default-tenant-owner-email",
			Usage:     "Email address for the default tenant owner",
			OptType:   types.String,
			ConfigKey: &cfg.DefaultTenantOwnerEmail,
			Required:  true,
		},
		{
			Name:      "default-tenant-owner-first-name",
			Usage:     "First name for the default tenant owner",
			OptType:   types.String,
			ConfigKey: &cfg.DefaultTenantOwnerFirstName,
			Required:  true,
		},
		{
			Name:      "default-tenant-owner-last-name",
			Usage:     "Last name for the default tenant owner",
			OptType:   types.String,
			ConfigKey: &cfg.DefaultTenantOwnerLastName,
			Required:  true,
		},
		{
			Name:        "default-tenant-distribution-account-type",
			Usage:       "Distribution account type for the default tenant",
			OptType:     types.String,
			ConfigKey:   &cfg.DefaultTenantDistributionAccountType,
			FlagDefault: string(schema.DistributionAccountStellarDBVault),
			Required:    false,
		},
		{
			Name:        db.DBConfigOptionFlagName,
			Usage:       `Postgres DB URL`,
			OptType:     types.String,
			FlagDefault: "postgres://localhost:5432/sdp?sslmode=disable",
			ConfigKey:   &globalOptions.DatabaseURL,
			Required:    true,
		},
		{
			Name:        "base-url",
			Usage:       "The SDP backend server's base URL.",
			OptType:     types.String,
			ConfigKey:   &globalOptions.BaseURL,
			FlagDefault: "http://localhost:8000",
			Required:    true,
		},
		{
			Name:        "sdp-ui-base-url",
			Usage:       "The SDP UI server's base URL.",
			OptType:     types.String,
			ConfigKey:   &globalOptions.SDPUIBaseURL,
			FlagDefault: "http://localhost:3000",
			Required:    true,
		},
		cmdUtils.DistributionPublicKey(&cfg.DistributionPublicKey),
		cmdUtils.NetworkPassphrase(&globalOptions.NetworkPassphrase),
	}

	configOpts = append(
		configOpts,
		cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)...,
	)

	tenantCmd := &cobra.Command{
		Use:   "tenants",
		Short: "Tenant related operations",
		Long:  "Manage tenant operations like creating a default tenant",
	}

	ensureDefaultCmd := &cobra.Command{
		Use:   "ensure-default",
		Short: "Ensure a default tenant exists",
		Long:  "Creates a default tenant if none exists and sets it as the default tenant. Uses environment variables for configuration.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			configOpts.Require()
			if err := configOpts.SetValues(); err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			cfg.MaxBaseFee = txSubmitterOpts.MaxBaseFee
			cfg.HorizonURL = txSubmitterOpts.HorizonURL
			cfg.DistAccEncryptionPassphrase = txSubmitterOpts.SignatureServiceOptions.DistAccEncryptionPassphrase
			cfg.DistributionPrivateKey = txSubmitterOpts.SignatureServiceOptions.DistributionPrivateKey
			cfg.ChAccEncryptionPassphrase = txSubmitterOpts.SignatureServiceOptions.ChAccEncryptionPassphrase
			// Set default account type if not provided
			distributionAccountType := schema.DistributionAccountStellarDBVault
			if cfg.DefaultTenantDistributionAccountType != "" {
				distributionAccountType = schema.AccountType(cfg.DefaultTenantDistributionAccountType)
			}

			// Validate account type
			switch distributionAccountType {
			case schema.DistributionAccountStellarDBVault, schema.DistributionAccountStellarEnv, schema.DistributionAccountCircleDBVault:
			default:
				log.Ctx(ctx).Fatalf("invalid distribution account type: %s", cfg.DefaultTenantDistributionAccountType)
				return
			}

			// Connect to the database
			dbcpOptions := di.DBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL}
			adminDBConnectionPool, err := di.NewAdminDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting Admin DB connection pool: %s", err)
				return
			}
			defer func() {
				di.CleanupInstanceByValue(ctx, adminDBConnectionPool)
			}()

			// Setup the Multi-tenant DB connection pool
			mtnDBConnectionPool, err := di.NewMtnDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting Multi-tenant DB connection pool: %s", err)
				return
			}

			// Setup the TSS DB connection pool
			tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting TSS DB connection pool: %s", err)
				return
			}

			// Create tenant manager
			tenantManager := tenant.NewManager(
				tenant.WithDatabase(adminDBConnectionPool),
			)

			// Check if a default tenant already exists
			defaultTenant, err := tenantManager.GetDefault(ctx)
			if err == nil {
				log.Ctx(ctx).Infof("Default tenant already exists: %s (%s)", defaultTenant.Name, defaultTenant.ID)
				return
			} else if err != tenant.ErrTenantDoesNotExist {
				log.Ctx(ctx).Fatalf("error checking for default tenant: %s", err)
				return
			}

			log.Ctx(ctx).Info("No default tenant found, creating...")

			// Setup Distribution Account Resolver
			distAccResolverOpts := signing.DistributionAccountResolverOptions{
				HostDistributionAccountPublicKey: cfg.DistributionPublicKey,
				AdminDBConnectionPool:            adminDBConnectionPool,
				MTNDBConnectionPool:              mtnDBConnectionPool,
			}
			distAccountResolver, err := di.NewDistributionAccountResolver(ctx, distAccResolverOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating distribution account resolver: %s", err)
				return
			}

			// Setup the Submitter Engine
			txSubmitterOpts.SignatureServiceOptions.DBConnectionPool = tssDBConnectionPool
			txSubmitterOpts.SignatureServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
			txSubmitterOpts.SignatureServiceOptions.DistributionAccountResolver = distAccountResolver

			submitterEngine, err := di.NewTxSubmitterEngine(ctx, txSubmitterOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating submitter engine: %s", err)
				return
			}

			networkType, err := utils.GetNetworkTypeFromNetworkPassphrase(globalOptions.NetworkPassphrase)
			if err != nil {
				log.Ctx(ctx).Fatalf("error getting network type: %s", err)
				return
			}

			// Create provisioning manager
			provisioningManager, err := provisioning.NewManager(provisioning.ManagerOptions{
				DBConnectionPool:           mtnDBConnectionPool,
				TenantManager:              tenantManager,
				SubmitterEngine:            submitterEngine,
				NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
			})
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating provisioning manager: %s", err)
				return
			}

			// Create the tenant
			newTenant, err := provisioningManager.ProvisionNewTenant(ctx, provisioning.ProvisionTenant{
				Name:                    "default",
				UserFirstName:           cfg.DefaultTenantOwnerFirstName,
				UserLastName:            cfg.DefaultTenantOwnerLastName,
				UserEmail:               cfg.DefaultTenantOwnerEmail,
				OrgName:                 "Default Organization",
				UiBaseURL:               globalOptions.SDPUIBaseURL,
				BaseURL:                 globalOptions.BaseURL,
				NetworkType:             string(networkType),
				DistributionAccountType: distributionAccountType,
			})
			if err != nil {
				log.Ctx(ctx).Fatalf("error creating default tenant: %s", err)
				return
			}

			_, err = tenantManager.SetDefault(ctx, adminDBConnectionPool, newTenant.ID)
			if err != nil {
				log.Ctx(ctx).Fatalf("error setting tenant as default: %s", err)
				return
			}

			log.Ctx(ctx).Infof("Successfully created and set default tenant: %s (%s)", newTenant.Name, newTenant.ID)
		},
	}

	if err := configOpts.Init(ensureDefaultCmd); err != nil {
		log.Fatalf("Error initializing a config option: %s", err.Error())
	}

	tenantCmd.AddCommand(ensureDefaultCmd)
	return tenantCmd
}
