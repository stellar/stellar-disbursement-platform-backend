package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_WalletMembershipModel(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := WalletMembershipModel{dbConnectionPool: dbConnectionPool}

	// Fixtures: two users, two active wallets + one archived wallet.
	newUser := func(email string) string {
		var id string
		err := dbConnectionPool.GetContext(ctx, &id, `
			INSERT INTO auth_users (encrypted_password, email, first_name, last_name, roles)
			VALUES ('x', $1, 'F', 'L', ARRAY['initiator']) RETURNING id`, email)
		require.NoError(t, err)
		return id
	}
	userAlice := newUser("alice@example.com")
	userBob := newUser("bob@example.com")

	defaultWallet := EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletB, archivedWallet string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletB, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('membership-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))
	require.NoError(t, dbConnectionPool.GetContext(ctx, &archivedWallet, `
		INSERT INTO distribution_wallets (name, distribution_account_type, status, archived_at)
		VALUES ('membership-wallet-archived', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', 'ARCHIVED', NOW()) RETURNING id`))

	t.Run("grant + read-back round trip", func(t *testing.T) {
		granted, gErr := m.Insert(ctx, dbConnectionPool, userAlice, defaultWallet.ID, InitiatorUserRole, &userBob)
		require.NoError(t, gErr)
		assert.Equal(t, InitiatorUserRole, granted.Role)
		require.NotNil(t, granted.GrantedBy)
		assert.Equal(t, userBob, *granted.GrantedBy)

		got, gErr := m.Get(ctx, dbConnectionPool, granted.ID)
		require.NoError(t, gErr)
		assert.Equal(t, granted.ID, got.ID)

		has, gErr := m.HasRoleOnWallet(ctx, dbConnectionPool, userAlice, defaultWallet.ID, InitiatorUserRole)
		require.NoError(t, gErr)
		assert.True(t, has)

		has, gErr = m.HasRoleOnWallet(ctx, dbConnectionPool, userAlice, walletB, InitiatorUserRole)
		require.NoError(t, gErr)
		assert.False(t, has, "membership on wallet A must not grant wallet B")

		walletIDs, gErr := m.GetWalletIDsForUser(ctx, dbConnectionPool, userAlice)
		require.NoError(t, gErr)
		assert.Equal(t, []string{defaultWallet.ID}, walletIDs)
	})

	t.Run("duplicate grant maps to ErrRecordAlreadyExists", func(t *testing.T) {
		_, gErr := m.Insert(ctx, dbConnectionPool, userAlice, defaultWallet.ID, InitiatorUserRole, nil)
		require.ErrorIs(t, gErr, ErrRecordAlreadyExists)
	})

	t.Run("owner role can never be wallet-scoped", func(t *testing.T) {
		_, gErr := m.Insert(ctx, dbConnectionPool, userAlice, defaultWallet.ID, OwnerUserRole, nil)
		require.Error(t, gErr)
		assert.ErrorContains(t, gErr, "tenant-wide")

		// And the DB CHECK independently rejects it.
		_, dbErr := dbConnectionPool.ExecContext(ctx, `
			INSERT INTO wallet_memberships (user_id, wallet_id, role) VALUES ($1, $2, 'owner')`,
			userAlice, defaultWallet.ID)
		require.Error(t, dbErr)
	})

	t.Run("grants on archived wallets are rejected (API maps to 409)", func(t *testing.T) {
		_, gErr := m.Insert(ctx, dbConnectionPool, userAlice, archivedWallet, ApproverUserRole, nil)
		require.ErrorIs(t, gErr, ErrWalletArchivedForMembership)
	})

	t.Run("memberships persist through wallet archival", func(t *testing.T) {
		granted, gErr := m.Insert(ctx, dbConnectionPool, userBob, walletB, ApproverUserRole, nil)
		require.NoError(t, gErr)

		// Need a second active wallet for B to be archivable (zero-active invariant).
		_, aErr := dbConnectionPool.ExecContext(ctx, `
			UPDATE distribution_wallets SET status = 'ARCHIVED', archived_at = NOW() WHERE id = $1`, walletB)
		require.NoError(t, aErr)

		// Existing membership still queryable for historical authorization.
		got, gErr := m.Get(ctx, dbConnectionPool, granted.ID)
		require.NoError(t, gErr)
		assert.Equal(t, walletB, got.WalletID)

		byWallet, gErr := m.ListByWallet(ctx, dbConnectionPool, walletB)
		require.NoError(t, gErr)
		require.Len(t, byWallet, 1)

		// But NEW grants on the now-archived wallet fail.
		_, gErr = m.Insert(ctx, dbConnectionPool, userAlice, walletB, ApproverUserRole, nil)
		require.ErrorIs(t, gErr, ErrWalletArchivedForMembership)
	})

	t.Run("revoke removes the row; audit keeps full history append-only", func(t *testing.T) {
		granted, gErr := m.Insert(ctx, dbConnectionPool, userBob, defaultWallet.ID, FinancialControllerUserRole, nil)
		require.NoError(t, gErr)
		require.NoError(t, m.Delete(ctx, dbConnectionPool, granted.ID))

		_, gErr = m.Get(ctx, dbConnectionPool, granted.ID)
		require.ErrorIs(t, gErr, ErrRecordNotFound)

		// Audit trail has INSERT + DELETE entries for the membership.
		var auditOps []string
		require.NoError(t, dbConnectionPool.SelectContext(ctx, &auditOps, `
			SELECT operation FROM wallet_memberships_audit WHERE id = $1 ORDER BY changed_at`, granted.ID))
		assert.Equal(t, []string{"INSERT", "DELETE"}, auditOps)

		// Append-only: UPDATE and DELETE on audit rows are rejected.
		_, dbErr := dbConnectionPool.ExecContext(ctx, `
			UPDATE wallet_memberships_audit SET operation = 'TAMPERED' WHERE id = $1`, granted.ID)
		require.Error(t, dbErr)
		assert.ErrorContains(t, dbErr, "append-only")

		_, dbErr = dbConnectionPool.ExecContext(ctx, `
			DELETE FROM wallet_memberships_audit WHERE id = $1`, granted.ID)
		require.Error(t, dbErr)
		assert.ErrorContains(t, dbErr, "append-only")

		// Revoking a missing membership maps to not-found.
		require.ErrorIs(t, m.Delete(ctx, dbConnectionPool, granted.ID), ErrRecordNotFound)
	})

	t.Run("ListAuditByWallet returns the wallet's grant/revoke history, newest first", func(t *testing.T) {
		// The previous subtest granted+revoked Bob on the default wallet, so its audit trail
		// already has entries; add one more grant (a role Alice doesn't hold yet on this
		// wallet) so ordering is observable.
		granted, gErr := m.Insert(ctx, dbConnectionPool, userAlice, defaultWallet.ID, ApproverUserRole, nil)
		require.NoError(t, gErr)

		entries, aErr := m.ListAuditByWallet(ctx, dbConnectionPool, defaultWallet.ID, 100)
		require.NoError(t, aErr)
		require.GreaterOrEqual(t, len(entries), 3, "INSERT+DELETE from the revoke subtest and this grant's INSERT")

		// Newest first, wallet-scoped, and the latest entry is this grant.
		for i := 1; i < len(entries); i++ {
			assert.False(t, entries[i-1].ChangedAt.Before(entries[i].ChangedAt))
		}
		for _, e := range entries {
			assert.Equal(t, defaultWallet.ID, e.WalletID)
		}
		assert.Equal(t, granted.ID, entries[0].ID)
		assert.Equal(t, "INSERT", entries[0].Operation)

		// Another wallet's audit history is not mixed in (walletB only ever saw Bob's grant).
		otherEntries, aErr := m.ListAuditByWallet(ctx, dbConnectionPool, walletB, 100)
		require.NoError(t, aErr)
		for _, e := range otherEntries {
			assert.Equal(t, walletB, e.WalletID)
		}

		// The limit caps the read.
		capped, aErr := m.ListAuditByWallet(ctx, dbConnectionPool, defaultWallet.ID, 1)
		require.NoError(t, aErr)
		require.Len(t, capped, 1)
	})

	t.Run("wallet referenced by memberships cannot be hard-deleted", func(t *testing.T) {
		_, dErr := dbConnectionPool.ExecContext(ctx, `DELETE FROM distribution_wallets WHERE id = $1`, walletB)
		require.Error(t, dErr)
		assert.ErrorContains(t, dErr, "wallet_memberships")
	})
}
