package provisioning

import (
	"context"
	"errors"
	"fmt"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type Manager struct {
	tenantManager              tenant.ManagerInterface
	db                         db.DBConnectionPool
	SubmitterEngine            engine.SubmitterEngine
	nativeAssetBootstrapAmount int
}

// ProvisionTenant contains all the metadata about a tenant to provision one
type ProvisionTenant struct {
	Name          string
	UserFirstName string
	UserLastName  string
	UserEmail     string
	OrgName       string
	UiBaseURL     string
	BaseURL       string
	NetworkType   string
}

var (
	ErrTenantCreationFailed                     = errors.New("tenant creation failed")
	ErrTenantSchemaFailed                       = errors.New("database schema creation for tenant failed")
	ErrTenantDataSetupFailed                    = errors.New("tenant data setup failed")
	ErrProvisionTenantDistributionAccountFailed = errors.New("tenant distribution account provisioning failed")
	ErrUpdateTenantFailed                       = errors.New("tenant update failed")
)

func deleteDistributionKeyErrors() []error {
	return []error{ErrUpdateTenantFailed}
}

func rollbackTenantDataSetupErrors() []error {
	return append(deleteDistributionKeyErrors(), ErrProvisionTenantDistributionAccountFailed, ErrTenantDataSetupFailed)
}

func rollbackTenantCreationAndSchemaErrors() []error {
	return append(rollbackTenantDataSetupErrors(), ErrTenantSchemaFailed)
}

func (m *Manager) ProvisionNewTenant(
	ctx context.Context, provisionTenant ProvisionTenant,
) (*tenant.Tenant, error) {
	log.Ctx(ctx).Infof("adding tenant %s", provisionTenant.Name)
	t, provisionErr := m.provisionTenant(ctx, &provisionTenant)
	if provisionErr != nil {
		return nil, m.handleProvisioningError(ctx, provisionErr, t)
	}

	return t, nil
}

func (m *Manager) handleProvisioningError(ctx context.Context, err error, t *tenant.Tenant) error {
	// We don't want to roll back an existing tenant
	if errors.Is(err, tenant.ErrDuplicatedTenantName) {
		return err
	}

	provisioningErr := fmt.Errorf("provisioning error: %w", err)

	if errors.Is(err, ErrUpdateTenantFailed) {
		log.Ctx(ctx).Errorf("tenant record not updated")
	}

	if isErrorInArray(err, deleteDistributionKeyErrors()) {
		deleteDistributionAccFromVaultErr := m.deleteDistributionAccountKey(ctx, t)
		// We should not let any failures from key deletion block us from completing the tenant cleanup process
		if deleteDistributionAccFromVaultErr != nil {
			deleteDistributionKeyErrPrefixMsg := fmt.Sprintf("deleting distribution account private key %s", *t.DistributionAccountAddress)
			provisioningErr = fmt.Errorf("%w. [additional errors]: %s: %w", provisioningErr, deleteDistributionKeyErrPrefixMsg, deleteDistributionAccFromVaultErr)
			log.Ctx(ctx).Errorf("%s: %v", deleteDistributionKeyErrPrefixMsg, deleteDistributionAccFromVaultErr)
		}
		log.Ctx(ctx).Errorf("distribution account cleanup successful")
	}

	if isErrorInArray(err, rollbackTenantDataSetupErrors()) {
		log.Ctx(ctx).Errorf("tenant data setup requires rollback")
	}

	if isErrorInArray(err, rollbackTenantCreationAndSchemaErrors()) {
		deleteTenantErr := m.tenantManager.DeleteTenantByName(ctx, t.Name)
		if deleteTenantErr != nil {
			return fmt.Errorf("%w. [rollback error]: %w", provisioningErr, deleteTenantErr)
		}

		log.Ctx(ctx).Errorf("tenant %s deleted", t.Name)

		dropTenantSchemaErr := m.tenantManager.DropTenantSchema(ctx, t.Name)
		if dropTenantSchemaErr != nil {
			return fmt.Errorf("%w. [rollback error]: %w", provisioningErr, dropTenantSchemaErr)
		}

		log.Ctx(ctx).Errorf("tenant schema sdp_%s dropped", t.Name)
	}

	return provisioningErr
}

func (m *Manager) provisionTenant(ctx context.Context, pt *ProvisionTenant) (*tenant.Tenant, error) {
	t, addTntErr := m.tenantManager.AddTenant(ctx, pt.Name)
	if addTntErr != nil {
		return t, fmt.Errorf("%w: adding tenant %s: %w", ErrTenantCreationFailed, pt.Name, addTntErr)
	}

	tenantSchemaDSN, tenantSchemaFailedErr := m.createSchemaAndRunMigrations(ctx, pt.Name)
	if tenantSchemaFailedErr != nil {
		return t, fmt.Errorf("%w: %w", ErrTenantSchemaFailed, tenantSchemaFailedErr)
	}

	tenantDataSetupErr := m.setupTenantData(ctx, tenantSchemaDSN, pt)
	if tenantDataSetupErr != nil {
		return t, fmt.Errorf("%w: %w", ErrTenantDataSetupFailed, tenantDataSetupErr)
	}

	// TODO: replace this hardcoded value with the one from the json payload in SDP-1167
	distAccType := schema.DistributionAccountStellarDBVault

	// Provision distribution account for tenant if necessary
	err := m.provisionDistributionAccount(ctx, t, distAccType)
	if err != nil {
		return t, fmt.Errorf("provisioning distribution account: %w", err)
	}

	tenantStatus := tenant.ProvisionedTenantStatus
	updatedTenant, err := m.tenantManager.UpdateTenantConfig(
		ctx,
		&tenant.TenantUpdate{
			ID:                         t.ID,
			Status:                     &tenantStatus,
			DistributionAccountAddress: *t.DistributionAccountAddress,
			DistributionAccountType:    t.DistributionAccountType,
			DistributionAccountStatus:  schema.AccountStatusActive,
			SDPUIBaseURL:               &pt.UiBaseURL,
			BaseURL:                    &pt.BaseURL,
		})
	if err != nil {
		return t, fmt.Errorf("%w: updating tenant %s status to %s: %w", ErrUpdateTenantFailed, pt.Name, tenant.ProvisionedTenantStatus, err)
	}

	err = m.fundTenantDistributionAccount(ctx, *updatedTenant.DistributionAccountAddress)
	if err != nil {
		return t, fmt.Errorf("%w. funding tenant distribution account: %w", ErrUpdateTenantFailed, err)
	}

	return updatedTenant, nil
}

func (m *Manager) fundTenantDistributionAccount(ctx context.Context, distributionAccount string) error {
	hostDistributionAccPubKey := m.SubmitterEngine.HostDistributionAccount()
	if distributionAccount != hostDistributionAccPubKey.Address {
		// Bootstrap tenant distribution account with native asset
		log.Ctx(ctx).Infof("Creating and funding tenant distribution account %s with native asset", distributionAccount)
		err := tssSvc.CreateAndFundAccount(ctx, m.SubmitterEngine, m.nativeAssetBootstrapAmount, hostDistributionAccPubKey.Address, distributionAccount)
		if err != nil {
			return fmt.Errorf("bootstrapping tenant distribution account with native asset: %w", err)
		}
	} else {
		log.Ctx(ctx).Info("host distribution account and tenant distribution account are the same, no need to initiate funding.")
	}
	return nil
}

func (m *Manager) provisionDistributionAccount(ctx context.Context, t *tenant.Tenant, accountType schema.AccountType) error {
	distributionAccounts, err := m.SubmitterEngine.SignerRouter.BatchInsert(ctx, accountType, 1)
	if err != nil {
		if errors.Is(err, signing.ErrUnsupportedCommand) {
			log.Ctx(ctx).Warnf(
				"Account provisioning not needed for distribution account of type=%s: %v",
				accountType, err)
		} else {
			return fmt.Errorf("%w: provisioning distribution account: %w", ErrProvisionTenantDistributionAccountFailed, err)
		}
	}

	// Assigning the account key to the tenant so that it can be referenced if it needs to be deleted in the vault if any subsequent errors are encountered
	if len(distributionAccounts) != 1 {
		return fmt.Errorf("%w: expected single distribution account public key, got %d", ErrUpdateTenantFailed, len(distributionAccounts))
	}
	t.DistributionAccountAddress = &distributionAccounts[0].Address
	t.DistributionAccountType = distributionAccounts[0].Type
	log.Ctx(ctx).Infof("distribution account %s created for tenant %s", *t.DistributionAccountAddress, t.Name)
	return nil
}

func (m *Manager) setupTenantData(ctx context.Context, tenantSchemaDSN string, pt *ProvisionTenant) error {
	tenantSchemaConnectionPool, models, err := GetTenantSchemaDBConnectionAndModels(tenantSchemaDSN)
	if err != nil {
		return fmt.Errorf("opening database connection on tenant schema and getting models: %w", err)
	}
	defer tenantSchemaConnectionPool.Close()

	err = services.SetupAssetsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(pt.NetworkType), services.DefaultAssetsNetworkMap)
	if err != nil {
		return fmt.Errorf("running setup assets for proper network: %w", err)
	}

	err = services.SetupWalletsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(pt.NetworkType), services.DefaultWalletsNetworkMap)
	if err != nil {
		return fmt.Errorf("running setup wallets for proper network: %w", err)
	}

	err = models.Organizations.Update(ctx, &data.OrganizationUpdate{Name: pt.OrgName})
	if err != nil {
		return fmt.Errorf("updating organization's name: %w", err)
	}

	// Creating new user and sending invitation email
	authManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(tenantSchemaConnectionPool, auth.NewDefaultPasswordEncrypter(), 0),
	)

	_, err = authManager.CreateUser(ctx, &auth.User{
		FirstName: pt.UserFirstName,
		LastName:  pt.UserLastName,
		Email:     pt.UserEmail,
		IsOwner:   true,
		Roles:     []string{data.OwnerUserRole.String()},
	}, "")
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	return nil
}

func (m *Manager) createSchemaAndRunMigrations(ctx context.Context, name string) (string, error) {
	dsn, getDSNForTntErr := m.tenantManager.GetDSNForTenant(ctx, name)
	if getDSNForTntErr != nil {
		return "", fmt.Errorf("getting database DSN for tenant %s: %w", name, getDSNForTntErr)
	}

	log.Ctx(ctx).Infof("creating tenant %s database schema", name)
	createTntSchemaErr := m.tenantManager.CreateTenantSchema(ctx, name)
	if createTntSchemaErr != nil {
		return "", fmt.Errorf("creating tenant %s database schema: %w", name, createTntSchemaErr)
	}

	// Applying migrations
	log.Ctx(ctx).Infof("applying SDP migrations on the tenant %s schema", name)
	runTntMigrationsErr := m.runMigrationsForTenant(ctx, dsn, migrate.Up, 0, migrations.SDPMigrationRouter)
	if runTntMigrationsErr != nil {
		return "", fmt.Errorf("applying SDP migrations: %w", runTntMigrationsErr)
	}

	log.Ctx(ctx).Infof("applying stellar-auth migrations on the tenant %s schema", name)
	runTntAuthMigrationsErr := m.runMigrationsForTenant(ctx, dsn, migrate.Up, 0, migrations.AuthMigrationRouter)
	if runTntAuthMigrationsErr != nil {
		return "", fmt.Errorf("applying stellar-auth migrations: %w", runTntAuthMigrationsErr)
	}

	return dsn, nil
}

func (m *Manager) deleteDistributionAccountKey(ctx context.Context, t *tenant.Tenant) error {
	distAccToDelete := schema.TransactionAccount{
		Address: *t.DistributionAccountAddress,
		Type:    t.DistributionAccountType,
		Status:  schema.AccountStatusActive,
	}
	sigClientDeleteKeyErr := m.SubmitterEngine.SignerRouter.Delete(ctx, distAccToDelete)
	if sigClientDeleteKeyErr != nil {
		if errors.Is(sigClientDeleteKeyErr, signing.ErrUnsupportedCommand) {
			log.Ctx(ctx).Warnf(
				"Private key deletion not needed for distribution account of type=%s: %v",
				t.DistributionAccountType, sigClientDeleteKeyErr)
		} else {
			return fmt.Errorf("unable to delete distribution account private key: %w", sigClientDeleteKeyErr)
		}
	}
	return nil
}

func (m *Manager) runMigrationsForTenant(
	ctx context.Context, dbURL string,
	dir migrate.MigrationDirection, count int,
	migrationRouter migrations.MigrationRouter,
) error {
	n, err := db.Migrate(dbURL, dir, count, migrationRouter)
	if err != nil {
		return fmt.Errorf("applying SDP migrations: %w", err)
	}
	log.Ctx(ctx).Infof("successful applied %d migrations", n)
	return nil
}

// GetTenantSchemaDBConnectionAndModels returns an opened database connection on the tenant schema and returns the models associated with the schema.
// The opened connection will be up to the caller to close.
func GetTenantSchemaDBConnectionAndModels(tenantSchemaDSN string) (tenantSchemaDBConnectionPool db.DBConnectionPool, models *data.Models, err error) {
	tenantSchemaDBConnectionPool, err = db.OpenDBConnectionPool(tenantSchemaDSN)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database connection on tenant schema: %w", err)
	}

	models, err = data.NewModels(tenantSchemaDBConnectionPool)
	if err != nil {
		return nil, nil, fmt.Errorf("getting models for tenant schema: %w", err)
	}

	return tenantSchemaDBConnectionPool, models, nil
}

type Option func(m *Manager) error

type ManagerOptions struct {
	DBConnectionPool           db.DBConnectionPool
	TenantManager              tenant.ManagerInterface
	SubmitterEngine            engine.SubmitterEngine
	NativeAssetBootstrapAmount int
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	if opts.DBConnectionPool == nil {
		return nil, fmt.Errorf("database connection pool cannot be nil")
	}

	if opts.TenantManager == nil {
		return nil, fmt.Errorf("tenant manager cannot be nil")
	}

	err := opts.SubmitterEngine.Validate()
	if err != nil {
		return nil, fmt.Errorf("validating submitter engine: %w", err)
	}

	isTooSmall := opts.NativeAssetBootstrapAmount < tenant.MinTenantDistributionAccountAmount
	isTooBig := opts.NativeAssetBootstrapAmount > tenant.MaxTenantDistributionAccountAmount
	if isTooSmall || isTooBig {
		return nil, fmt.Errorf(
			"the amount of XLM configured (%d XLM) is outside the permitted range of [%d XLM, %d XLM]",
			opts.NativeAssetBootstrapAmount,
			tenant.MinTenantDistributionAccountAmount,
			tenant.MaxTenantDistributionAccountAmount,
		)
	}

	return &Manager{
		db:                         opts.DBConnectionPool,
		tenantManager:              opts.TenantManager,
		SubmitterEngine:            opts.SubmitterEngine,
		nativeAssetBootstrapAmount: opts.NativeAssetBootstrapAmount,
	}, nil
}

func isErrorInArray(target error, errArray []error) bool {
	for _, err := range errArray {
		if errors.Is(target, err) {
			return true
		}
	}
	return false
}
