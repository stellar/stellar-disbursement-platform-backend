package tenant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

var (
	ErrTenantDoesNotExist   = errors.New("tenant does not exist")
	ErrDuplicatedTenantName = errors.New("duplicated tenant name")
	ErrEmptyTenantName      = errors.New("tenant name cannot be empty")
)

type tenantContextKey struct{}

type ManagerInterface interface {
	GetDSNForTenant(ctx context.Context, tenantName string) (string, error)
	GetTenantByID(ctx context.Context, id string) (*Tenant, error)
	GetTenantByName(ctx context.Context, name string) (*Tenant, error)
	AddTenant(ctx context.Context, name string) (*Tenant, error)
	UpdateTenantConfig(ctx context.Context, tu *TenantUpdate) (*Tenant, error)
}

type Manager struct {
	db db.DBConnectionPool
}

func (m *Manager) GetDSNForTenant(ctx context.Context, tenantName string) (string, error) {
	dataSourceName, err := m.db.DSN(ctx)
	if err != nil {
		return "", fmt.Errorf("getting database DSN: %w", err)
	}
	u, err := url.Parse(dataSourceName)
	if err != nil {
		return "", fmt.Errorf("parsing database DSN: %w", err)
	}
	q := u.Query()
	schemaName := fmt.Sprintf("sdp_%s", tenantName)
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (m *Manager) GetTenantByID(ctx context.Context, id string) (*Tenant, error) {
	const q = "SELECT * FROM tenants WHERE id = $1"
	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("getting tenant %s: %w", id, err)
	}
	return &t, nil
}

func (m *Manager) GetTenantByName(ctx context.Context, name string) (*Tenant, error) {
	const q = "SELECT * FROM tenants WHERE name = $1"
	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("getting tenant %s: %w", name, err)
	}
	return &t, nil
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

// GetTenantFromContext retrieves the tenant information from the context.
func GetTenantFromContext(ctx context.Context) (*Tenant, bool) {
	currentTenant, ok := ctx.Value(tenantContextKey{}).(*Tenant)
	return currentTenant, ok
}

// SaveTenantInContext stores the tenant information in the context.
func SaveTenantInContext(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, t)
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

var _ ManagerInterface = (*Manager)(nil)
