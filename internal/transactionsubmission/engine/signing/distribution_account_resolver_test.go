package signing

import (
	"context"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_DistributionAccountResolverOptions_Validate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	testCases := []struct {
		name            string
		config          DistributionAccountResolverOptions
		wantErrContains string
	}{
		{
			name: "return an error if AdminDBConnectionPool is nil",
			config: DistributionAccountResolverOptions{
				AdminDBConnectionPool: nil,
			},
			wantErrContains: "AdminDBConnectionPool cannot be nil",
		},
		{
			name: "return an error if HostDistributionAccountPublicKey is empty",
			config: DistributionAccountResolverOptions{
				AdminDBConnectionPool:            dbConnectionPool,
				HostDistributionAccountPublicKey: "",
			},
			wantErrContains: "HostDistributionAccountPublicKey cannot be empty",
		},
		{
			name: "return an error if HostDistributionAccountPublicKey is not a valid ed25519 public key",
			config: DistributionAccountResolverOptions{
				AdminDBConnectionPool:            dbConnectionPool,
				HostDistributionAccountPublicKey: "not-a-valid-ed25519-public-key",
			},
			wantErrContains: "HostDistributionAccountPublicKey is not a valid ed25519 public key",
		},
		{
			name: "🎉 successfully validate the config",
			config: DistributionAccountResolverOptions{
				AdminDBConnectionPool:            dbConnectionPool,
				HostDistributionAccountPublicKey: keypair.MustRandom().Address(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_NewDistributionAccountResolver(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	hostDistPublicKey := keypair.MustRandom().Address()

	testCases := []struct {
		name            string
		config          DistributionAccountResolverOptions
		wantErrContains string
		wantResult      DistributionAccountResolver
	}{
		{
			name: "return an error if config is invalid",
			config: DistributionAccountResolverOptions{
				AdminDBConnectionPool:            nil,
				MTNDBConnectionPool:              nil,
				HostDistributionAccountPublicKey: "",
			},
			wantErrContains: "validating config in NewDistributionAccountResolver: AdminDBConnectionPool cannot be nil",
		},
		{
			name: "🎉 successfully create a new DistributionAccountResolver",
			config: DistributionAccountResolverOptions{
				AdminDBConnectionPool:            dbConnectionPool,
				MTNDBConnectionPool:              dbConnectionPool,
				HostDistributionAccountPublicKey: hostDistPublicKey,
			},
			wantResult: &DistributionAccountResolverImpl{
				tenantManager:                 tenant.NewManager(tenant.WithDatabase(dbConnectionPool)),
				hostDistributionAccountPubKey: hostDistPublicKey,
				circleConfigModel:             circle.NewClientConfigModel(dbConnectionPool),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotResult, err := NewDistributionAccountResolver(tc.config)
			if tc.wantErrContains == "" {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, gotResult)
			} else {
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.Empty(t, tc.wantResult)
			}
		})
	}
}

func Test_DistributionAccountResolverImpl_DistributionAccount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	ctx := context.Background()

	hostDistributionAccountPubKey := keypair.MustRandom().Address()
	distAccResolver, err := NewDistributionAccountResolver(DistributionAccountResolverOptions{
		AdminDBConnectionPool:            dbConnectionPool,
		MTNDBConnectionPool:              dbConnectionPool,
		HostDistributionAccountPublicKey: hostDistributionAccountPubKey,
	})
	require.NoError(t, err)

	t.Run("return an error if the tenant_id cannot be found in the DB", func(t *testing.T) {
		distAccount, err := distAccResolver.DistributionAccount(ctx, "tenant-id-not-found")
		assert.ErrorContains(t, err, "getting tenant")
		assert.ErrorIs(t, err, tenant.ErrTenantDoesNotExist)
		assert.Empty(t, distAccount)
	})

	t.Run("return an error if the tenant exists but its distribution account is empty", func(t *testing.T) {
		defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := m.AddTenant(ctx, "myorg1")
		require.NoError(t, err)
		assert.NotEmpty(t, tnt.ID)

		distAccount, err := distAccResolver.DistributionAccount(ctx, tnt.ID)
		assert.Empty(t, distAccount)
		assert.ErrorIs(t, err, ErrDistributionAccountIsEmpty)
	})

	t.Run("successfully return the distribution account from the tenant ID provided", func(t *testing.T) {
		defer tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		tnt, err := m.AddTenant(ctx, "myorg1")
		require.NoError(t, err)

		distributionPublicKey := keypair.MustRandom().Address()
		tnt, err = m.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
			ID:                         tnt.ID,
			DistributionAccountAddress: distributionPublicKey,
			DistributionAccountType:    schema.DistributionAccountStellarDBVault,
			DistributionAccountStatus:  schema.AccountStatusActive,
		})
		require.NoError(t, err)

		distAccount, err := distAccResolver.DistributionAccount(ctx, tnt.ID)
		assert.NoError(t, err)
		assert.Equal(t, schema.TransactionAccount{
			Address: distributionPublicKey,
			Type:    schema.DistributionAccountStellarDBVault,
			Status:  schema.AccountStatusActive,
		}, distAccount)
	})
}

func Test_DistributionAccountResolverImpl_DistributionAccountFromContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	hostDistributionAccountPubKey := keypair.MustRandom().Address()
	distAccResolver, err := NewDistributionAccountResolver(DistributionAccountResolverOptions{
		AdminDBConnectionPool:            dbConnectionPool,
		MTNDBConnectionPool:              dbConnectionPool,
		HostDistributionAccountPublicKey: hostDistributionAccountPubKey,
	})
	require.NoError(t, err)

	t.Run("return an error if there's no tenant in the context", func(t *testing.T) {
		distAccount, err := distAccResolver.DistributionAccountFromContext(context.Background())
		assert.ErrorContains(t, err, "getting tenant")
		assert.ErrorIs(t, err, tenant.ErrTenantNotFoundInContext)
		assert.Empty(t, distAccount)
	})

	t.Run("return an error if the tenant exists in the context but its distribution account is empty", func(t *testing.T) {
		tnt := &schema.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e44", Name: "aid-org-1"}
		ctxWithTenant := tenant.SaveTenantInContext(context.Background(), tnt)

		distAccount, err := distAccResolver.DistributionAccountFromContext(ctxWithTenant)
		assert.Empty(t, distAccount)
		assert.ErrorIs(t, err, ErrDistributionAccountIsEmpty)
	})

	t.Run("correctly returns the CIRCLE response after the initial setup, when there's no entry in the circleConfigModel", func(t *testing.T) {
		tnt := &schema.Tenant{
			ID:                        "95e788b6-c80e-4975-9d12-141001fe6e44",
			Name:                      "aid-org-1",
			DistributionAccountType:   schema.DistributionAccountCircleDBVault,
			DistributionAccountStatus: schema.AccountStatusPendingUserActivation,
		}
		ctxWithTenant := tenant.SaveTenantInContext(context.Background(), tnt)

		distAccount, err := distAccResolver.DistributionAccountFromContext(ctxWithTenant)
		assert.NoError(t, err)
		assert.Equal(t, schema.TransactionAccount{
			Type:   schema.DistributionAccountCircleDBVault,
			Status: schema.AccountStatusPendingUserActivation,
		}, distAccount)
	})

	t.Run("🎉 successfully returns the CIRCLE response after it's being fully configured", func(t *testing.T) {
		tnt := &schema.Tenant{
			ID:                        "95e788b6-c80e-4975-9d12-141001fe6e44",
			Name:                      "aid-org-1",
			DistributionAccountType:   schema.DistributionAccountCircleDBVault,
			DistributionAccountStatus: schema.AccountStatusActive,
		}
		ctxWithTenant := tenant.SaveTenantInContext(context.Background(), tnt)

		circleConfigModel := circle.NewClientConfigModel(dbConnectionPool)
		err := circleConfigModel.Upsert(context.Background(), circle.ClientConfigUpdate{
			EncryptedAPIKey:    utils.StringPtr("encrypted-api-key"),
			WalletID:           utils.StringPtr("wallet-id"),
			EncrypterPublicKey: utils.StringPtr("encrypter-public-key"),
		})
		require.NoError(t, err)

		distAccount, err := distAccResolver.DistributionAccountFromContext(ctxWithTenant)
		assert.NoError(t, err)
		assert.Equal(t, schema.TransactionAccount{
			CircleWalletID: "wallet-id",
			Type:           schema.DistributionAccountCircleDBVault,
			Status:         schema.AccountStatusActive,
		}, distAccount)
	})

	t.Run("🎉 successfully return the distribution account from the tenant stored in the context", func(t *testing.T) {
		distributionPublicKey := keypair.MustRandom().Address()
		ctxTenant := &schema.Tenant{
			ID:                         "95e788b6-c80e-4975-9d12-141001fe6e44",
			Name:                       "aid-org-1",
			DistributionAccountAddress: &distributionPublicKey,
			DistributionAccountType:    schema.DistributionAccountStellarEnv,
			DistributionAccountStatus:  schema.AccountStatusActive,
		}
		ctxWithTenant := tenant.SaveTenantInContext(context.Background(), ctxTenant)

		distAccount, err := distAccResolver.DistributionAccountFromContext(ctxWithTenant)
		assert.NoError(t, err)
		assert.Equal(t, schema.TransactionAccount{
			Address: distributionPublicKey,
			Type:    schema.DistributionAccountStellarEnv,
			Status:  schema.AccountStatusActive,
		}, distAccount)
	})
}

func Test_DistributionAccountResolverImpl_HostDistributionAccount(t *testing.T) {
	hostAccounts := []schema.TransactionAccount{
		schema.NewDefaultHostAccount(keypair.MustRandom().Address()),
		schema.NewDefaultHostAccount(keypair.MustRandom().Address()),
		schema.NewDefaultHostAccount(keypair.MustRandom().Address()),
	}

	for i, hostAccount := range hostAccounts {
		distAccResolver := &DistributionAccountResolverImpl{hostDistributionAccountPubKey: hostAccount.Address}
		assert.Equalf(t, hostAccount, distAccResolver.HostDistributionAccount(), "assertion failed at index %d", i)
	}
}
