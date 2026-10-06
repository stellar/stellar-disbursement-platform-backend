package provisioning

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_Manager_syncDefaultDistributionWallet(t *testing.T) {
	dbt := dbtest.OpenWithAdminMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Tenant row + migrated tenant schema (sdp_sync-me), through the production fixtures.
	tnt := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "sync-me", "GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL")
	tenant.PrepareDBForTenant(t, dbt, tnt.Name)

	m := Manager{tenantManager: tenant.NewManager(tenant.WithDatabase(dbConnectionPool))}

	readDefault := func() (id string, address *string, accountType, accountStatus string) {
		row := dbConnectionPool.QueryRowxContext(ctx, `
			SELECT id, distribution_account_address, distribution_account_type, distribution_account_status
			FROM "sdp_sync-me".distribution_wallets WHERE is_default`)
		require.NoError(t, row.Scan(&id, &address, &accountType, &accountStatus))
		return id, address, accountType, accountStatus
	}

	t.Run("creates the default wallet when the backfill could not seed it", func(t *testing.T) {
		addr := "GDIVVKL6QYF6C6K3C5PZZBQ2NQDLN2OSLMVIEQRHS6DZE7WRL33ZDNXL"
		tnt.DistributionAccountAddress = &addr
		tnt.DistributionAccountType = schema.DistributionAccountStellarDBVault
		tnt.DistributionAccountStatus = schema.AccountStatusActive

		require.NoError(t, m.syncDefaultDistributionWallet(ctx, *tnt))

		_, gotAddr, gotType, gotStatus := readDefault()
		require.NotNil(t, gotAddr)
		assert.Equal(t, addr, *gotAddr)
		assert.Equal(t, string(schema.DistributionAccountStellarDBVault), gotType)
		assert.Equal(t, string(schema.AccountStatusActive), gotStatus)
	})

	t.Run("updates the existing default wallet on re-sync", func(t *testing.T) {
		beforeID, _, _, _ := readDefault()

		tnt.DistributionAccountType = schema.DistributionAccountCircleDBVault
		tnt.DistributionAccountStatus = schema.AccountStatusPendingUserActivation
		tnt.DistributionAccountAddress = nil

		require.NoError(t, m.syncDefaultDistributionWallet(ctx, *tnt))

		afterID, gotAddr, gotType, gotStatus := readDefault()
		assert.Equal(t, beforeID, afterID, "must update in place, not create a second default")
		assert.Nil(t, gotAddr, "non-Stellar account types must not carry a Stellar address")
		assert.Equal(t, string(schema.DistributionAccountCircleDBVault), gotType)
		assert.Equal(t, string(schema.AccountStatusPendingUserActivation), gotStatus)
	})
}
