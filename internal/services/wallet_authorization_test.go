package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_WalletScopedAuthorization proves the W2 acceptance criterion: a user with role X on
// Wallet A receives 403 on every action targeting Wallet B unless explicitly granted, while
// Owners remain tenant-wide. The gate is exercised both directly and through the real
// disbursement state-transition path.
func Test_WalletScopedAuthorization(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Two wallets: A (the default) and B.
	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('authz-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	approverOnA := &auth.User{ID: "user-approver-a", Roles: []string{string(data.ApproverUserRole)}}
	owner := &auth.User{ID: "user-owner", IsOwner: true, Roles: []string{string(data.OwnerUserRole)}}

	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, approverOnA.ID, walletA.ID, data.ApproverUserRole, nil)
	require.NoError(t, err)

	t.Run("gate unit semantics", func(t *testing.T) {
		// Member with qualifying role on A → allowed on A.
		gErr := EnsureUserCanActOnWallet(ctx, dbConnectionPool, models.WalletMemberships, approverOnA, walletA.ID,
			data.FinancialControllerUserRole, data.ApproverUserRole)
		require.NoError(t, gErr)

		// Same user, wallet B → forbidden.
		gErr = EnsureUserCanActOnWallet(ctx, dbConnectionPool, models.WalletMemberships, approverOnA, walletBID,
			data.FinancialControllerUserRole, data.ApproverUserRole)
		require.ErrorIs(t, gErr, ErrWalletActionForbidden)

		// Membership exists but the role doesn't qualify for the action → forbidden.
		gErr = EnsureUserCanActOnWallet(ctx, dbConnectionPool, models.WalletMemberships, approverOnA, walletA.ID,
			data.InitiatorUserRole)
		require.ErrorIs(t, gErr, ErrWalletActionForbidden)

		// Owner bypasses everywhere with zero membership rows.
		for _, w := range []string{walletA.ID, walletBID} {
			require.NoError(t, EnsureUserCanActOnWallet(ctx, dbConnectionPool, models.WalletMemberships, owner, w,
				data.ApproverUserRole))
		}
	})

	t.Run("cross-wallet state transitions are rejected end-to-end", func(t *testing.T) {
		svc := &DisbursementManagementService{Models: models}

		// A STARTED disbursement sourced from wallet B.
		disbursementFromB := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:           "authz-disbursement-from-b",
			Status:         data.StartedDisbursementStatus,
			SourceWalletID: walletBID,
		})

		// Approver on A cannot pause it…
		pErr := svc.PauseDisbursement(ctx, disbursementFromB.ID, approverOnA)
		require.ErrorIs(t, pErr, ErrWalletActionForbidden)

		// …or start it (gate fires before any transition/balance logic).
		sErr := svc.StartDisbursement(ctx, disbursementFromB.ID, approverOnA, nil)
		require.ErrorIs(t, sErr, ErrWalletActionForbidden)

		// State unchanged.
		got, gErr := models.Disbursements.Get(ctx, dbConnectionPool, disbursementFromB.ID)
		require.NoError(t, gErr)
		assert.Equal(t, data.StartedDisbursementStatus, got.Status)

		// Explicit grant on B unlocks the action: pause now succeeds fully.
		_, mErr := models.WalletMemberships.Insert(ctx, dbConnectionPool, approverOnA.ID, walletBID, data.ApproverUserRole, nil)
		require.NoError(t, mErr)
		require.NoError(t, svc.PauseDisbursement(ctx, disbursementFromB.ID, approverOnA))

		got, gErr = models.Disbursements.Get(ctx, dbConnectionPool, disbursementFromB.ID)
		require.NoError(t, gErr)
		assert.Equal(t, data.PausedDisbursementStatus, got.Status)
	})

	t.Run("owner is tenant-wide on every wallet's transitions", func(t *testing.T) {
		svc := &DisbursementManagementService{Models: models}

		disbursementFromB := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Name:           "authz-owner-disbursement",
			Status:         data.StartedDisbursementStatus,
			SourceWalletID: walletBID,
		})

		require.NoError(t, svc.PauseDisbursement(ctx, disbursementFromB.ID, owner),
			"owner must act on any wallet without membership rows")
	})
}

// Test_ResolveWalletReadScope pins the read-visibility contract directly (it was previously
// only exercised through endpoint tests). The security-critical case is the nil-vs-empty
// distinction: Owners get nil (tenant-wide, unfiltered), but a non-owner with no memberships
// MUST get a non-nil empty slice — returning nil there would unfilter their reads and leak
// every wallet's rows.
func Test_ResolveWalletReadScope(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('scope-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	member := &auth.User{ID: "user-scope-member", Roles: []string{string(data.ApproverUserRole)}}
	_, err = models.WalletMemberships.Insert(ctx, dbConnectionPool, member.ID, walletA.ID, data.ApproverUserRole, nil)
	require.NoError(t, err)

	t.Run("owner via IsOwner → nil scope (tenant-wide, unfiltered)", func(t *testing.T) {
		scope, sErr := ResolveWalletReadScope(ctx, dbConnectionPool, models.WalletMemberships,
			&auth.User{ID: "owner-flag", IsOwner: true})
		require.NoError(t, sErr)
		assert.Nil(t, scope)
	})

	t.Run("owner via role → nil scope", func(t *testing.T) {
		scope, sErr := ResolveWalletReadScope(ctx, dbConnectionPool, models.WalletMemberships,
			&auth.User{ID: "owner-role", Roles: []string{string(data.OwnerUserRole)}})
		require.NoError(t, sErr)
		assert.Nil(t, scope)
	})

	t.Run("member sees exactly the wallets they hold membership on", func(t *testing.T) {
		scope, sErr := ResolveWalletReadScope(ctx, dbConnectionPool, models.WalletMemberships, member)
		require.NoError(t, sErr)
		assert.ElementsMatch(t, []string{walletA.ID}, scope)

		_, mErr := models.WalletMemberships.Insert(ctx, dbConnectionPool, member.ID, walletBID, data.ApproverUserRole, nil)
		require.NoError(t, mErr)
		scope, sErr = ResolveWalletReadScope(ctx, dbConnectionPool, models.WalletMemberships, member)
		require.NoError(t, sErr)
		assert.ElementsMatch(t, []string{walletA.ID, walletBID}, scope)
	})

	t.Run("non-owner with no memberships → empty, non-nil scope (sees nothing)", func(t *testing.T) {
		scope, sErr := ResolveWalletReadScope(ctx, dbConnectionPool, models.WalletMemberships,
			&auth.User{ID: "user-no-memberships", Roles: []string{string(data.ApproverUserRole)}})
		require.NoError(t, sErr)
		require.NotNil(t, scope, "non-nil empty scope filters reads to nothing; nil would unfilter and leak all wallets")
		assert.Empty(t, scope)
	})

	t.Run("nil user is rejected", func(t *testing.T) {
		_, sErr := ResolveWalletReadScope(ctx, dbConnectionPool, models.WalletMemberships, nil)
		require.Error(t, sErr)
	})
}
