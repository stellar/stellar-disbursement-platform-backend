package provisioning

import (
	"context"
	"embed"
	"errors"
	"fmt"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
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

var (
	ErrTenantCreationFailed                     = errors.New("tenant creation failed")
	ErrTenantSchemaFailed                       = errors.New("database schema creation for tenant failed")
	ErrOpenTenantSchemaDBConnectionFailed       = errors.New("opening tenant schema database connection failed")
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

func rollbackTenantCreationErrors() []error {
	return []error{ErrUpdateTenantFailed, ErrProvisionTenantDistributionAccountFailed, ErrTenantDataSetupFailed, ErrOpenTenantSchemaDBConnectionFailed, ErrTenantSchemaFailed}
}

func rollbackTenantSchemaErrors() []error {
	return []error{ErrUpdateTenantFailed, ErrProvisionTenantDistributionAccountFailed, ErrTenantDataSetupFailed, ErrOpenTenantSchemaDBConnectionFailed, ErrTenantSchemaFailed, ErrTenantCreationFailed}
}

func (m *Manager) ProvisionNewTenant(
	ctx context.Context, name, userFirstName, userLastName, userEmail,
	organizationName, uiBaseURL, networkType string,
) (*tenant.Tenant, error) {
	log.Ctx(ctx).Infof("adding tenant %s", name)
	t, err := func() (*tenant.Tenant, error) {
		t, err := m.tenantManager.AddTenant(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("%w: adding tenant %s: %w", ErrTenantCreationFailed, name, err)
		}

		u, err := m.tenantManager.GetDSNForTenant(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("%w: getting database DSN for tenant %s: %w", ErrTenantSchemaFailed, name, err)
		}

		log.Ctx(ctx).Infof("creating tenant %s database schema", name)
		err = m.tenantManager.CreateTenantSchema(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("%w: creating tenant %s database schema: %w", ErrTenantSchemaFailed, name, err)
		}

		// Applying migrations
		log.Ctx(ctx).Infof("applying SDP migrations on the tenant %s schema", name)
		err = m.RunMigrationsForTenant(ctx, u, migrate.Up, 0, sdpmigrations.FS, db.StellarPerTenantSDPMigrationsTableName)
		if err != nil {
			return nil, fmt.Errorf("%w: applying SDP migrations: %w", ErrTenantSchemaFailed, err)
		}

		log.Ctx(ctx).Infof("applying stellar-auth migrations on the tenant %s schema", name)
		err = m.RunMigrationsForTenant(ctx, u, migrate.Up, 0, authmigrations.FS, db.StellarPerTenantAuthMigrationsTableName)
		if err != nil {
			return nil, fmt.Errorf("%w: applying stellar-auth migrations: %w", ErrTenantSchemaFailed, err)
		}

		// Connecting to the tenant database schema
		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u)
		if err != nil {
			return nil, fmt.Errorf("%w: opening database connection on tenant schema: %w", ErrOpenTenantSchemaDBConnectionFailed, err)
		}
		defer tenantSchemaConnectionPool.Close()

		tenantDataSetupErr := func() error {
			err = services.SetupAssetsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(networkType), services.DefaultAssetsNetworkMap)
			if err != nil {
				return fmt.Errorf("running setup assets for proper network: %w", err)
			}

			err = services.SetupWalletsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(networkType), services.DefaultWalletsNetworkMap)
			if err != nil {
				return fmt.Errorf("running setup wallets for proper network: %w", err)
			}

			// Updating organization's name
			models, getTntModelsErr := data.NewModels(tenantSchemaConnectionPool)
			if getTntModelsErr != nil {
				return fmt.Errorf("getting models: %w", err)
			}

			err = models.Organizations.Update(ctx, &data.OrganizationUpdate{Name: organizationName})
			if err != nil {
				return fmt.Errorf("updating organization's name: %w", err)
			}

			// Creating new user and sending invitation email
			authManager := auth.NewAuthManager(
				auth.WithDefaultAuthenticatorOption(tenantSchemaConnectionPool, auth.NewDefaultPasswordEncrypter(), 0),
			)
			s := services.NewCreateUserService(models, tenantSchemaConnectionPool, authManager, m.messengerClient)
			_, err = s.CreateUser(ctx, auth.User{
				FirstName: userFirstName,
				LastName:  userLastName,
				Email:     userEmail,
				IsOwner:   true,
				Roles:     []string{"owner"},
			}, uiBaseURL)
			if err != nil {
				return fmt.Errorf("creating user: %w", err)
			}

			return nil
		}()
		if tenantDataSetupErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrTenantDataSetupFailed, err)
		}

		// Provision distribution account for tenant if necessary
		distributionAccPubKeys, err := m.SubmitterEngine.DistAccountSigner.BatchInsert(ctx, 1)
		if err != nil {
			if errors.Is(err, signing.ErrUnsupportedCommand) {
				log.Ctx(ctx).Warnf(
					"Account provisioning not needed for distribution account signature client type %s: %v",
					m.SubmitterEngine.DistAccountSigner.Type(), err)
			} else {
				return nil, fmt.Errorf("%w: provisioning distribution account: %w", ErrProvisionTenantDistributionAccountFailed, err)
			}
		}

		// Assigning the account key to the tenant so that it can be referenced if it needs to be deleted in the vault if any subsequent errors are encountered
		t.DistributionAccount = &distributionAccPubKeys[0]
		if len(distributionAccPubKeys) != 1 {
			return t, fmt.Errorf("%w: expected single distribution account public key, got %d", ErrUpdateTenantFailed, len(distributionAccPubKeys))
		}
		log.Ctx(ctx).Infof("distribution account %s created for tenant %s", *t.DistributionAccount, name)

		tenantStatus := tenant.ProvisionedTenantStatus
		t, err = m.tenantManager.UpdateTenantConfig(
			ctx,
			&tenant.TenantUpdate{
				ID:                  t.ID,
				Status:              &tenantStatus,
				SDPUIBaseURL:        &uiBaseURL,
				DistributionAccount: t.DistributionAccount,
			})
		if err != nil {
			updateTenantErrMsg := fmt.Errorf("%w: updating tenant %s status to %s: %w", ErrUpdateTenantFailed, name, tenant.ProvisionedTenantStatus, err)
			return t, updateTenantErrMsg
		}

		return t, nil
	}()
	if err != nil {
		if errors.Is(err, ErrUpdateTenantFailed) {
			log.Ctx(ctx).Errorf("tenant record not updated")
		}

		for _, deleteDistributionKeyErr := range deleteDistributionKeyErrors() {
			if errors.Is(err, deleteDistributionKeyErr) {
				deleteDistributionAccFromVaultErr := m.deleteDistributionAccountKey(ctx, t)
				// We should not let any failures from key deletion block us from completing the tenant cleanup process
				if deleteDistributionKeyErr != nil {
					deleteDistributionKeyErrPrefixMsg := fmt.Sprintf("deleting distribution account private key %s", *t.DistributionAccount)
					err = fmt.Errorf("%w. [additional errors]: %s: %w", err, deleteDistributionKeyErrPrefixMsg, deleteDistributionAccFromVaultErr)
					log.Ctx(ctx).Errorf("%s: %v", deleteDistributionKeyErrPrefixMsg, deleteDistributionAccFromVaultErr)
				}

				log.Ctx(ctx).Errorf("distribution account cleanup successful")
				break
			}
		}

		for _, rollbackTenantDataSetupErr := range rollbackTenantDataSetupErrors() {
			if errors.Is(err, rollbackTenantDataSetupErr) {
				log.Ctx(ctx).Errorf("tenant data setup requires rollback")
				break
			}
		}

		for _, rollbackTenantCreationErr := range rollbackTenantCreationErrors() {
			if errors.Is(err, rollbackTenantCreationErr) {
				deleteTenantErr := m.tenantManager.DeleteTenantByName(ctx, name)
				if deleteTenantErr != nil {
					return nil, deleteTenantErr
				}

				log.Ctx(ctx).Errorf("tenant %s deleted", name)
				break
			}
		}

		for _, rollbackTenantSchemaErr := range rollbackTenantSchemaErrors() {
			if errors.Is(err, rollbackTenantSchemaErr) {
				dropTenantSchemaErr := m.tenantManager.DropTenantSchema(ctx, name)
				if dropTenantSchemaErr != nil {
					return nil, dropTenantSchemaErr
				}

				log.Ctx(ctx).Errorf("tenant schema sdp_%s dropped", name)
				break
			}
		}

		return nil, fmt.Errorf("most recent error: %w", err)
	}

	hostDistributionAccPubKey := m.SubmitterEngine.HostDistributionAccount()
	if *t.DistributionAccount != hostDistributionAccPubKey {
		err = tenant.ValidateNativeAssetBootstrapAmount(m.nativeAssetBootstrapAmount)
		if err != nil {
			return nil, fmt.Errorf("invalid native asset bootstrap amount: %w", err)
		}

		// Bootstrap tenant distribution account with native asset
		err = tssSvc.CreateAndFundAccount(ctx, m.SubmitterEngine, m.nativeAssetBootstrapAmount, hostDistributionAccPubKey, *t.DistributionAccount)
		if err != nil {
			return nil, fmt.Errorf("bootstrapping tenant distribution account with native asset: %w", err)
		}
	} else {
		log.Ctx(ctx).Info("Host distribution account and tenant distribution account are the same, no need to initiate funding.")
	}

	return t, nil
}

func (m *Manager) deleteDistributionAccountKey(ctx context.Context, t *tenant.Tenant) error {
	sigClientDeleteKeyErr := m.SubmitterEngine.DistAccountSigner.Delete(ctx, *t.DistributionAccount)
	if sigClientDeleteKeyErr != nil {
		sigClientDeleteKeyErrMsg := fmt.Errorf("unable to delete distribution account private key: %w", sigClientDeleteKeyErr)
		if errors.Is(sigClientDeleteKeyErr, signing.ErrUnsupportedCommand) {
			log.Ctx(ctx).Warnf(
				"Private key deletion not needed for distribution account signature client type %s: %v",
				m.SubmitterEngine.DistAccountSigner.Type(), sigClientDeleteKeyErr)
		} else {
			return sigClientDeleteKeyErrMsg
		}
	}

	return nil
}

func (m *Manager) RunMigrationsForTenant(
	ctx context.Context, dbURL string,
	dir migrate.MigrationDirection, count int,
	migrationFiles embed.FS, migrationTableName db.MigrationTableName,
) error {
	n, err := db.Migrate(dbURL, dir, count, migrationFiles, migrationTableName)
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
