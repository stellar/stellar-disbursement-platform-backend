package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_DistributionWalletModel(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := DistributionWalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("Insert validates input", func(t *testing.T) {
		_, iErr := m.Insert(ctx, dbConnectionPool, DistributionWalletInsert{})
		require.ErrorIs(t, iErr, ErrMissingInput)

		_, iErr = m.Insert(ctx, dbConnectionPool, DistributionWalletInsert{
			Name:        "channel-type",
			AccountType: schema.ChannelAccountStellarDB,
		})
		require.ErrorContains(t, iErr, "not a distribution account type")
	})

	t.Run("Insert + Get + GetByName round-trip", func(t *testing.T) {
		desc := "program two"
		created, iErr := m.Insert(ctx, dbConnectionPool, DistributionWalletInsert{
			Name:        "program-2",
			Description: &desc,
			AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.NoError(t, iErr)
		assert.NotEmpty(t, created.ID)
		assert.Equal(t, PendingDistributionWalletStatus, created.Status, "not usable until Activate() succeeds")
		assert.False(t, created.IsDefault)
		assert.Nil(t, created.Address)

		got, gErr := m.Get(ctx, dbConnectionPool, created.ID)
		require.NoError(t, gErr)
		assert.Equal(t, created.ID, got.ID)

		got, gErr = m.GetByName(ctx, dbConnectionPool, "program-2")
		require.NoError(t, gErr)
		assert.Equal(t, created.ID, got.ID)

		_, gErr = m.Get(ctx, dbConnectionPool, "c4866b67-a850-42b2-a2a8-d993cf33b352")
		require.ErrorIs(t, gErr, ErrRecordNotFound)
	})

	t.Run("Insert duplicate name maps to ErrRecordAlreadyExists", func(t *testing.T) {
		_, iErr := m.Insert(ctx, dbConnectionPool, DistributionWalletInsert{
			Name:        "program-2",
			AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.ErrorIs(t, iErr, ErrRecordAlreadyExists)
	})

	t.Run("Activate only succeeds on a PENDING wallet", func(t *testing.T) {
		created, iErr := m.Insert(ctx, dbConnectionPool, DistributionWalletInsert{
			Name:        "program-activate",
			AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.NoError(t, iErr)
		require.Equal(t, PendingDistributionWalletStatus, created.Status)

		activated, aErr := m.Activate(ctx, dbConnectionPool, created.ID)
		require.NoError(t, aErr)
		assert.Equal(t, ActiveDistributionWalletStatus, activated.Status)

		// Not re-activatable: it's already ACTIVE, not PENDING.
		_, aErr = m.Activate(ctx, dbConnectionPool, created.ID)
		require.ErrorIs(t, aErr, ErrRecordNotFound)
	})

	t.Run("UpdateAddress sets the address exactly once", func(t *testing.T) {
		created, iErr := m.Insert(ctx, dbConnectionPool, DistributionWalletInsert{
			Name:        "program-3",
			AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.NoError(t, iErr)

		const addr = "GAAAPROGRAM3AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		updated, uErr := m.UpdateAddress(ctx, dbConnectionPool, created.ID, addr)
		require.NoError(t, uErr)
		require.NotNil(t, updated.Address)
		assert.Equal(t, addr, *updated.Address)

		// Address is immutable once set.
		_, uErr = m.UpdateAddress(ctx, dbConnectionPool, created.ID, "GBBBOTHERADDRESSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		require.ErrorIs(t, uErr, ErrRecordNotFound)
	})

	t.Run("GetDefault and EnsureDefaultWallet upsert behavior", func(t *testing.T) {
		// No default yet.
		_, gErr := m.GetDefault(ctx, dbConnectionPool)
		require.ErrorIs(t, gErr, ErrRecordNotFound)

		// Ensure with no existing default → INSERT path.
		addr := "GCCCDEFAULTADDRESSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		ensured, eErr := m.EnsureDefaultWallet(ctx, dbConnectionPool, &addr, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive)
		require.NoError(t, eErr)
		assert.Equal(t, DefaultDistributionWalletName, ensured.Name)
		assert.True(t, ensured.IsDefault)
		require.NotNil(t, ensured.Address)
		assert.Equal(t, addr, *ensured.Address)

		// Ensure again with new material → UPDATE path, same row.
		addr2 := "GDDDROTATEDADDRESSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		ensured2, eErr := m.EnsureDefaultWallet(ctx, dbConnectionPool, &addr2, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive)
		require.NoError(t, eErr)
		assert.Equal(t, ensured.ID, ensured2.ID, "must update the existing default, not create another")
		require.NotNil(t, ensured2.Address)
		assert.Equal(t, addr2, *ensured2.Address)

		got, gErr := m.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, gErr)
		assert.Equal(t, ensured.ID, got.ID)

		// Rejects non-distribution account types.
		_, eErr = m.EnsureDefaultWallet(ctx, dbConnectionPool, &addr, schema.ChannelAccountStellarDB, schema.AccountStatusActive)
		require.ErrorContains(t, eErr, "not a distribution account type")
	})

	t.Run("GetAll hides archived unless asked; Count includes archived", func(t *testing.T) {
		// program-2 is still PENDING (never activated) — activate it so this subtest's actual
		// target (archived-exclusion) isn't conflated with pending-exclusion, which has its own
		// coverage above.
		program2, gErr := m.GetByName(ctx, dbConnectionPool, "program-2")
		require.NoError(t, gErr)
		_, aErr := m.Activate(ctx, dbConnectionPool, program2.ID)
		require.NoError(t, aErr)

		// Archive program-3 directly (service-level archive lands in).
		_, aErr = dbConnectionPool.ExecContext(ctx, `
			UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE name = 'program-3'`)
		require.NoError(t, aErr)

		operational, gErr := m.GetAll(ctx, dbConnectionPool, false)
		require.NoError(t, gErr)
		names := make([]string, 0, len(operational))
		for _, w := range operational {
			names = append(names, w.Name)
		}
		assert.NotContains(t, names, "program-3")
		assert.Contains(t, names, DefaultDistributionWalletName)
		assert.Equal(t, DefaultDistributionWalletName, operational[0].Name, "default sorts first")

		all, gErr := m.GetAll(ctx, dbConnectionPool, true)
		require.NoError(t, gErr)
		assert.Len(t, all, len(operational)+1)

		count, cErr := m.Count(ctx, dbConnectionPool)
		require.NoError(t, cErr)
		assert.Equal(t, len(all), count, "archived wallets still occupy a cap slot")
	})
}
