package services

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// ErrWalletActionForbidden is returned when a user attempts an action on a wallet they hold no
// qualifying membership on. The API layer maps it to 403 Forbidden. Per the leak matrix,
// the public error message must not disclose wallet existence or details.
var ErrWalletActionForbidden = errors.New("user is not authorized to act on this wallet")

// IsTenantWideUser reports whether the user spans every wallet (owners and developers), so
// membership never scopes them.
func IsTenantWideUser(user *auth.User) bool {
	if user.IsOwner {
		return true
	}
	return slices.ContainsFunc(user.Roles, func(role string) bool {
		return data.IsTenantWideRole(data.UserRole(role))
	})
}

// ResolveWalletReadScope returns the caller's read scope: nil for tenant-wide users (no filter), otherwise the wallet IDs they hold membership on.
// An empty non-nil slice means the user sees no per-wallet rows.
func ResolveWalletReadScope(ctx context.Context, sqlExec db.SQLExecuter, memberships *data.WalletMembershipModel, user *auth.User) ([]string, error) {
	if user == nil {
		return nil, fmt.Errorf("user is required to resolve wallet read scope")
	}
	if IsTenantWideUser(user) {
		return nil, nil
	}

	walletIDs, err := memberships.GetWalletIDsForUser(ctx, sqlExec, user.ID)
	if err != nil {
		return nil, fmt.Errorf("resolving wallet read scope for user %q: %w", user.ID, err)
	}
	return walletIDs, nil
}

// EnsureUserCanActOnWallet passes tenant-wide users; anyone else needs one of requiredRoles on the wallet via wallet_memberships.
// It adds the wallet dimension on top of the route-level role checks without changing role semantics.
func EnsureUserCanActOnWallet(ctx context.Context, sqlExec db.SQLExecuter, memberships *data.WalletMembershipModel, user *auth.User, walletID string, requiredRoles ...data.UserRole) error {
	if user == nil {
		return fmt.Errorf("user is required for wallet authorization")
	}
	if walletID == "" {
		return fmt.Errorf("wallet ID is required for wallet authorization")
	}
	if IsTenantWideUser(user) {
		return nil
	}

	has, err := memberships.HasRoleOnWallet(ctx, sqlExec, user.ID, walletID, requiredRoles...)
	if err != nil {
		return fmt.Errorf("checking wallet membership for user %q on wallet %q: %w", user.ID, walletID, err)
	}
	if !has {
		return fmt.Errorf("user %q holds none of %v on wallet %q: %w", user.ID, requiredRoles, walletID, ErrWalletActionForbidden)
	}

	return nil
}
