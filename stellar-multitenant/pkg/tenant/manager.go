package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/db"
)

var (
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

	const q = "INSERT INTO tenants (name) VALUES ($1) RETURNING id, name"
	var t Tenant
	if err := m.db.GetContext(ctx, &t, q, name); err != nil {
		if pqError, ok := err.(*pq.Error); ok && pqError.Constraint == "idx_unique_name" {
			return nil, ErrDuplicatedTenantName
		}
		return nil, fmt.Errorf("inserting tenant %s: %w", name, err)
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
