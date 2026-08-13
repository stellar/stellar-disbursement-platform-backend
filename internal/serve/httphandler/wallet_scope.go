package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// resolveWalletReadScope computes the caller's read-visibility scope for
// membership-filtered endpoints (taxonomy): nil = Owner, no filtering; non-nil = the exact
// wallet set the caller may see (possibly empty).
//
// An API key answers from its own stored scope and never loads a user: a key's reach is fixed at
// creation, so it neither depends on its creator still being active nor moves when their
// memberships change.
func resolveWalletReadScope(ctx context.Context, authManager auth.AuthManager, models *data.Models) ([]string, *httperror.HTTPError) {
	if apiKey, err := sdpcontext.GetAPIKeyFromContext(ctx); err == nil && apiKey != nil {
		return apiKey.WalletScope(), nil
	}

	user, err := ctxHelper.GetUserFromContext(ctx, authManager)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return nil, httperror.Unauthorized("", err, nil)
		}
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

// resolveWalletListScope computes the scope for the membership-filtered LIST/aggregate endpoints
// (disbursements, payments, receivers, statistics, exports), layering the active account
// selection on top of read visibility so the per-account view is consistent for everyone:
//   - no X-Wallet-Id ("All accounts"): full visibility — Owner sees tenant-wide (nil), a member
//     sees their membership set.
//   - explicit X-Wallet-Id: narrow to that one account, but never beyond what the caller may
//     see. Owners can select any account; a member selecting an account they hold no membership
//     on gets an empty scope (sees nothing) rather than a leak.
//
// This is intentionally separate from resolveWalletReadScope, which stays a pure visibility
// check for the switcher's account list and for single-resource reads (where navigating to a
// specific item must not be filtered out by the currently-selected account).
func resolveWalletListScope(ctx context.Context, req *http.Request, authManager auth.AuthManager, models *data.Models) ([]string, *httperror.HTTPError) {
	visibility, httpErr := resolveWalletReadScope(ctx, authManager, models)
	if httpErr != nil {
		return nil, httpErr
	}

	headerWalletID := req.Header.Get(XWalletIDHeader)
	if headerWalletID == "" {
		return visibility, nil
	}

	// visibility == nil means Owner (may see every account); otherwise the wallet must be in the
	// member's set.
	if visibility == nil || slices.Contains(visibility, headerWalletID) {
		return []string{headerWalletID}, nil
	}
	return []string{}, nil
}

// ensureWalletActionAllowed gates a state transition on the caller's wallet membership
// Owners pass; everyone else needs a qualifying role on the wallet. Returns a 403
// that discloses no wallet details.
//
// For an API key the wallet dimension is its own scope and the role dimension is its permission
// set, already checked by RequirePermission on the route — so the key's scope is the whole answer.
func ensureWalletActionAllowed(ctx context.Context, authManager auth.AuthManager, models *data.Models, walletID string, requiredRoles ...data.UserRole) *httperror.HTTPError {
	if apiKey, err := sdpcontext.GetAPIKeyFromContext(ctx); err == nil && apiKey != nil {
		if !apiKey.CanActOnWallet(walletID) {
			return httperror.Forbidden(services.ErrWalletActionForbidden.Error(), nil, nil)
		}
		return nil
	}

	user, err := ctxHelper.GetUserFromContext(ctx, authManager)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return httperror.Unauthorized("", err, nil)
		}
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

// walletCapability is one wallet-scoped write action, written as the two gates a request has to
// clear: the tenant-role gate declared on its route (serve.go) and the membership-role gate
// enforced at the action site. Authorization is global role first and membership second — a
// membership can only narrow, never widen — so the caller must satisfy both sets.
type walletCapability struct {
	name        string
	globalRoles []data.UserRole
	walletRoles []data.UserRole
}

// walletCapabilityMatrix is the single place that pairing is written down: the capabilities
// endpoint and the inert-grant check both read it instead of restating the rules, and
// Test_WalletCapabilityMatrix_Conformance pins each entry to the enforcement site named in its
// comment, so this table cannot drift from the code that actually enforces it.
var walletCapabilityMatrix = []walletCapability{
	{
		// POST /disbursements, POST /disbursements/{id}/instructions, DELETE /disbursements/{id}.
		name:        "can_create_disbursement",
		globalRoles: []data.UserRole{data.OwnerUserRole, data.FinancialControllerUserRole, data.InitiatorUserRole},
		walletRoles: []data.UserRole{data.FinancialControllerUserRole, data.InitiatorUserRole},
	},
	{
		// PATCH /disbursements/{id}/status → DisbursementManagementService.StartDisbursement.
		name:        "can_start_disbursement",
		globalRoles: []data.UserRole{data.OwnerUserRole, data.FinancialControllerUserRole, data.ApproverUserRole},
		walletRoles: []data.UserRole{data.FinancialControllerUserRole, data.ApproverUserRole},
	},
	{
		// PATCH /disbursements/{id}/status → DisbursementManagementService.PauseDisbursement.
		name:        "can_pause_disbursement",
		globalRoles: []data.UserRole{data.OwnerUserRole, data.FinancialControllerUserRole, data.ApproverUserRole},
		walletRoles: []data.UserRole{data.FinancialControllerUserRole, data.ApproverUserRole},
	},
	{
		// PATCH /disbursements/{id}/status → DisbursementManagementService.CancelDisbursement.
		name:        "can_cancel_disbursement",
		globalRoles: []data.UserRole{data.OwnerUserRole, data.FinancialControllerUserRole, data.ApproverUserRole},
		walletRoles: []data.UserRole{data.FinancialControllerUserRole, data.ApproverUserRole},
	},
	{
		// POST /payments (direct payment) → PostDirectPayment.
		name:        "can_create_payment",
		globalRoles: []data.UserRole{data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole},
		walletRoles: []data.UserRole{data.FinancialControllerUserRole, data.BusinessUserRole},
	},
	{
		// PATCH /payments/retry → RetryPayments.
		name:        "can_retry_payment",
		globalRoles: []data.UserRole{data.OwnerUserRole, data.FinancialControllerUserRole, data.BusinessUserRole},
		walletRoles: []data.UserRole{data.FinancialControllerUserRole, data.BusinessUserRole},
	},
	{
		// PATCH /payments/{id}/status (cancel) → PatchPaymentStatus.
		name:        "can_cancel_payment",
		globalRoles: []data.UserRole{data.OwnerUserRole, data.FinancialControllerUserRole},
		walletRoles: []data.UserRole{data.FinancialControllerUserRole},
	},
}

// walletCapabilitiesFor computes what a caller may actually do on one wallet, given their global
// roles and the membership roles they hold on that wallet. Owners are tenant-wide: neither gate
// applies to them. There is deliberately no "effective role" here — the two gates do not
// collapse into one, which is exactly why the pair has to be reported as a capability set.
func walletCapabilitiesFor(user *auth.User, membershipRoles []data.UserRole) map[string]bool {
	isOwner := user.IsOwner || slices.Contains(user.Roles, string(data.OwnerUserRole))
	globalRoles := userRoles(user)

	capabilities := make(map[string]bool, len(walletCapabilityMatrix))
	for _, capability := range walletCapabilityMatrix {
		globalOK := isOwner || rolesIntersect(globalRoles, capability.globalRoles)
		walletOK := isOwner || rolesIntersect(membershipRoles, capability.walletRoles)
		capabilities[capability.name] = globalOK && walletOK
	}
	return capabilities
}

// walletGrantIsInert reports whether granting role to user would confer nothing whatsoever.
//
// The reviewer's rule is "reject a grant that narrows to nothing", and his example was an
// approver membership on a global developer. Measured on WRITE capability alone that pair is
// indeed empty — but it is not inert, and rejecting it would break the product:
//
//   - A membership is also the READ-visibility grant. GetWalletIDsForUser selects every
//     membership row regardless of role, and ResolveWalletReadScope filters on that set, so ANY
//     membership makes the account visible to its holder.
//   - serve.go admits developers to the membership-scoped reads via GetAllRoles(), and
//     GetDistributionWallets filters by membership. So a membership is the ONLY way a developer
//     ever sees an account at all; rejecting the grant leaves their account list permanently
//     empty with no API call able to populate it.
//
// The genuinely inert case is the one the check was written to catch — "a no-op the operator
// cannot see" — and it is the opposite grantee: a global Owner. Owners short-circuit both gates
// (EnsureUserCanActOnWallet and ResolveWalletReadScope return early for them), so the row
// changes nothing at all, and the operator cannot tell it had no effect.
//
// Grants that confer read but not write are real and are allowed here. Making their consequence
// legible is the job of the grant picker's per-role annotation, which is the reviewer's own
// separate fix for the same confusion — enforcement rejects no-ops, the affordance explains the
// rest.
func walletGrantIsInert(user *auth.User, _ data.UserRole) bool {
	return user.IsOwner || slices.Contains(user.Roles, string(data.OwnerUserRole))
}

// inertGrantReason explains a rejected grant in the operator's terms.
func inertGrantReason(role data.UserRole) string {
	article := "a"
	if name := role.String(); name != "" && strings.ContainsRune("aeiou", rune(name[0])) {
		article = "an"
	}

	return fmt.Sprintf(
		"%s %s membership grants nothing to an owner: owners already have tenant-wide access to "+
			"every distribution account, so this membership would have no effect", article, role)
}

func userRoles(user *auth.User) []data.UserRole {
	roles := make([]data.UserRole, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, data.UserRole(role))
	}
	return roles
}

func rolesIntersect(have, want []data.UserRole) bool {
	for _, role := range have {
		if slices.Contains(want, role) {
			return true
		}
	}
	return false
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
//
// An API key resolves against its own scope throughout: it never loads a user, a key naming exactly
// one wallet keeps working without the header, and an unknown id is always a 403 (a key has no
// owner identity to earn the honest 404). The header only selects — it never grants.
func resolveSourceWalletForWrite(ctx context.Context, req *http.Request, authManager auth.AuthManager, models *data.Models, requiredRoles ...data.UserRole) (*data.DistributionWallet, *httperror.HTTPError) {
	apiKey, keyErr := sdpcontext.GetAPIKeyFromContext(ctx)
	viaAPIKey := keyErr == nil && apiKey != nil

	var user *auth.User
	if !viaAPIKey {
		var err error
		user, err = ctxHelper.GetUserFromContext(ctx, authManager)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				return nil, httperror.Unauthorized("", err, nil)
			}
			return nil, httperror.InternalError(ctx, "Cannot get user from context", err, nil)
		}
	}

	dbPool := models.DBConnectionPool
	headerWalletID := req.Header.Get(XWalletIDHeader)

	// A key naming one wallet is unambiguous without the header. The tenant-level fallback below
	// only fires when exactly one wallet is active, so without this an existing integration would
	// start failing the moment its tenant adds a second wallet.
	if headerWalletID == "" && viaAPIKey && len(apiKey.WalletScope()) == 1 {
		headerWalletID = apiKey.WalletScope()[0]
	}

	var wallet *data.DistributionWallet
	if headerWalletID == "" {
		activeWallets, listErr := models.DistributionWallets.GetAll(ctx, dbPool, false)
		if listErr != nil {
			return nil, httperror.InternalError(ctx, "Cannot resolve the source wallet", listErr, nil)
		}
		if len(activeWallets) != 1 {
			return nil, httperror.BadRequest(
				"the X-Wallet-Id header is required to select a source distribution wallet", nil, nil)
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
			if !viaAPIKey && user.IsOwner {
				return nil, httperror.NotFound("distribution wallet not found", getErr, nil)
			}
			return nil, httperror.Forbidden(services.ErrWalletActionForbidden.Error(), getErr, nil)
		}
		wallet = loaded
	}

	if viaAPIKey {
		if !apiKey.CanActOnWallet(wallet.ID) {
			return nil, httperror.Forbidden(services.ErrWalletActionForbidden.Error(), nil, nil)
		}
	} else if authzErr := services.EnsureUserCanActOnWallet(ctx, dbPool, models.WalletMemberships, user, wallet.ID, requiredRoles...); authzErr != nil {
		if errors.Is(authzErr, services.ErrWalletActionForbidden) {
			return nil, httperror.Forbidden(services.ErrWalletActionForbidden.Error(), authzErr, nil)
		}
		return nil, httperror.InternalError(ctx, "Cannot authorize wallet action", authzErr, nil)
	}

	if wallet.Status != data.ActiveDistributionWalletStatus {
		return nil, httperror.BadRequest("the wallet is not active and accepts no new disbursements or payments", nil, nil)
	}

	// Per-wallet observability: wallet_id joins the request's structured-log context.
	log.Set(ctx, log.Ctx(ctx).WithField("wallet_id", wallet.ID))

	return wallet, nil
}
