package httphandler

import (
	"context"
	"slices"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// resolveWalletReadScope computes the caller's read-visibility scope for
// membership-filtered endpoints (W2 taxonomy): nil = Owner, no filtering; non-nil = the exact
// wallet set the caller may see (possibly empty).
func resolveWalletReadScope(ctx context.Context, authManager auth.AuthManager, models *data.Models) ([]string, *httperror.HTTPError) {
	user, err := ctxHelper.GetUserFromContext(ctx, authManager)
	if err != nil {
		return nil, httperror.InternalError(ctx, "Cannot get user from context", err, nil)
	}

	scope, err := services.ResolveWalletReadScope(ctx, models.DBConnectionPool, models.WalletMemberships, user)
	if err != nil {
		return nil, httperror.InternalError(ctx, "Cannot resolve wallet visibility", err, nil)
	}

	return scope, nil
}

// walletInReadScope reports whether a wallet is visible within the resolved scope. Callers
// must respond with 404 (not 403) on individual reads outside the scope — per the read-leakage
// rules, existence is never disclosed.
func walletInReadScope(scope []string, walletID string) bool {
	return scope == nil || slices.Contains(scope, walletID)
}
