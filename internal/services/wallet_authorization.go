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
// qualifying membership on. The API layer maps it to 403 Forbidden. Per the W4 leak matrix,
// the public error message must not disclose wallet existence or details.
var ErrWalletActionForbidden = errors.New("user is not authorized to act on this wallet")

// EnsureUserCanActOnWallet enforces wallet-scoped action authorization (Workstream 2):
// Owners are always tenant-wide; every other caller must hold at least one of requiredRoles
// ON the target wallet via wallet_memberships. Role semantics are unchanged — this adds the
// wallet dimension on top of the route-level tenant-role checks.
func EnsureUserCanActOnWallet(ctx context.Context, sqlExec db.SQLExecuter, memberships *data.WalletMembershipModel, user *auth.User, walletID string, requiredRoles ...data.UserRole) error {
	if user == nil {
		return fmt.Errorf("user is required for wallet authorization")
	}
	if walletID == "" {
		return fmt.Errorf("wallet ID is required for wallet authorization")
	}
	if user.IsOwner || slices.Contains(user.Roles, string(data.OwnerUserRole)) {
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
