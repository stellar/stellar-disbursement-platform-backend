package tenant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"
)

var (
	ErrTenantDoesNotExist      = errors.New("tenant does not exist")
	ErrDuplicatedTenantName    = errors.New("duplicated tenant name")
	ErrEmptyTenantName         = errors.New("tenant name cannot be empty")
	ErrEmptyUpdateTenant       = errors.New("provide at least one field to be updated")
	ErrTenantNotFoundInContext = errors.New("tenant not found in context")
)

type tenantContextKey struct{}

type ManagerInterface interface {
	GetDSNForTenant(ctx context.Context, tenantName string) (string, error)
	GetAllTenants(ctx context.Context) ([]Tenant, error)
	GetTenantByID(ctx context.Context, id string) (*Tenant, error)
	GetTenantByName(ctx context.Context, name string) (*Tenant, error)
	GetTenantByIDOrName(ctx context.Context, arg string) (*Tenant, error)
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

	return router.GetDSNForTenant(dataSourceName, tenantName)
}

var selectQuery string = `
	SELECT 
		*
	FROM
		tenants t
	%s
`

// GetAllTenants returns all tenants in the database.
func (m *Manager) GetAllTenants(ctx context.Context) ([]Tenant, error) {
	tnts := []Tenant{}

	query := fmt.Sprintf(selectQuery, "ORDER BY t.name ASC")

	err := m.db.SelectContext(ctx, &tnts, query)
	if err != nil {
		return nil, fmt.Errorf("getting all tenants: %w", err)
	}

	return tnts, nil
}

func (m *Manager) GetTenantByID(ctx context.Context, id string) (*Tenant, error) {
	q := fmt.Sprintf(selectQuery, "WHERE t.id = $1")
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
	q := fmt.Sprintf(selectQuery, "WHERE t.name = $1")
	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("getting tenant %s: %w", name, err)
	}
	return &t, nil
}

// GetTenantByIDOrName returns the tenant with a given id or name.
func (m *Manager) GetTenantByIDOrName(ctx context.Context, arg string) (*Tenant, error) {
	var tnt Tenant
	query := fmt.Sprintf(selectQuery, "WHERE t.id = $1 OR t.name = $1")

	err := m.db.GetContext(ctx, &tnt, query, arg)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantDoesNotExist
		}
		return nil, fmt.Errorf("getting tenant %s: %w", arg, err)
	}

	return &tnt, nil
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
func GetTenantFromContext(ctx context.Context) (*Tenant, error) {
	currentTenant, ok := ctx.Value(tenantContextKey{}).(*Tenant)
	if !ok {
		return nil, ErrTenantNotFoundInContext
	}
	return currentTenant, nil
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
