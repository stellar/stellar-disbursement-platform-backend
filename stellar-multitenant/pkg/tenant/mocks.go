package tenant

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stretchr/testify/mock"
)

type TenantManagerMock struct {
	mock.Mock
}

func (m *TenantManagerMock) GetDSNForTenant(ctx context.Context, tenantName string) (string, error) {
	args := m.Called(ctx, tenantName)
	return args.String(0), args.Error(1)
}

func (m *TenantManagerMock) GetDSNForTenantByID(ctx context.Context, id string) (string, error) {
	args := m.Called(ctx, id)
	return args.String(0), args.Error(1)
}

func (m *TenantManagerMock) GetAllTenants(ctx context.Context, queryParams *QueryParams) ([]Tenant, error) {
	args := m.Called(ctx, queryParams)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Tenant), args.Error(1)
}

func (m *TenantManagerMock) GetTenant(ctx context.Context, queryParams *QueryParams) (*Tenant, error) {
	args := m.Called(ctx, queryParams)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

func (m *TenantManagerMock) GetTenantByName(ctx context.Context, name string) (*Tenant, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

func (m *TenantManagerMock) GetTenantByID(ctx context.Context, id string) (*Tenant, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

func (m *TenantManagerMock) GetTenantByIDOrName(ctx context.Context, arg string) (*Tenant, error) {
	args := m.Called(ctx, arg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

func (m *TenantManagerMock) GetDefault(ctx context.Context) (*Tenant, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

func (m *TenantManagerMock) SetDefault(ctx context.Context, sqlExec db.SQLExecuter, id string) (*Tenant, error) {
	args := m.Called(ctx, sqlExec, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

func (m *TenantManagerMock) AddTenant(ctx context.Context, name string) (*Tenant, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

func (m *TenantManagerMock) DeleteTenantByName(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return args.Error(0)
	}
	return args.Error(0)
}

func (m *TenantManagerMock) CreateTenantSchema(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return args.Error(0)
	}
	return args.Error(0)
}

func (m *TenantManagerMock) DropTenantSchema(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return args.Error(0)
	}
	return args.Error(0)
}

func (m *TenantManagerMock) UpdateTenantConfig(ctx context.Context, tu *TenantUpdate) (*Tenant, error) {
	args := m.Called(ctx, tu)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Tenant), args.Error(1)
}

var _ ManagerInterface = (*TenantManagerMock)(nil)
