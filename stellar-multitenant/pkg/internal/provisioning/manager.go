package provisioning

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type Manager struct {
	tenantManager    *tenant.Manager
	db               db.DBConnectionPool
	messengerClient  message.MessengerClient
	distAccSigClient signing.SignatureClient
}

func (m *Manager) ProvisionNewTenant(
	ctx context.Context, name, userFirstName, userLastName, userEmail,
	organizationName, uiBaseURL, networkType string,
) (*tenant.Tenant, error) {
	// TODO (SDP-1107): Run this in a database transaction.
	log.Infof("adding tenant %s", name)
	t, err := m.tenantManager.AddTenant(ctx, name)
	if err != nil {
		return nil, err
	}

	log.Infof("creating tenant %s database schema", t.Name)
	schemaName := fmt.Sprintf("sdp_%s", t.Name)
	_, err = m.db.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", pq.QuoteIdentifier(schemaName)))
	if err != nil {
		return nil, fmt.Errorf("creating a new database schema: %w", err)
	}

	u, err := m.tenantManager.GetDSNForTenant(ctx, t.Name)
	if err != nil {
		return nil, fmt.Errorf("getting database DSN for tenant %s: %w", t.Name, err)
	}

	// Applying migrations
	log.Infof("applying SDP migrations on the tenant %s schema", t.Name)
	err = m.RunMigrationsForTenant(ctx, t, u, migrate.Up, 0, sdpmigrations.FS, db.StellarPerTenantSDPMigrationsTableName)
	if err != nil {
		return nil, fmt.Errorf("applying SDP migrations: %w", err)
	}

	log.Infof("applying stellar-auth migrations on the tenant %s schema", t.Name)
	err = m.RunMigrationsForTenant(ctx, t, u, migrate.Up, 0, authmigrations.FS, db.StellarPerTenantAuthMigrationsTableName)
	if err != nil {
		return nil, fmt.Errorf("applying stellar-auth migrations: %w", err)
	}

	// Connecting to the tenant database schema
	tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u)
	if err != nil {
		return nil, fmt.Errorf("opening database connection on tenant schema: %w", err)
	}
	defer tenantSchemaConnectionPool.Close()

	err = services.SetupAssetsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(networkType), services.DefaultAssetsNetworkMap)
	if err != nil {
		return nil, fmt.Errorf("running setup assets for proper network: %w", err)
	}

	err = services.SetupWalletsForProperNetwork(ctx, tenantSchemaConnectionPool, utils.NetworkType(networkType), services.DefaultWalletsNetworkMap)
	if err != nil {
		return nil, fmt.Errorf("running setup wallets for proper network: %w", err)
	}

	// Updating organization's name
	models, err := data.NewModels(tenantSchemaConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("getting models: %w", err)
	}

	err = models.Organizations.Update(ctx, &data.OrganizationUpdate{Name: organizationName})
	if err != nil {
		return nil, fmt.Errorf("updating organization's name: %w", err)
	}

	// Provision distribution account for tenant if necessary
	distributionAccPubKeys, err := m.distAccSigClient.BatchInsert(ctx, 1)
	if err != nil {
		if errors.Is(err, signing.ErrUnsupportedCommand) {
			log.Ctx(ctx).Warnf(
				"Account provisioning not needed for distribution account signature client type %s: %v",
				m.distAccSigClient.Type(), err)
		} else {
			return nil, fmt.Errorf("provisioning distribution account: %w", err)
		}
	}
	if len(distributionAccPubKeys) != 1 {
		return nil, fmt.Errorf("expected single distribution account public key, got %d", len(distributionAccPubKeys))
	}

	distributionAccPubKey := distributionAccPubKeys[0]
	log.Ctx(ctx).Infof("distribution account %s created for tenant %s", distributionAccPubKey, t.Name)

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
		return nil, fmt.Errorf("creating user: %w", err)
	}

	tenantStatus := tenant.ProvisionedTenantStatus
	t, err = m.tenantManager.UpdateTenantConfig(
		ctx,
		&tenant.TenantUpdate{
			ID:                  t.ID,
			Status:              &tenantStatus,
			SDPUIBaseURL:        &uiBaseURL,
			DistributionAccount: &distributionAccPubKey,
		})
	if err != nil {
		updateTenantErrMsg := fmt.Errorf("updating tenant %s status to %s: %w", name, tenant.ProvisionedTenantStatus, err)
		// Rollback distribution account provisioning
		sigClientDeleteKeyErr := m.distAccSigClient.Delete(ctx, distributionAccPubKey)
		if sigClientDeleteKeyErr != nil {
			sigClientDeleteKeyErrMsg := fmt.Errorf("unable to delete distribution account private key: %w", sigClientDeleteKeyErr)
			if errors.Is(sigClientDeleteKeyErr, signing.ErrUnsupportedCommand) {
				log.Ctx(ctx).Warnf(
					"Private key deletion not needed for distribution account signature client type %s: %v",
					m.distAccSigClient.Type(), sigClientDeleteKeyErr)
			} else {
				log.Ctx(ctx).Error(sigClientDeleteKeyErrMsg)
				updateTenantErrMsg = fmt.Errorf("%w. %w", updateTenantErrMsg, sigClientDeleteKeyErrMsg)
			}
		}

		return nil, updateTenantErrMsg
	}

	return t, nil
}

func (m *Manager) RunMigrationsForTenant(
	ctx context.Context, t *tenant.Tenant, dbURL string,
	dir migrate.MigrationDirection, count int,
	migrationFiles embed.FS, migrationTableName db.MigrationTableName,
) error {
	n, err := db.Migrate(dbURL, dir, count, migrationFiles, migrationTableName)
	if err != nil {
		return fmt.Errorf("applying SDP migrations: %w", err)
	}
	log.Infof("successful applied %d migrations", n)
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

func WithTenantManager(tenantManager *tenant.Manager) Option {
	return func(m *Manager) {
		m.tenantManager = tenantManager
	}
}

func WithDistributionAccountSignatureClient(distAccSigClient signing.SignatureClient) Option {
	return func(m *Manager) {
		m.distAccSigClient = distAccSigClient
	}
}
