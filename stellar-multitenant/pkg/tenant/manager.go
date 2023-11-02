package tenant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

var (
	ErrTenantDoesNotExist   = errors.New("tenant does not exist")
	ErrDuplicatedTenantName = errors.New("duplicated tenant name")
	ErrEmptyTenantName      = errors.New("tenant name cannot be empty")
)

type Manager struct {
	db db.DBConnectionPool
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
