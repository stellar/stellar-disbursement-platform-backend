package cmd

import (
	"context"
	"fmt"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

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

// TenantsService interface for tenant operations
type TenantsService interface {
	EnsureDefaultTenant(ctx context.Context, cfg DefaultTenantConfig) error
}

type DefaultTenantConfig struct {
	DefaultTenantOwnerEmail              string
	DefaultTenantOwnerFirstName          string
	DefaultTenantOwnerLastName           string
	DefaultTenantDistributionAccountType string
	DistributionPublicKey                string
	HorizonURL                           string
	NetworkPassphrase                    string
	DatabaseURL                          string
}

// DefaultTenantsService implements TenantsService
type DefaultTenantsService struct{}

// EnsureDefaultTenant creates a default tenant if one doesn't exist and sets it as the default
func (s *DefaultTenantsService) EnsureDefaultTenant(ctx context.Context, cfg DefaultTenantConfig) error {
	// Set default account type if not provided
	distributionAccountType := schema.DistributionAccountStellarDBVault
	if cfg.DefaultTenantDistributionAccountType != "" {
		distributionAccountType = schema.AccountType(cfg.DefaultTenantDistributionAccountType)
	}

	// Validate account type
	switch distributionAccountType {
	case schema.DistributionAccountStellarDBVault, schema.DistributionAccountStellarEnv, schema.DistributionAccountCircleDBVault:
	default:
		return fmt.Errorf("invalid distribution account type: %s", cfg.DefaultTenantDistributionAccountType)
	}

	// Connect to the database
	dbcpOptions := di.DBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL}
	adminDBConnectionPool, err := di.NewAdminDBConnectionPool(ctx, dbcpOptions)
	if err != nil {
		return fmt.Errorf("error getting Admin DB connection pool: %w", err)
	}
	defer func() {
		di.CleanupInstanceByValue(ctx, adminDBConnectionPool)
	}()

	// Setup the Multi-tenant DB connection pool
	mtnDBConnectionPool, err := di.NewMtnDBConnectionPool(ctx, dbcpOptions)
	if err != nil {
		return fmt.Errorf("error getting Multi-tenant DB connection pool: %w", err)
	}
	defer func() {
		di.CleanupInstanceByValue(ctx, mtnDBConnectionPool)
	}()

	// Setup the TSS DB connection pool
	tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, dbcpOptions)
	if err != nil {
		return fmt.Errorf("error getting TSS DB connection pool: %w", err)
	}
	defer func() {
		di.CleanupInstanceByValue(ctx, tssDBConnectionPool)
	}()

	// Create tenant manager
	tenantManager := tenant.NewManager(
		tenant.WithDatabase(adminDBConnectionPool),
	)

	// Check if a default tenant already exists
	defaultTenant, err := tenantManager.GetDefault(ctx)
	if err == nil {
		log.Ctx(ctx).Infof("Default tenant already exists: %s (%s)", defaultTenant.Name, defaultTenant.ID)
		return nil
	} else if err != tenant.ErrTenantDoesNotExist {
		return fmt.Errorf("error checking for default tenant: %w", err)
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
		return fmt.Errorf("error creating distribution account resolver: %w", err)
	}

	// Setup the Submitter Engine
	txSubmitterOpts := di.TxSubmitterEngineOptions{
		SignatureServiceOptions: signing.SignatureServiceOptions{
			DBConnectionPool:            tssDBConnectionPool,
			NetworkPassphrase:           globalOptions.NetworkPassphrase,
			DistributionAccountResolver: distAccountResolver,
		},
		HorizonURL: cfg.HorizonURL,
	}

	submitterEngine, err := di.NewTxSubmitterEngine(ctx, txSubmitterOpts)
	if err != nil {
		return fmt.Errorf("error creating submitter engine: %w", err)
	}

	networkType, err := utils.GetNetworkTypeFromNetworkPassphrase(globalOptions.NetworkPassphrase)
	if err != nil {
		return fmt.Errorf("error getting network type: %w", err)
	}

	// Create provisioning manager
	provisioningManager, err := provisioning.NewManager(provisioning.ManagerOptions{
		DBConnectionPool:           mtnDBConnectionPool,
		TenantManager:              tenantManager,
		SubmitterEngine:            submitterEngine,
		NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
	})
	if err != nil {
		return fmt.Errorf("error creating provisioning manager: %w", err)
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
		return fmt.Errorf("error creating default tenant: %w", err)
	}

	_, err = tenantManager.SetDefault(ctx, adminDBConnectionPool, newTenant.ID)
	if err != nil {
		return fmt.Errorf("error setting tenant as default: %w", err)
	}

	log.Ctx(ctx).Infof("Successfully created and set default tenant: %s (%s)", newTenant.Name, newTenant.ID)
	return nil
}

// Command returns the cobra command for tenant operations
// Command returns the cobra command for tenant operations
func (cmd *TenantsCommand) Command(tenantService TenantsService) *cobra.Command {
	cfg := DefaultTenantConfig{}

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
		cmdUtils.DistributionPublicKey(&cfg.DistributionPublicKey),
		cmdUtils.HorizonURL(&cfg.HorizonURL),
	}

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

			cfg.NetworkPassphrase = globalOptions.NetworkPassphrase
			cfg.DatabaseURL = globalOptions.DatabaseURL
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			if err := tenantService.EnsureDefaultTenant(ctx, cfg); err != nil {
				log.Ctx(ctx).Fatalf("Error ensuring default tenant: %s", err)
			}
		},
	}

	err := configOpts.Init(tenantCmd)
	if err != nil {
		log.Fatalf("Error initializing a config option: %s", err.Error())
	}

	tenantCmd.AddCommand(ensureDefaultCmd)
	return tenantCmd
}
