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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type Manager struct {
	tenantManager   tenant.ManagerInterface
	db              db.DBConnectionPool
	messengerClient message.MessengerClient
	engine.SubmitterEngine
	nativeAssetBootstrapAmount int
}

// ProvisionTenant contains all the metadata about a tenant to provision one
type ProvisionTenant struct {
	name          string
	userFirstName string
	userLastName  string
	userEmail     string
	orgName       string
	uiBaseURL     string
	networkType   string
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
	return []error{ErrUpdateTenantFailed, ErrProvisionTenantDistributionAccountFailed, ErrTenantDataSetupFailed}
}

func rollbackTenantCreationAndSchemaErrors() []error {
	return []error{ErrUpdateTenantFailed, ErrProvisionTenantDistributionAccountFailed, ErrTenantDataSetupFailed, ErrTenantSchemaFailed}
}

func (m *Manager) ProvisionNewTenant(
	ctx context.Context, name, userFirstName, userLastName, userEmail,
	organizationName, uiBaseURL, networkType string,
) (*tenant.Tenant, error) {
	pt := &ProvisionTenant{
		name:          name,
		userFirstName: userFirstName,
		userLastName:  userLastName,
		userEmail:     userEmail,
		uiBaseURL:     uiBaseURL,
		orgName:       organizationName,
		networkType:   networkType,
	}

	log.Ctx(ctx).Infof("adding tenant %s", name)
	t, provisionErr := m.provisionTenant(ctx, pt)
	if provisionErr != nil {
		return nil, m.handleProvisioningError(ctx, provisionErr, t)
	}

	// Last step when no errors - fund tenant distribution account
	fundErr := m.fundTenantDistributionAccount(ctx, *t.DistributionAccountAddress)
	if fundErr != nil {
		// error already wrapped
		return nil, fundErr
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
	t, addTntErr := m.tenantManager.AddTenant(ctx, pt.name)
	if addTntErr != nil {
		return t, fmt.Errorf("%w: adding tenant %s: %w", ErrTenantCreationFailed, pt.name, addTntErr)
	}

	u, tenantSchemaFailedErr := m.createSchemaAndRunMigrations(ctx, pt.name)
	if tenantSchemaFailedErr != nil {
		return t, fmt.Errorf("%w: %w", ErrTenantSchemaFailed, tenantSchemaFailedErr)
	}

	tenantDataSetupErr := m.setupTenantData(ctx, u, pt)
	if tenantDataSetupErr != nil {
		return t, fmt.Errorf("%w: %w", ErrTenantDataSetupFailed, tenantDataSetupErr)
	}

	// Provision distribution account for tenant if necessary
	err := m.provisionDistributionAccount(ctx, t)
	if err != nil {
		// error already wrapped
		return t, err
	}

	tenantStatus := tenant.ProvisionedTenantStatus
	t, err = m.tenantManager.UpdateTenantConfig(
		ctx,
		&tenant.TenantUpdate{
			ID:                         t.ID,
			Status:                     &tenantStatus,
			SDPUIBaseURL:               &pt.uiBaseURL,
			DistributionAccountAddress: t.DistributionAccountAddress,
		})
	if err != nil {
		return t, fmt.Errorf("%w: updating tenant %s status to %s: %w", ErrUpdateTenantFailed, pt.name, tenant.ProvisionedTenantStatus, err)
	}

	return t, nil
}

func (m *Manager) fundTenantDistributionAccount(ctx context.Context, distributionAccount string) error {
	hostDistributionAccPubKey := m.SubmitterEngine.HostDistributionAccount()
	if distributionAccount != hostDistributionAccPubKey {
		err := tenant.ValidateNativeAssetBootstrapAmount(m.nativeAssetBootstrapAmount)
		if err != nil {
			return fmt.Errorf("invalid native asset bootstrap amount: %w", err)
		}

		// Bootstrap tenant distribution account with native asset
		err = tssSvc.CreateAndFundAccount(ctx, m.SubmitterEngine, m.nativeAssetBootstrapAmount, hostDistributionAccPubKey, distributionAccount)
		if err != nil {
			return fmt.Errorf("bootstrapping tenant distribution account with native asset: %w", err)
		}
	} else {
		log.Ctx(ctx).Info("host distribution account and tenant distribution account are the same, no need to initiate funding.")
	}
	return nil
}

func (m *Manager) provisionDistributionAccount(ctx context.Context, t *tenant.Tenant) error {
	distributionAccPubKeys, err := m.SubmitterEngine.DistAccountSigner.BatchInsert(ctx, 1)
	if err != nil {
		if errors.Is(err, signing.ErrUnsupportedCommand) {
			log.Ctx(ctx).Warnf(
				"Account provisioning not needed for distribution account signature client type %s: %v",
				m.SubmitterEngine.DistAccountSigner.Type(), err)
		} else {
			return fmt.Errorf("%w: provisioning distribution account: %w", ErrProvisionTenantDistributionAccountFailed, err)
		}
	}

	// Assigning the account key to the tenant so that it can be referenced if it needs to be deleted in the vault if any subsequent errors are encountered
	t.DistributionAccountAddress = &distributionAccPubKeys[0]
	if len(distributionAccPubKeys) != 1 {
		return fmt.Errorf("%w: expected single distribution account public key, got %d", ErrUpdateTenantFailed, len(distributionAccPubKeys))
	}
	log.Ctx(ctx).Infof("distribution account %s created for tenant %s", *t.DistributionAccountAddress, t.Name)
	return nil
}

func (m *Manager) setupTenantData(ctx context.Context, tenantSchemaDSN string, pt *ProvisionTenant) error {
	// Connecting to the tenant database schema
	tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(tenantSchemaDSN)
	if err != nil {
		return fmt.Errorf("opening database connection on tenant schema: %w", err)
	}
	defer tenantSchemaConnectionPool.Close()

	err = services.SetupAssetsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(pt.networkType), services.DefaultAssetsNetworkMap)
	if err != nil {
		return fmt.Errorf("running setup assets for proper network: %w", err)
	}

	err = services.SetupWalletsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(pt.networkType), services.DefaultWalletsNetworkMap)
	if err != nil {
		return fmt.Errorf("running setup wallets for proper network: %w", err)
	}

	// Updating organization's name
	models, getTntModelsErr := data.NewModels(tenantSchemaConnectionPool)
	if getTntModelsErr != nil {
		return fmt.Errorf("getting models: %w", err)
	}

	err = models.Organizations.Update(ctx, &data.OrganizationUpdate{Name: pt.orgName})
	if err != nil {
		return fmt.Errorf("updating organization's name: %w", err)
	}

	// Creating new user and sending invitation email
	authManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(tenantSchemaConnectionPool, auth.NewDefaultPasswordEncrypter(), 0),
	)
	s := services.NewCreateUserService(models, tenantSchemaConnectionPool, authManager, m.messengerClient)
	_, err = s.CreateUser(ctx, auth.User{
		FirstName: pt.userFirstName,
		LastName:  pt.userLastName,
		Email:     pt.userEmail,
		IsOwner:   true,
		Roles:     []string{"owner"},
	}, pt.uiBaseURL)
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	return nil
}

func (m *Manager) createSchemaAndRunMigrations(ctx context.Context, name string) (string, error) {
	u, getDSNForTntErr := m.tenantManager.GetDSNForTenant(ctx, name)
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
	runTntMigrationsErr := m.runMigrationsForTenant(ctx, u, migrate.Up, 0, migrations.SDPMigrationRouter)
	if runTntMigrationsErr != nil {
		return "", fmt.Errorf("applying SDP migrations: %w", runTntMigrationsErr)
	}

	log.Ctx(ctx).Infof("applying stellar-auth migrations on the tenant %s schema", name)
	runTntAuthMigrationsErr := m.runMigrationsForTenant(ctx, u, migrate.Up, 0, migrations.AuthMigrationRouter)
	if runTntAuthMigrationsErr != nil {
		return "", fmt.Errorf("applying stellar-auth migrations: %w", runTntAuthMigrationsErr)
	}

	return u, nil
}

func (m *Manager) deleteDistributionAccountKey(ctx context.Context, t *tenant.Tenant) error {
	sigClientDeleteKeyErr := m.SubmitterEngine.DistAccountSigner.Delete(ctx, *t.DistributionAccountAddress)
	if sigClientDeleteKeyErr != nil {
		if errors.Is(sigClientDeleteKeyErr, signing.ErrUnsupportedCommand) {
			log.Ctx(ctx).Warnf(
				"Private key deletion not needed for distribution account signature client type %s: %v",
				m.SubmitterEngine.DistAccountSigner.Type(), sigClientDeleteKeyErr)
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

type Option func(m *Manager)

func NewManager(opts ...Option) *Manager {
	m := Manager{}
	for _, opt := range opts {
		opt(&m)
	}
	return &m
}

func WithDatabase(dbConnectionPool db.DBConnectionPool) Option {
	return func(m *Manager) {
		m.db = dbConnectionPool
	}
}

func WithMessengerClient(messengerClient message.MessengerClient) Option {
	return func(m *Manager) {
		m.messengerClient = messengerClient
	}
}

func WithTenantManager(tenantManager tenant.ManagerInterface) Option {
	return func(m *Manager) {
		m.tenantManager = tenantManager
	}
}

func WithSubmitterEngine(submitterEngine engine.SubmitterEngine) Option {
	return func(m *Manager) {
		m.SubmitterEngine = submitterEngine
	}
}

func WithNativeAssetBootstrapAmount(nativeAssetBootstrapAmount int) Option {
	return func(m *Manager) {
		m.nativeAssetBootstrapAmount = nativeAssetBootstrapAmount
	}
}

func isErrorInArray(target error, errArray []error) bool {
	for _, err := range errArray {
		if errors.Is(target, err) {
			return true
		}
	}
	return false
}
