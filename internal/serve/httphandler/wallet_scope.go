package httphandler

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// resolveWalletReadScope computes the caller's read-visibility scope for
// membership-filtered endpoints (taxonomy): nil = Owner, no filtering; non-nil = the exact
// wallet set the caller may see (possibly empty).
func resolveWalletReadScope(ctx context.Context, authManager auth.AuthManager, models *data.Models) ([]string, *httperror.HTTPError) {
	user, err := ctxHelper.GetUserFromContext(ctx, authManager)
	if err != nil {
		return nil, httperror.InternalError(ctx, "Cannot get user from context", err, nil)
	}

	// Owners are tenant-wide by definition — no membership lookup (and no DB) needed.
	if user.IsOwner || slices.Contains(user.Roles, string(data.OwnerUserRole)) {
		return nil, nil
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

// ensureWalletActionAllowed gates a state transition on the caller's wallet membership
// Owners pass; everyone else needs a qualifying role on the wallet. Returns a 403
// that discloses no wallet details.
func ensureWalletActionAllowed(ctx context.Context, authManager auth.AuthManager, models *data.Models, walletID string, requiredRoles ...data.UserRole) *httperror.HTTPError {
	user, err := ctxHelper.GetUserFromContext(ctx, authManager)
	if err != nil {
		return httperror.InternalError(ctx, "Cannot get user from context", err, nil)
	}
	if err := services.EnsureUserCanActOnWallet(ctx, models.DBConnectionPool, models.WalletMemberships, user, walletID, requiredRoles...); err != nil {
		if errors.Is(err, services.ErrWalletActionForbidden) {
			return httperror.Forbidden(services.ErrWalletActionForbidden.Error(), err, nil)
		}
		return httperror.InternalError(ctx, "Cannot authorize wallet action", err, nil)
	}
	return nil
}

// XWalletIDHeader carries the explicit source distribution wallet on write requests.
const XWalletIDHeader = "X-Wallet-Id"

// resolveSourceWalletForWrite implements the routing rule for fund-moving writes:
//   - explicit X-Wallet-Id is honored after entitlement + status checks
//   - omitted header: tenants with EXACTLY ONE active wallet (pre-opt-in single-wallet
//     tenants) legitimately fall back to it per the spec's narrow default semantics; tenants
//     with multiple wallets get 400 — no silent routing fallbacks
//   - 403 carries no wallet existence/details (unknown wallet and unentitled wallet are
//     indistinguishable to non-owners); Owners get an honest 404 for unknown ids
//   - archived wallets accept no new disbursements or payments → 400 (only after the caller
//     proves entitlement, so archived-ness is not leaked)
func resolveSourceWalletForWrite(ctx context.Context, req *http.Request, authManager auth.AuthManager, models *data.Models, requiredRoles ...data.UserRole) (*data.DistributionWallet, *httperror.HTTPError) {
	user, err := ctxHelper.GetUserFromContext(ctx, authManager)
	if err != nil {
		return nil, httperror.InternalError(ctx, "Cannot get user from context", err, nil)
	}

	dbPool := models.DBConnectionPool
	headerWalletID := req.Header.Get(XWalletIDHeader)

	var wallet *data.DistributionWallet
	if headerWalletID == "" {
		activeWallets, listErr := models.DistributionWallets.GetAll(ctx, dbPool, false)
		if listErr != nil {
			return nil, httperror.InternalError(ctx, "Cannot resolve the source wallet", listErr, nil)
		}
		if len(activeWallets) != 1 {
			return nil, httperror.BadRequest(
				"the X-Wallet-Id header is required: this tenant has multiple distribution wallets", nil, nil)
		}
		wallet = &activeWallets[0]
	} else {
		loaded, getErr := models.DistributionWallets.Get(ctx, dbPool, headerWalletID)
		if getErr != nil {
			if !errors.Is(getErr, data.ErrRecordNotFound) {
				return nil, httperror.InternalError(ctx, "Cannot resolve the source wallet", getErr, nil)
			}
			// Unknown wallet: Owners get an honest 404; everyone else gets the same 403 as
			// an unentitled wallet — existence is never disclosed.
			if user.IsOwner {
				return nil, httperror.NotFound("distribution wallet not found", getErr, nil)
			}
			return nil, httperror.Forbidden(services.ErrWalletActionForbidden.Error(), getErr, nil)
		}
		wallet = loaded
	}

	if authzErr := services.EnsureUserCanActOnWallet(ctx, dbPool, models.WalletMemberships, user, wallet.ID, requiredRoles...); authzErr != nil {
		if errors.Is(authzErr, services.ErrWalletActionForbidden) {
			return nil, httperror.Forbidden(services.ErrWalletActionForbidden.Error(), authzErr, nil)
		}
		return nil, httperror.InternalError(ctx, "Cannot authorize wallet action", authzErr, nil)
	}

	if wallet.Status == data.ArchivedDistributionWalletStatus {
		return nil, httperror.BadRequest("the wallet is archived and accepts no new disbursements or payments", nil, nil)
	}

	// Per-wallet observability: wallet_id joins the request's structured-log context.
	log.Set(ctx, log.Ctx(ctx).WithField("wallet_id", wallet.ID))

	return wallet, nil
}
