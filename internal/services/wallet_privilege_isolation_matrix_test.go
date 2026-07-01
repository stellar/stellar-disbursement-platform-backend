package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// Test_CrossWallet_PrivilegeIsolation_Matrix is the acceptance matrix: one test per
// (action × actor × wallet) combination. A user with role X on Wallet A receives 403 on every
// action targeting Wallet B unless explicitly granted; Owners are tenant-wide; roles that do
// not qualify for an action are rejected even on their own wallet.
func Test_CrossWallet_PrivilegeIsolation_Matrix(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	svc := &DisbursementManagementService{Models: models}

	// Wallets A (default) and B.
	walletA := data.EnsureDefaultDistributionWalletFixture(t, ctx, dbConnectionPool)
	var walletBID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &walletBID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('matrix-wallet-b', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	// Actors: every (role, wallet-membership) configuration relevant to transition actions.
	newActor := func(id string, roles ...data.UserRole) *auth.User {
		roleStrings := make([]string, len(roles))
		for i, r := range roles {
			roleStrings[i] = string(r)
		}
		return &auth.User{ID: id, Email: id + "@matrix.test", Roles: roleStrings}
	}
	approverOnA := newActor("matrix-approver-a", data.ApproverUserRole)
	fcOnA := newActor("matrix-fc-a", data.FinancialControllerUserRole)
	initiatorOnA := newActor("matrix-initiator-a", data.InitiatorUserRole)
	approverOnB := newActor("matrix-approver-b", data.ApproverUserRole)
	approverNoWallet := newActor("matrix-approver-none", data.ApproverUserRole)
	owner := &auth.User{ID: "matrix-owner", Email: "owner@matrix.test", IsOwner: true}

	grant := func(user *auth.User, walletID string, role data.UserRole) {
		_, gErr := models.WalletMemberships.Insert(ctx, dbConnectionPool, user.ID, walletID, role, nil)
		require.NoError(t, gErr)
	}
	grant(approverOnA, walletA.ID, data.ApproverUserRole)
	grant(fcOnA, walletA.ID, data.FinancialControllerUserRole)
	grant(initiatorOnA, walletA.ID, data.InitiatorUserRole)
	grant(approverOnB, walletBID, data.ApproverUserRole)

	// The matrix: action × actor × target wallet → is the wallet gate expected to fire?
	type actionFn func(t *testing.T, disbursementID string, user *auth.User) error
	pause := func(t *testing.T, id string, user *auth.User) error {
		t.Helper()
		return svc.PauseDisbursement(ctx, id, user)
	}
	start := func(t *testing.T, id string, user *auth.User) error {
		t.Helper()
		return svc.StartDisbursement(ctx, id, user, nil)
	}

	matrix := []struct {
		action       string
		fn           actionFn
		actor        *auth.User
		targetWallet string
		wantGate403  bool
	}{
		// PauseDisbursement on a disbursement sourced from wallet A:
		{"pause", pause, approverOnA, walletA.ID, false},
		{"pause", pause, fcOnA, walletA.ID, false},
		{"pause", pause, initiatorOnA, walletA.ID, true}, // initiator does not qualify for transitions
		{"pause", pause, approverOnB, walletA.ID, true},  // cross-wallet
		{"pause", pause, approverNoWallet, walletA.ID, true},
		{"pause", pause, owner, walletA.ID, false},
		// PauseDisbursement on a disbursement sourced from wallet B:
		{"pause", pause, approverOnA, walletBID, true}, // cross-wallet (the canonical AC)
		{"pause", pause, approverOnB, walletBID, false},
		{"pause", pause, owner, walletBID, false},
		// StartDisbursement gate (gate fires before any transition/balance logic):
		{"start", start, approverOnA, walletBID, true},
		{"start", start, approverNoWallet, walletA.ID, true},
		{"start", start, approverOnB, walletA.ID, true},
	}

	for i, tc := range matrix {
		name := fmt.Sprintf("%s as %s on wallet=%s gate403=%v", tc.action, tc.actor.ID, short(tc.targetWallet, walletA.ID, walletBID), tc.wantGate403)
		t.Run(name, func(t *testing.T) {
			// Fresh STARTED disbursement from the target wallet for each combination.
			disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
				Name:           fmt.Sprintf("matrix-%d-%s", i, tc.actor.ID),
				Status:         data.StartedDisbursementStatus,
				SourceWalletID: tc.targetWallet,
			})

			actErr := tc.fn(t, disbursement.ID, tc.actor)
			if tc.wantGate403 {
				require.ErrorIs(t, actErr, ErrWalletActionForbidden,
					"the wallet gate must reject this combination")
			} else {
				require.NotErrorIs(t, actErr, ErrWalletActionForbidden,
					"the wallet gate must NOT reject this combination")
			}
		})
	}
}

func short(id string, a, b string) string {
	switch id {
	case a:
		return "A"
	case b:
		return "B"
	default:
		return id
	}
}
