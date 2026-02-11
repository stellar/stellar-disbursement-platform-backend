package provisioning

import (
	"context"
	"errors"
	"fmt"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go-stellar-sdk/support/log"

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
	Name                    string
	UserFirstName           string
	UserLastName            string
	UserEmail               string
	OrgName                 string
	UIBaseURL               string
	BaseURL                 string
	NetworkType             string
	DistributionAccountType schema.AccountType
	MFADisabled             *bool
	CAPTCHADisabled         *bool
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
) (*schema.Tenant, error) {
	log.Ctx(ctx).Infof("adding tenant %s", provisionTenant.Name)
	t, provisionErr := m.provisionTenant(ctx, &provisionTenant)
	if provisionErr != nil {
		return nil, m.handleProvisioningError(ctx, provisionErr, t)
	}

	return t, nil
}

func (m *Manager) handleProvisioningError(ctx context.Context, err error, t *schema.Tenant) error {
	// We don't want to roll back an existing tenant
	if errors.Is(err, tenant.ErrDuplicatedTenantName) {
		return err
	}

	provisioningErr := fmt.Errorf("provisioning error: %w", err)

	if errors.Is(err, ErrUpdateTenantFailed) {
		log.Ctx(ctx).Errorf("tenant record not updated: %v", err)
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

func (m *Manager) provisionTenant(ctx context.Context, pt *ProvisionTenant) (*schema.Tenant, error) {
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

	// Provision distribution account for tenant if necessary
	err := m.provisionDistributionAccount(ctx, t, pt.DistributionAccountType)
	if err != nil {
		return t, fmt.Errorf("provisioning distribution account: %w", err)
	}

	tenantStatus := schema.ProvisionedTenantStatus
	tenantUpdate := &tenant.TenantUpdate{
		ID:                        t.ID,
		Status:                    &tenantStatus,
		DistributionAccountType:   t.DistributionAccountType,
		DistributionAccountStatus: t.DistributionAccountStatus,
		SDPUIBaseURL:              &pt.UIBaseURL,
		BaseURL:                   &pt.BaseURL,
	}
	if t.DistributionAccountType.IsStellar() {
		tenantUpdate.DistributionAccountAddress = *t.DistributionAccountAddress
	}
	updatedTenant, err := m.tenantManager.UpdateTenantConfig(ctx, tenantUpdate)
	if err != nil {
		return t, fmt.Errorf("%w: updating tenant %s status to %s: %w", ErrUpdateTenantFailed, pt.Name, schema.ProvisionedTenantStatus, err)
	}

	err = m.fundTenantDistributionStellarAccountIfNeeded(ctx, *updatedTenant)
	if err != nil {
		return t, fmt.Errorf("%w. funding tenant distribution account: %w", ErrUpdateTenantFailed, err)
	}

	if updatedTenant.DistributionAccountType.IsStellar() && updatedTenant.DistributionAccountAddress != nil {
		if err := m.addTrustlinesForDistributionAccount(ctx, *updatedTenant); err != nil {
			return t, fmt.Errorf("%w. provisioning trustlines for distribution account: %w", ErrUpdateTenantFailed, err)
		}
	}

	return updatedTenant, nil
}

// fundTenantDistributionStellarAccountIfNeeded funds the tenant distribution account with native asset if necessary, based on the accountType provided.
func (m *Manager) fundTenantDistributionStellarAccountIfNeeded(ctx context.Context, tenant schema.Tenant) error {
	switch tenant.DistributionAccountType {
	case schema.DistributionAccountStellarDBVault:
		hostDistributionAccPubKey := m.SubmitterEngine.HostDistributionAccount()
		// Bootstrap tenant distribution account with native asset
		log.Ctx(ctx).Infof("Creating and funding tenant distribution account %s with %d XLM", *tenant.DistributionAccountAddress, m.nativeAssetBootstrapAmount)
		err := tssSvc.CreateAndFundAccount(ctx, m.SubmitterEngine, m.nativeAssetBootstrapAmount, hostDistributionAccPubKey.Address, *tenant.DistributionAccountAddress)
		if err != nil {
			return fmt.Errorf("bootstrapping tenant distribution account with native asset: %w", err)
		}
		return nil

	case schema.DistributionAccountStellarEnv:
		log.Ctx(ctx).Warnf("Tenant distribution account is configured to use accountType=%s, no need to initiate funding.", tenant.DistributionAccountType)
		return nil

	case schema.DistributionAccountCircleDBVault:
		log.Ctx(ctx).Warnf("Tenant distribution account is configured to use accountType=%s, the tenant will need to complete the setup through the UI.", tenant.DistributionAccountType)
		return nil

	default:
		return fmt.Errorf("unsupported accountType=%s", tenant.DistributionAccountType)
	}
}

func (m *Manager) addTrustlinesForDistributionAccount(ctx context.Context, tenant schema.Tenant) error {
	tenantSchemaDSN, err := m.tenantManager.GetDSNForTenant(ctx, tenant.Name)
	if err != nil {
		return fmt.Errorf("getting tenant DSN: %w", err)
	}

	tenantSchemaConnectionPool, models, err := GetTenantSchemaDBConnectionAndModels(tenantSchemaDSN)
	if err != nil {
		return fmt.Errorf("opening tenant schema connection: %w", err)
	}
	defer utils.DeferredClose(ctx, tenantSchemaConnectionPool, "closing tenant schema connection pool after adding trustlines")

	// Gather the non-native assets currently linked to enabled wallets.
	wallets, err := models.Wallets.FindWallets(ctx, data.NewFilter(data.FilterEnabledWallets, true))
	if err != nil {
		return fmt.Errorf("listing enabled wallets: %w", err)
	}

	supportedAssets := make(map[string]data.Asset)
	for _, wallet := range wallets {
		for _, asset := range wallet.Assets {
			if asset.IsNative() {
				continue
			}
			key := fmt.Sprintf("%s:%s", asset.Code, asset.Issuer)
			supportedAssets[key] = asset
		}
	}

	if len(supportedAssets) == 0 {
		log.Ctx(ctx).Info("no non-native supported assets found for tenant during provisioning; skipping trustline setup")
		return nil
	}

	distAccount := schema.TransactionAccount{
		Address: *tenant.DistributionAccountAddress,
		Type:    tenant.DistributionAccountType,
		Status:  tenant.DistributionAccountStatus,
	}

	assetsToTrust := make([]data.Asset, 0, len(supportedAssets))
	for _, asset := range supportedAssets {
		assetsToTrust = append(assetsToTrust, asset)
	}

	_, err = tssSvc.AddTrustlines(ctx, m.SubmitterEngine, distAccount, assetsToTrust)
	if err != nil {
		return fmt.Errorf("submitting change trust transaction: %w", err)
	}

	return nil
}

// provisionDistributionAccount provisions a distribution account for the tenant if necessary, based on the accountType provided.
func (m *Manager) provisionDistributionAccount(ctx context.Context, t *schema.Tenant, accountType schema.AccountType) error {
	switch accountType {
	case schema.DistributionAccountCircleDBVault:
		log.Ctx(ctx).Warnf("Circle account cannot be automatically provisioned, the tenant %s will need to provision it through the UI.", t.Name)
		t.DistributionAccountType = accountType
		t.DistributionAccountStatus = schema.AccountStatusPendingUserActivation
		return nil

	case schema.DistributionAccountStellarEnv, schema.DistributionAccountStellarDBVault:
		distributionAccounts, err := m.SubmitterEngine.SignerRouter.BatchInsert(ctx, accountType, 1)
		if err != nil {
			if errors.Is(err, signing.ErrUnsupportedCommand) {
				log.Ctx(ctx).Warnf("Account provisioning for distribution account of type=%s is NO-OP: %v", accountType, err)
			} else {
				return fmt.Errorf("%w: provisioning distribution account: %w", ErrProvisionTenantDistributionAccountFailed, err)
			}
		}

		// Assigning the account key to the tenant so that it can be referenced if it needs to be deleted in the vault if any subsequent errors are encountered
		if len(distributionAccounts) != 1 {
			return fmt.Errorf("%w: expected single distribution account public key, got %d", ErrUpdateTenantFailed, len(distributionAccounts))
		}
		t.DistributionAccountAddress = &distributionAccounts[0].Address
		t.DistributionAccountType = accountType
		t.DistributionAccountStatus = schema.AccountStatusActive
		log.Ctx(ctx).Infof("distribution account for tenant %s was set to %s", t.Name, *t.DistributionAccountAddress)
		return nil

	default:
		return fmt.Errorf("%w: unsupported accountType=%s", ErrProvisionTenantDistributionAccountFailed, accountType)
	}
}

func (m *Manager) setupTenantData(ctx context.Context, tenantSchemaDSN string, pt *ProvisionTenant) error {
	tenantSchemaConnectionPool, models, err := GetTenantSchemaDBConnectionAndModels(tenantSchemaDSN)
	if err != nil {
		return fmt.Errorf("opening database connection on tenant schema and getting models: %w", err)
	}
	defer utils.DeferredClose(ctx, tenantSchemaConnectionPool, "closing tenant schema connection pool")

	err = services.SetupAssetsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(pt.NetworkType), pt.DistributionAccountType.Platform())
	if err != nil {
		return fmt.Errorf("running setup assets for proper network: %w", err)
	}

	err = services.SetupWalletsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(pt.NetworkType), services.DefaultWalletsNetworkMap)
	if err != nil {
		return fmt.Errorf("running setup wallets for proper network: %w", err)
	}

	err = models.Organizations.Update(ctx, &data.OrganizationUpdate{
		Name:                   pt.OrgName,
		MFADisabled:            pt.MFADisabled,
		CAPTCHADisabled:        pt.CAPTCHADisabled,
		IsLinkShortenerEnabled: utils.Ptr(true),
	})
	if err != nil {
		return fmt.Errorf("updating organization's name and settings: %w", err)
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
	runTntMigrationsErr := m.applyTenantMigrations(ctx, dsn, migrations.SDPMigrationRouter)
	if runTntMigrationsErr != nil {
		return "", fmt.Errorf("applying SDP migrations: %w", runTntMigrationsErr)
	}

	log.Ctx(ctx).Infof("applying stellar-auth migrations on the tenant %s schema", name)
	runTntAuthMigrationsErr := m.applyTenantMigrations(ctx, dsn, migrations.AuthMigrationRouter)
	if runTntAuthMigrationsErr != nil {
		return "", fmt.Errorf("applying stellar-auth migrations: %w", runTntAuthMigrationsErr)
	}

	return dsn, nil
}

func (m *Manager) deleteDistributionAccountKey(ctx context.Context, t *schema.Tenant) error {
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

// applyTenantMigrations applies the migrations on the tenant schema when provisioning a new tenant.
func (m *Manager) applyTenantMigrations(
	ctx context.Context,
	dbURL string,
	migrationRouter migrations.MigrationRouter,
) error {
	n, err := db.Migrate(dbURL, migrate.Up, 0, migrationRouter)
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
