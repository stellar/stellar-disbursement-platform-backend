package provisioning

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

//go:generate mockery --name=TenantProvisioningService --case=underscore --structname=MockTenantProvisioningServiceInterface --inpackage --filename=mocks.go
type TenantProvisioningService interface {
	ProvisionNewTenant(ctx context.Context, pt ProvisionTenant) (*schema.Tenant, error)
}

type ProvisionTenant struct {
	Name                    string
	UserFirstName           string
	UserLastName            string
	UserEmail               string
	OrgName                 string
	UiBaseURL               string
	BaseURL                 string
	NetworkType             string
	DistributionAccountType schema.AccountType
}

type ManagerOptions struct {
	DBConnectionPool           db.DBConnectionPool
	TenantManager              tenant.ManagerInterface
	SubmitterEngine            engine.SubmitterEngine
	NativeAssetBootstrapAmount int
}

// Manager provides a public API for tenant provisioning
type Manager struct {
	internalManager *provisioning.Manager
}

func NewManager(opts ManagerOptions) (TenantProvisioningService, error) {
	internalOpts := provisioning.ManagerOptions{
		DBConnectionPool:           opts.DBConnectionPool,
		TenantManager:              opts.TenantManager,
		SubmitterEngine:            opts.SubmitterEngine,
		NativeAssetBootstrapAmount: opts.NativeAssetBootstrapAmount,
	}

	internalManager, err := provisioning.NewManager(internalOpts)
	if err != nil {
		return nil, err
	}

	return &Manager{
		internalManager: internalManager,
	}, nil
}

func (m *Manager) ProvisionNewTenant(ctx context.Context, pt ProvisionTenant) (*schema.Tenant, error) {
	internalPT := provisioning.ProvisionTenant{
		Name:                    pt.Name,
		UserFirstName:           pt.UserFirstName,
		UserLastName:            pt.UserLastName,
		UserEmail:               pt.UserEmail,
		OrgName:                 pt.OrgName,
		UiBaseURL:               pt.UiBaseURL,
		BaseURL:                 pt.BaseURL,
		NetworkType:             pt.NetworkType,
		DistributionAccountType: pt.DistributionAccountType,
	}

	return m.internalManager.ProvisionNewTenant(ctx, internalPT)
}
