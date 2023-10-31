package tenant

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

var (
	ErrTenantDoesNotExist   = errors.New("tenant does not exist")
	ErrDuplicatedTenantName = errors.New("duplicated tenant name")
	ErrEmptyTenantName      = errors.New("tenant name cannot be empty")
)

type Manager struct {
	db db.DBConnectionPool
}

func (m *Manager) ProvisionNewTenant(ctx context.Context, name, userFirstName, userLastName, userEmail, networkType string) (*Tenant, error) {
	log.Infof("adding tenant %s", name)
	t, err := m.AddTenant(ctx, name)
	if err != nil {
		return nil, err
	}

	log.Infof("creating tenant %s database schema", t.Name)
	schemaName := fmt.Sprintf("sdp_%s", t.Name)
	_, err = m.db.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", pq.QuoteIdentifier(schemaName)))
	if err != nil {
		return nil, fmt.Errorf("creating a new database schema: %w", err)
	}

	dataSourceName := m.db.DSN()
	u, err := url.Parse(dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("parsing database DSN: %w", err)
	}
	q := u.Query()
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()

	// Applying migrations
	log.Infof("applying SDP migrations on the tenant %s schema", t.Name)
	err = m.RunMigrationForTenant(ctx, t, u.String(), migrate.Up, 0, sdpmigrations.FS, db.StellarSDPMigrationsTableName)
	if err != nil {
		return nil, fmt.Errorf("applying SDP migrations: %w", err)
	}

	log.Infof("applying stellar-auth migrations on the tenant %s schema", t.Name)
	err = m.RunMigrationForTenant(ctx, t, u.String(), migrate.Up, 0, authmigrations.FS, db.StellarAuthMigrationsTableName)
	if err != nil {
		return nil, fmt.Errorf("applying stellar-auth migrations: %w", err)
	}

	// Connecting to the tenant database schema
	tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
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

	// TODO: send invitation email to this new user
	authManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(tenantSchemaConnectionPool, auth.NewDefaultPasswordEncrypter(), 0),
	)
	_, err = authManager.CreateUser(ctx, &auth.User{
		FirstName: userFirstName,
		LastName:  userLastName,
		Email:     userEmail,
		IsOwner:   true,
		Roles:     []string{"owner"},
	}, "")
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	tenantStatus := ProvisionedTenantStatus
	t, err = m.UpdateTenantConfig(ctx, &TenantUpdate{ID: t.ID, Status: &tenantStatus})
	if err != nil {
		return nil, fmt.Errorf("updating tenant %s status to %s: %w", name, ProvisionedTenantStatus, err)
	}

	return t, nil
}

func (m *Manager) AddTenant(ctx context.Context, name string) (*Tenant, error) {
	if name == "" {
		return nil, ErrEmptyTenantName
	}

	const q = "INSERT INTO tenants (name) VALUES ($1) RETURNING *"
	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, name); err != nil {
		if pqError, ok := err.(*pq.Error); ok && pqError.Constraint == "idx_unique_name" {
			return nil, ErrDuplicatedTenantName
		}
		return nil, fmt.Errorf("inserting tenant %s: %w", name, err)
	}
	return &t, nil
}

func (m *Manager) UpdateTenantConfig(ctx context.Context, tu *TenantUpdate) (*Tenant, error) {
	if tu == nil {
		return nil, fmt.Errorf("tenant update cannot be nil")
	}

	if err := tu.Validate(); err != nil {
		return nil, err
	}

	q := `
		UPDATE tenants
		SET
			%s
		WHERE
			id = ?
		RETURNING *
	`

	fields := make([]string, 0)
	args := make([]interface{}, 0)
	if tu.EmailSenderType != nil {
		fields = append(fields, "email_sender_type = ?")
		args = append(args, *tu.EmailSenderType)
	}

	if tu.SMSSenderType != nil {
		fields = append(fields, "sms_sender_type = ?")
		args = append(args, *tu.SMSSenderType)
	}

	if tu.SEP10SigningPublicKey != nil {
		fields = append(fields, "sep10_signing_public_key = ?")
		args = append(args, *tu.SEP10SigningPublicKey)
	}

	if tu.DistributionPublicKey != nil {
		fields = append(fields, "distribution_public_key = ?")
		args = append(args, *tu.DistributionPublicKey)
	}

	if tu.EnableMFA != nil {
		fields = append(fields, "enable_mfa = ?")
		args = append(args, *tu.EnableMFA)
	}

	if tu.EnableReCAPTCHA != nil {
		fields = append(fields, "enable_recaptcha = ?")
		args = append(args, *tu.EnableReCAPTCHA)
	}

	if tu.BaseURL != nil {
		fields = append(fields, "base_url = ?")
		args = append(args, *tu.BaseURL)
	}

	if tu.SDPUIBaseURL != nil {
		fields = append(fields, "sdp_ui_base_url = ?")
		args = append(args, *tu.SDPUIBaseURL)
	}

	if tu.CORSAllowedOrigins != nil && len(tu.CORSAllowedOrigins) > 0 {
		fields = append(fields, "cors_allowed_origins = ?")
		args = append(args, pq.Array(tu.CORSAllowedOrigins))
	}

	if tu.Status != nil {
		fields = append(fields, "status = ?")
		args = append(args, *tu.Status)
	}

	args = append(args, tu.ID)
	q = fmt.Sprintf(q, strings.Join(fields, ",\n"))
	q = m.db.Rebind(q)

	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("updating tenant ID %s: %w", tu.ID, err)
	}

	return &t, nil
}

func (m *Manager) RunMigrationForTenant(
	ctx context.Context, t *Tenant, dbURL string,
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
