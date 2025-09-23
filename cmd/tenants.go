package cmd

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"time"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	dbpkg "github.com/stellar/stellar-disbursement-platform-backend/db"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant/provisioning"
)

type TenantsCommand struct{}

// DefaultTenantConfig holds configuration for default tenant creation
type DefaultTenantConfig struct {
	DefaultTenantOwnerEmail              string
	DefaultTenantOwnerFirstName          string
	DefaultTenantOwnerLastName           string
	DefaultTenantDistributionAccountType string
	DistributionPublicKey                string
}

func (c *DefaultTenantConfig) Validate() error {
	if c.DefaultTenantOwnerEmail == "" {
		return errors.New("missing default-tenant-owner-email")
	}
	if c.DefaultTenantOwnerFirstName == "" {
		return errors.New("missing default-tenant-owner-first-name")
	}
	if c.DefaultTenantOwnerLastName == "" {
		return errors.New("missing default-tenant-owner-last-name")
	}

	acctType := schema.AccountType(c.DefaultTenantDistributionAccountType)
	switch acctType {
	case schema.DistributionAccountStellarDBVault,
		schema.DistributionAccountStellarEnv,
		schema.DistributionAccountCircleDBVault:
		// valid
	default:
		return fmt.Errorf("invalid distribution account type: %s", c.DefaultTenantDistributionAccountType)
	}
	return nil
}

type TenantsService interface {
	EnsureDefaultTenant(ctx context.Context, cfg DefaultTenantConfig, opts cmdUtils.GlobalOptionsType) error
}

type defaultTenantsService struct {
	adminDBConnectionPool dbpkg.DBConnectionPool
	tenantProvisioning    provisioning.TenantProvisioningService
	submitterEngine       engine.SubmitterEngine
	tenantManager         tenant.ManagerInterface
}

func NewDefaultTenantsService(
	dbc dbpkg.DBConnectionPool,
	tenantProvisioning provisioning.TenantProvisioningService,
	submitterEngine engine.SubmitterEngine,
	tenantManager tenant.ManagerInterface,
) TenantsService {
	return &defaultTenantsService{
		adminDBConnectionPool: dbc,
		tenantProvisioning:    tenantProvisioning,
		submitterEngine:       submitterEngine,
		tenantManager:         tenantManager,
	}
}

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
		},
		cmdUtils.DistributionPublicKey(&cfg.DistributionPublicKey),
	}

	configOpts = append(
		configOpts,
		cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)...,
	)

	tenantsRoot := &cobra.Command{
		Use:   "tenants",
		Short: "Tenant related operations",
		Long:  "Manage tenant operations like creating a default tenant",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
		},
	}

	ensureDefault := &cobra.Command{
		Use:   "ensure-default",
		Short: "Ensure a default tenant exists",
		Long:  "Creates a default tenant if none exists and sets it as default.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			configOpts.Require()
			if err := configOpts.SetValues(); err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}

			return cfg.Validate()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// Initialize dependencies
			adminPool, mtnPool, tssPool, err := initDBPools(ctx, globalOptions.DatabaseURL)
			if err != nil {
				return err
			}
			defer di.CleanupInstanceByValue(ctx, adminPool)

			submitter, err := initSubmitter(ctx, txSubmitterOpts, adminPool, mtnPool, tssPool, cfg.DistributionPublicKey)
			if err != nil {
				return err
			}

			provMgr, tenantMgr, err := initProvisioning(adminPool, mtnPool, submitter)
			if err != nil {
				return err
			}

			svc := NewDefaultTenantsService(adminPool, provMgr, submitter, tenantMgr)
			return svc.EnsureDefaultTenant(ctx, cfg, globalOptions)
		},
	}

	if err := configOpts.Init(ensureDefault); err != nil {
		log.Fatalf("Error initializing config: %s", err)
	}

	tenantsRoot.AddCommand(ensureDefault)
	return tenantsRoot
}

func initDBPools(ctx context.Context, dbURL string) (admin, mtn, tss dbpkg.DBConnectionPool, err error) {
	opts := di.DBConnectionPoolOptions{DatabaseURL: dbURL}
	if admin, err = di.NewAdminDBConnectionPool(ctx, opts); err != nil {
		return
	}
	if mtn, err = di.NewMtnDBConnectionPool(ctx, opts); err != nil {
		return
	}
	tss, err = di.NewTSSDBConnectionPool(ctx, opts)
	return
}

func initSubmitter(
	ctx context.Context,
	txOpts di.TxSubmitterEngineOptions,
	adminPool, mtnPool, tssPool dbpkg.DBConnectionPool,
	hostDistPubKey string,
) (engine.SubmitterEngine, error) {
	txOpts.SignatureServiceOptions.DBConnectionPool = tssPool
	txOpts.SignatureServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase

	distAccResolver, err := di.NewDistributionAccountResolver(ctx, signing.DistributionAccountResolverOptions{
		HostDistributionAccountPublicKey: hostDistPubKey,
		AdminDBConnectionPool:            adminPool,
		MTNDBConnectionPool:              mtnPool,
	})
	if err != nil {
		return engine.SubmitterEngine{}, fmt.Errorf("distribution account resolver: %w", err)
	}
	txOpts.SignatureServiceOptions.DistributionAccountResolver = distAccResolver

	submitter, err := di.NewTxSubmitterEngine(ctx, txOpts)
	if err != nil {
		return engine.SubmitterEngine{}, fmt.Errorf("submitter engine: %w", err)
	}
	return submitter, nil
}

func initProvisioning(
	adminPool, mtnPool dbpkg.DBConnectionPool,
	submitter engine.SubmitterEngine,
) (provisioning.TenantProvisioningService, tenant.ManagerInterface, error) {
	tenantMgr := tenant.NewManager(
		tenant.WithDatabase(adminPool),
		tenant.WithSingleTenantMode(true),
	)
	provMgr, err := provisioning.NewManager(provisioning.ManagerOptions{
		DBConnectionPool:           mtnPool,
		TenantManager:              tenantMgr,
		SubmitterEngine:            submitter,
		NativeAssetBootstrapAmount: tenant.MinTenantDistributionAccountAmount,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("provisioning manager: %w", err)
	}
	return provMgr, tenantMgr, nil
}

func (s *defaultTenantsService) EnsureDefaultTenant(
	ctx context.Context,
	cfg DefaultTenantConfig,
	opts cmdUtils.GlobalOptionsType,
) error {
	// Check existing default
	existing, err := s.tenantManager.GetDefault(ctx)
	if err == nil {
		log.Ctx(ctx).Infof("Default tenant exists: %s (%s)", existing.Name, existing.ID)
		return nil
	}
	if !errors.Is(err, tenant.ErrTenantDoesNotExist) {
		return fmt.Errorf("checking default tenant: %w", err)
	}

	log.Ctx(ctx).Info("Provisioning default tenant")
	netType, err := utils.GetNetworkTypeFromNetworkPassphrase(opts.NetworkPassphrase)
	if err != nil {
		return fmt.Errorf("network type: %w", err)
	}

	newTenant, err := s.tenantProvisioning.ProvisionNewTenant(ctx, provisioning.ProvisionTenant{
		Name:                    "default",
		UserFirstName:           cfg.DefaultTenantOwnerFirstName,
		UserLastName:            cfg.DefaultTenantOwnerLastName,
		UserEmail:               cfg.DefaultTenantOwnerEmail,
		OrgName:                 "Default Organization",
		UIBaseURL:               opts.SDPUIBaseURL,
		BaseURL:                 opts.BaseURL,
		NetworkType:             string(netType),
		DistributionAccountType: schema.AccountType(cfg.DefaultTenantDistributionAccountType),
	})
	if err != nil {
		return fmt.Errorf("provision new tenant: %w", err)
	}
	if _, err := s.tenantManager.SetDefault(ctx, s.adminDBConnectionPool, newTenant.ID); err != nil {
		return fmt.Errorf("set default tenant: %w", err)
	}
	log.Ctx(ctx).Infof("Created default tenant: %s (%s)", newTenant.Name, newTenant.ID)
	return nil
}
