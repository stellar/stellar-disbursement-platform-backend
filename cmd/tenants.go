package cmd

import (
	"context"
	"fmt"
	"go/types"
	"time"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)


//go:generate mockery --name=TenantsCmdServiceInterface --case=underscore --structname=MockTenantsCmdServiceInterface --inpackage --filename=tenants_cmd_service_mock.go
type TenantsCmdServiceInterface interface {
	EnsureDefaultTenant(ctx context.Context, tenantService TenantService) error
}

// TenantsCmdService is a lightweight implementation that forwards to the actual service
type TenantsCmdService struct{}

// EnsureDefaultTenant forwards to the actual service implementation
func (t *TenantsCmdService) EnsureDefaultTenant(ctx context.Context, tenantService TenantService) error {
	return tenantService.ensureDefaultTenant(ctx)
}

// Ensure TenantsCmdService implements TenantsCmdServiceInterface
var _ TenantsCmdServiceInterface = (*TenantsCmdService)(nil)

// TenantService is the actual service implementation with all dependencies
type TenantService struct {
	tenantManager           tenant.ManagerInterface
	dbConnectionPool        db.DBConnectionPool
	ownerEmail              string
	ownerFirstName          string
	ownerLastName           string
	distributionAccountType schema.AccountType
}

// ensureDefaultTenant ensures a default tenant exists
func (s *TenantService) ensureDefaultTenant(ctx context.Context) error {
	defaultTenant, err := s.tenantManager.GetDefault(ctx)
	if err == nil && defaultTenant != nil {
		log.Ctx(ctx).Infof("Default tenant already exists: %s (ID: %s)", defaultTenant.Name, defaultTenant.ID)
		return nil
	}

	if err != nil && err != tenant.ErrTenantDoesNotExist && err != tenant.ErrTooManyDefaultTenants {
		return fmt.Errorf("error checking for default tenant: %w", err)
	}

	if err == tenant.ErrTooManyDefaultTenants {
		return fmt.Errorf("multiple default tenants found; please resolve manually")
	}

	// Check if a tenant named "default" already exists
	existingTenant, err := s.tenantManager.GetTenantByName(ctx, "default")
	if err == nil && existingTenant != nil {
		log.Ctx(ctx).Infof("Tenant named 'default' exists but is not set as default, setting as default...")

		// Set it as the default tenant
		defaultTenant, err = s.tenantManager.SetDefault(ctx, s.dbConnectionPool, existingTenant.ID)
		if err != nil {
			return fmt.Errorf("error setting existing tenant as default: %w", err)
		}

		log.Ctx(ctx).Infof("Existing tenant set as default: %s (ID: %s)", defaultTenant.Name, defaultTenant.ID)
		return nil
	}

	// Create the default tenant
	log.Ctx(ctx).Info("Creating default tenant...")
	defaultTenant, err = s.tenantManager.AddTenant(ctx, "default")
	if err != nil {
		return fmt.Errorf("error creating default tenant: %w", err)
	}

	// Create tenant schema
	log.Ctx(ctx).Info("Creating tenant schema...")
	err = s.tenantManager.CreateTenantSchema(ctx, defaultTenant.Name)
	if err != nil {
		return fmt.Errorf("error creating tenant schema: %w", err)
	}

	// Apply migrations for tenant
	log.Ctx(ctx).Info("Applying migrations for tenant...")
	err = s.applyTenantMigrations(ctx, defaultTenant)
	if err != nil {
		return fmt.Errorf("error applying tenant migrations: %w", err)
	}

	// Create the owner user
	log.Ctx(ctx).Info("Creating owner user...")
	err = s.createTenantOwner(ctx, defaultTenant)
	if err != nil {
		return fmt.Errorf("error creating tenant owner: %w", err)
	}

	// Update the tenant status to TENANT_PROVISIONED
	status := tenant.ProvisionedTenantStatus
	tuOpts := &tenant.TenantUpdate{
		ID:                      defaultTenant.ID,
		Status:                  &status,
		DistributionAccountType: s.distributionAccountType,
	}

	_, err = s.tenantManager.UpdateTenantConfig(ctx, tuOpts)
	if err != nil {
		return fmt.Errorf("error updating tenant status: %w", err)
	}

	// Set it as the default tenant
	log.Ctx(ctx).Info("Setting tenant as default...")
	defaultTenant, err = s.tenantManager.SetDefault(ctx, s.dbConnectionPool, defaultTenant.ID)
	if err != nil {
		return fmt.Errorf("error setting tenant as default: %w", err)
	}

	log.Ctx(ctx).Infof("Default tenant created and set: %s (ID: %s)", defaultTenant.Name, defaultTenant.ID)
	return nil
}

// applyTenantMigrations applies the necessary migrations for a tenant
func (s *TenantService) applyTenantMigrations(ctx context.Context, t *tenant.Tenant) error {
	tenantDSN, err := s.tenantManager.GetDSNForTenant(ctx, t.Name)
	if err != nil {
		return fmt.Errorf("getting DSN for tenant: %w", err)
	}

	// Apply SDP migrations
	_, err = db.Migrate(tenantDSN, migrate.Up, 0, migrations.SDPMigrationRouter)
	if err != nil {
		return fmt.Errorf("applying SDP migrations: %w", err)
	}

	// Apply Auth migrations
	_, err = db.Migrate(tenantDSN, migrate.Up, 0, migrations.AuthMigrationRouter)
	if err != nil {
		return fmt.Errorf("applying Auth migrations: %w", err)
	}

	return nil
}

// createTenantOwner creates an owner user for the tenant
func (s *TenantService) createTenantOwner(ctx context.Context, t *tenant.Tenant) error {
	// Save tenant in context
	ctx = tenant.SaveTenantInContext(ctx, t)

	// Get database connection for tenant
	tenantDSN, err := s.tenantManager.GetDSNForTenant(ctx, t.Name)
	if err != nil {
		return fmt.Errorf("getting DSN for tenant: %w", err)
	}

	// Create tenant database connection pool
	tenantDBConnectionPool, err := db.OpenDBConnectionPool(tenantDSN)
	if err != nil {
		return fmt.Errorf("opening tenant database connection: %w", err)
	}
	defer tenantDBConnectionPool.Close()

	// Create auth manager
	authManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(tenantDBConnectionPool, auth.NewDefaultPasswordEncrypter(), time.Hour),
	)

	// Create user
	user := &auth.User{
		FirstName: s.ownerFirstName,
		LastName:  s.ownerLastName,
		Email:     s.ownerEmail,
		IsOwner:   true,
		Roles:     []string{"owner"},
	}

	// Create user with empty password, which will generate a random one
	_, err = authManager.CreateUser(ctx, user, "")
	if err != nil {
		return fmt.Errorf("creating tenant owner: %w", err)
	}

	log.Ctx(ctx).Infof("Created tenant owner user with email %s. A random password has been generated.", s.ownerEmail)
	log.Ctx(ctx).Info("Please use the forgot password flow to set a new password for this user.")

	return nil
}

// TenantsCommand defines the tenant command
type TenantsCommand struct{}

// Command returns the cobra.Command for tenant management
func (c *TenantsCommand) Command(service TenantsCmdServiceInterface) *cobra.Command {
	var ownerEmail, ownerFirstName, ownerLastName, distributionAccountType string

	configOpts := config.ConfigOptions{
		{
			Name:      "default-tenant-owner-email",
			Usage:     "The email for the default tenant owner",
			OptType:   types.String,
			ConfigKey: &ownerEmail,
			EnvVar:    "DEFAULT_TENANT_OWNER_EMAIL",
			Required:  true,
		},
		{
			Name:      "default-tenant-owner-first-name",
			Usage:     "The first name for the default tenant owner",
			OptType:   types.String,
			ConfigKey: &ownerFirstName,
			EnvVar:    "DEFAULT_TENANT_OWNER_FIRST_NAME",
			Required:  true,
		},
		{
			Name:      "default-tenant-owner-last-name",
			Usage:     "The last name for the default tenant owner",
			OptType:   types.String,
			ConfigKey: &ownerLastName,
			EnvVar:    "DEFAULT_TENANT_OWNER_LAST_NAME",
			Required:  true,
		},
		{
			Name:        "default-tenant-distribution-account-type",
			Usage:       "The distribution account type for the default tenant",
			OptType:     types.String,
			ConfigKey:   &distributionAccountType,
			EnvVar:      "DEFAULT_TENANT_DISTRIBUTION_ACCOUNT_TYPE",
			FlagDefault: string(schema.DistributionAccountStellarDBVault),
			Required:    false,
		},
	}

	cmd := &cobra.Command{
		Use:   "tenants",
		Short: "Commands for managing tenants",
	}

	// Subcommand for ensuring a default tenant
	ensureDefaultCmd := &cobra.Command{
		Use:   "ensure-default",
		Short: "Ensure a default tenant exists",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting config values: %s", err.Error())
			}

			// Set up database connection
			dbConnectionPool, err := db.OpenDBConnectionPool(globalOptions.DatabaseURL)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error opening database connection: %s", err.Error())
			}
			defer dbConnectionPool.Close()

			// Create tenant manager
			tenantManager := tenant.NewManager(
				tenant.WithDatabase(dbConnectionPool),
			)

			// Create tenant service
			tenantService := TenantService{
				tenantManager:           tenantManager,
				dbConnectionPool:        dbConnectionPool,
				ownerEmail:              ownerEmail,
				ownerFirstName:          ownerFirstName,
				ownerLastName:           ownerLastName,
				distributionAccountType: schema.AccountType(distributionAccountType),
			}

			// Ensure default tenant
			err = service.EnsureDefaultTenant(ctx, tenantService)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error ensuring default tenant: %s", err.Error())
			}

			log.Ctx(ctx).Info("Default tenant ensured successfully")
		},
	}

	cmd.AddCommand(ensureDefaultCmd)

	err := configOpts.Init(ensureDefaultCmd)
	if err != nil {
		log.Ctx(cmd.Context()).Fatalf("Error initializing config options: %s", err.Error())
	}

	return cmd
}
