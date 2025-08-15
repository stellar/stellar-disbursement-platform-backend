package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	prov "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant/provisioning"
)

func TestEnsureDefaultTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	pool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	hc := &horizonclient.MockClient{}
	lt := preconditionsMocks.NewMockLedgerNumberTracker(t)
	sigSvc, _, _ := signing.NewMockSignatureService(t)
	submitter := engine.SubmitterEngine{
		HorizonClient:       hc,
		SignatureService:    sigSvc,
		LedgerNumberTracker: lt,
		MaxBaseFee:          100,
	}

	opts := utils.GlobalOptionsType{
		NetworkPassphrase: network.TestNetworkPassphrase,
	}

	t.Run("AlreadyExists", func(t *testing.T) {
		tenantMock := tenant.NewTenantManagerMock(t)
		provMock := prov.NewMockTenantProvisioningServiceInterface(t)

		tenantMock.
			On("GetDefault", ctx).
			Return(&schema.Tenant{ID: "ultra-id", Name: "Ultramarines"}, nil).
			Once()

		cfg := DefaultTenantConfig{
			DefaultTenantOwnerEmail:              "calgar@ultramarines",
			DefaultTenantOwnerFirstName:          "Marneus",
			DefaultTenantOwnerLastName:           "Calgar",
			DefaultTenantDistributionAccountType: string(schema.DistributionAccountStellarDBVault),
			DistributionPublicKey:                "GULTRAMARINES",
		}

		svc := NewDefaultTenantsService(pool, provMock, submitter, tenantMock)
		err := svc.EnsureDefaultTenant(ctx, cfg, opts)
		require.NoError(t, err)

		tenantMock.AssertExpectations(t)
		provMock.AssertNotCalled(t, "ProvisionNewTenant", mock.Anything, mock.Anything)
	})

	t.Run("ProvisionNew", func(t *testing.T) {
		tenantMock := tenant.NewTenantManagerMock(t)
		provMock := prov.NewMockTenantProvisioningServiceInterface(t)

		tenantMock.
			On("GetDefault", ctx).
			Return(nil, tenant.ErrTenantDoesNotExist).
			Once()

		newTenant := &schema.Tenant{ID: "fists-id", Name: "ImperialFists"}
		provMock.
			On("ProvisionNewTenant", ctx, mock.MatchedBy(func(pt prov.ProvisionTenant) bool {
				return pt.UserEmail == "dorn@imperialfists" && pt.Name == "default"
			})).
			Return(newTenant, nil).
			Once()

		tenantMock.
			On("SetDefault", ctx, pool, "fists-id").
			Return(newTenant, nil).
			Once()

		cfg := DefaultTenantConfig{
			DefaultTenantOwnerEmail:              "dorn@imperialfists",
			DefaultTenantOwnerFirstName:          "Rogal",
			DefaultTenantOwnerLastName:           "Dorn",
			DefaultTenantDistributionAccountType: string(schema.DistributionAccountStellarDBVault),
			DistributionPublicKey:                "GIMPERIALFISTS",
		}

		svc := NewDefaultTenantsService(pool, provMock, submitter, tenantMock)
		err := svc.EnsureDefaultTenant(ctx, cfg, opts)
		require.NoError(t, err)

		tenantMock.AssertExpectations(t)
		provMock.AssertExpectations(t)
	})
}
