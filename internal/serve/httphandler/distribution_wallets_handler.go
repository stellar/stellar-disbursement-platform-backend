package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// DistributionWalletsHandler exposes Owner-only CRUD for the tenant's distribution wallets
// (the sending accounts — not the recipient wallet providers served by WalletsHandler).
type DistributionWalletsHandler struct {
	Service                     services.DistributionWalletManagementServiceInterface
	AuthManager                 auth.AuthManager
	Models                      *data.Models
	DistributionAccountService  services.DistributionAccountServiceInterface
	DistributionAccountResolver signing.DistributionAccountResolver
}

// WalletBalanceResponse is one wallet's live balance set (the dashboard Total Balance tile).
type WalletBalanceResponse struct {
	WalletID string            `json:"wallet_id,omitempty"`
	Balances map[string]string `json:"balances"`
}

// GetDistributionWalletBalance returns one wallet's live on-chain balances. Reads follow the
// membership taxonomy: 404 outside the caller's scope (existence undisclosed).
func (h DistributionWalletsHandler) GetDistributionWalletBalance(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

	scope, scopeErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if scopeErr != nil {
		scopeErr.Render(rw)
		return
	}
	if !walletInReadScope(scope, walletID) {
		httperror.NotFound("distribution wallet not found", nil, nil).Render(rw)
		return
	}

	wallet, err := h.Service.GetWallet(ctx, walletID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("distribution wallet not found", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot load distribution wallet", err, nil).Render(rw)
		return
	}

	balances, httpErr := h.walletBalances(ctx, wallet)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	httpjson.Render(rw, WalletBalanceResponse{WalletID: wallet.ID, Balances: balances}, httpjson.JSON)
}

// GetDistributionWalletsTotalBalance returns the balance aggregate, scoped per the read
// taxonomy exactly like /statistics: Owners sum every wallet (the tenant-wide view stays
// Owner-only); members sum only the wallets they hold memberships on. No caller ever sees a
// balance outside their scope.
func (h DistributionWalletsHandler) GetDistributionWalletsTotalBalance(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	scope, scopeErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if scopeErr != nil {
		scopeErr.Render(rw)
		return
	}

	wallets, err := h.Service.ListWallets(ctx, false)
	if err != nil {
		httperror.InternalError(ctx, "Cannot list distribution wallets", err, nil).Render(rw)
		return
	}

	totals := map[string]string{}
	for i := range wallets {
		// walletInReadScope returns true for a nil (owner/tenant-wide) scope, so no extra guard.
		if !walletInReadScope(scope, wallets[i].ID) {
			continue
		}
		balances, httpErr := h.walletBalances(ctx, &wallets[i])
		if httpErr != nil {
			httpErr.Render(rw)
			return
		}
		for asset, amount := range balances {
			totals[asset] = sumDecimalStrings(ctx, totals[asset], amount)
		}
	}

	httpjson.Render(rw, WalletBalanceResponse{Balances: totals}, httpjson.JSON)
}

// walletBalances reports one wallet's live balances. The account is resolved through the shared
// helper rather than read off the wallet row: a Circle tenant's row is blank by design, so
// reading it directly reported an empty balance set here while the legacy GET /balances (which
// resolves from the tenant context) reported the real one — two endpoints disagreeing for the
// same tenant, and a Total Balance tile stuck at zero.
//
// The aggregate caller loops over wallets, but a Circle tenant cannot double-count: multi-wallet
// is Stellar-only (CreateWallet rejects any other tenant with ErrTenantNotEligibleForMultiWallet),
// so such a tenant has exactly one wallet row to resolve.
func (h DistributionWalletsHandler) walletBalances(ctx context.Context, wallet *data.DistributionWallet) (map[string]string, *httperror.HTTPError) {
	account, ok, httpErr := resolveDistributionAccountForBalanceRead(ctx, h.DistributionAccountResolver, wallet)
	if httpErr != nil {
		return nil, httpErr
	}
	if !ok {
		// Pending-activation Stellar wallets have no on-chain account yet.
		return map[string]string{}, nil
	}

	balances, err := h.DistributionAccountService.GetBalances(ctx, &account)
	if err != nil {
		return nil, httperror.InternalError(ctx, "Cannot retrieve wallet balances", err, nil)
	}

	out := make(map[string]string, len(balances))
	for asset, amount := range balances {
		out[assetBalanceKey(asset)] = amount.String()
	}
	return out, nil
}

func assetBalanceKey(asset data.Asset) string {
	if asset.IsNative() {
		return "XLM"
	}
	return asset.Code + ":" + asset.Issuer
}

func sumDecimalStrings(ctx context.Context, a, b string) string {
	if a == "" {
		return b
	}
	da, errA := decimal.NewFromString(a)
	db, errB := decimal.NewFromString(b)
	if errA != nil || errB != nil {
		// Inputs come from decimal.String() today so this never fires, but log rather than
		// silently under-report the aggregated Total Balance if a non-decimal source appears.
		log.Ctx(ctx).Warnf("sumDecimalStrings: skipping unparseable balance (a=%q, b=%q)", a, b)
		return a
	}
	return da.Add(db).String()
}

// WalletMembershipRequest is the request body to grant a wallet-scoped role.
type WalletMembershipRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// ensureWalletInReadScope applies the read taxonomy to a single-wallet read: 404 outside the
// caller's scope, existence never disclosed.
func (h DistributionWalletsHandler) ensureWalletInReadScope(ctx context.Context, walletID string) *httperror.HTTPError {
	scope, scopeErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if scopeErr != nil {
		return scopeErr
	}
	if !walletInReadScope(scope, walletID) {
		return httperror.NotFound("distribution wallet not found", nil, nil)
	}
	return nil
}

// ensureCallerIsOwner enforces the Owner-only requirement of the distribution-wallet WRITE
// surface inside the handler, where it holds on every authentication path.
//
// The route gate cannot carry it alone: middleware.RequirePermission short-circuits straight to
// the handler as soon as an API key carries write:distribution_wallets (or the write:all
// wildcard) and never invokes the AnyRoleMiddleware(OwnerUserRole) it was handed. Without this
// check any such key could create, archive, promote, grant and revoke — an escalation on
// owner-only operations, and an existence oracle whose every probe mutates.
//
// "The acting user must be an Owner" is the rule, mirroring what the route gate intended, rather
// than a per-wallet scope check: these operations are tenant-wide (create, promote-to-default) or
// hand out authority itself, and Owner is deliberately not grantable per wallet
// (PostDistributionWalletMembership rejects it), so no membership can stand in for it. On the API
// key path the acting identity is the key's creator — the middleware sets
// SetUserIDInContext(apiKey.CreatedBy) — so a key minted by a non-owner is denied here.
//
// It fails closed: an acting user that cannot be resolved (missing from the context, deleted
// since the key was created) is rejected, never passed through.
func (h DistributionWalletsHandler) ensureCallerIsOwner(ctx context.Context) (*auth.User, *httperror.HTTPError) {
	user, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return nil, httperror.Unauthorized("", err, nil)
		}
		return nil, httperror.InternalError(ctx, "Cannot get user from context", err, nil)
	}

	if !user.IsOwner && !slices.Contains(user.Roles, string(data.OwnerUserRole)) {
		// Same empty-message 403 as the route gate: no wallet or role detail is disclosed.
		return nil, httperror.Forbidden("", nil, nil)
	}

	return user, nil
}

// GetDistributionWalletMemberships lists a wallet's memberships (admin/audit surface —
// archived wallets remain queryable). The route is Owner-only, but API keys authenticate past
// AnyRoleMiddleware on permission alone, so the read scope is enforced here too.
func (h DistributionWalletsHandler) GetDistributionWalletMemberships(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

	if httpErr := h.ensureWalletInReadScope(ctx, walletID); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	memberships, err := h.Service.ListMemberships(ctx, walletID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("distribution wallet not found", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot list wallet memberships", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, memberships, httpjson.JSON)
}

// GetDistributionWalletAudit returns a wallet's membership-change history (grants and
// revokes) from the append-only audit table, newest first (admin/audit surface — archived
// wallets remain queryable). Scope-checked here for the same reason as the membership list.
func (h DistributionWalletsHandler) GetDistributionWalletAudit(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

	if httpErr := h.ensureWalletInReadScope(ctx, walletID); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	entries, err := h.Service.ListMembershipAudit(ctx, walletID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("distribution wallet not found", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot list wallet membership audit", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, entries, httpjson.JSON)
}

// PostDistributionWalletMembership grants a wallet-scoped role. Grants on archived wallets
// return 409 Conflict per the spec — an inert grant would only add audit-log noise.
func (h DistributionWalletsHandler) PostDistributionWalletMembership(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

	// The caller is established first: an unauthenticated or deleted caller must be told so
	// before the request body's user_id is looked up, or the grantee checks below become a
	// user-existence and role oracle for a caller who has no identity at all.
	grantor, httpErr := h.ensureCallerIsOwner(ctx)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	var reqBody WalletMembershipRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(rw)
		return
	}

	// Validate the role up front: an unknown role would otherwise pass the service checks, hit
	// the wallet_memberships CHECK constraint, and surface to the operator as a 500. Owner is
	// tenant-wide and cannot be granted per wallet.
	role := data.UserRole(reqBody.Role)
	if !role.IsValid() || role == data.OwnerUserRole {
		scopableRoles := make([]data.UserRole, 0)
		for _, r := range data.GetAllRoles() {
			if r != data.OwnerUserRole {
				scopableRoles = append(scopableRoles, r)
			}
		}
		httperror.BadRequest(
			fmt.Sprintf("unexpected value for role=%q. Expect one of these values: %s", reqBody.Role, scopableRoles),
			nil, nil,
		).Render(rw)
		return
	}

	// The wallet is validated before the grantee, so a POST to a wallet that does not exist
	// reports the wallet rather than answering questions about the request-supplied user_id.
	if _, walletErr := h.Service.GetWallet(ctx, walletID); walletErr != nil {
		if errors.Is(walletErr, data.ErrRecordNotFound) {
			httperror.NotFound("distribution wallet not found", walletErr, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot load distribution wallet", walletErr, nil).Render(rw)
		return
	}

	// A membership narrows the grantee's global role, so a grant whose capability set is empty
	// is a no-op the operator cannot see: "Manage access" reads as "give this person this role
	// here", and it must not silently mean nothing. An empty user_id is left to the service —
	// that is its own 400.
	if reqBody.UserID != "" {
		if inertErr := h.ensureGrantIsNotInert(ctx, reqBody.UserID, role); inertErr != nil {
			inertErr.Render(rw)
			return
		}
	}

	membership, err := h.Service.GrantMembership(ctx, walletID, reqBody.UserID, role, grantor.ID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrWalletArchivedForMembership):
			httperror.Conflict("cannot grant membership on an archived wallet", err, nil).Render(rw)
		case errors.Is(err, data.ErrRecordAlreadyExists):
			httperror.Conflict("the user already holds this role on the wallet", err, nil).Render(rw)
		case errors.Is(err, data.ErrRecordNotFound):
			httperror.NotFound("distribution wallet not found", err, nil).Render(rw)
		case errors.Is(err, data.ErrMissingInput):
			httperror.BadRequest("user_id and a wallet-scopable role are required", err, nil).Render(rw)
		default:
			httperror.InternalError(ctx, "Cannot grant wallet membership", err, nil).Render(rw)
		}
		return
	}

	httpjson.RenderStatus(rw, http.StatusCreated, membership, httpjson.JSON)
}

// ensureGrantIsNotInert rejects a grant that would yield the grantee no capability at all on
// the wallet. Deliberately not "the grantee must already hold the role globally": a global
// financial controller granted approver here (approve, but do not create) is the feature's
// principal use, and that pair does yield capabilities.
func (h DistributionWalletsHandler) ensureGrantIsNotInert(ctx context.Context, granteeID string, role data.UserRole) *httperror.HTTPError {
	grantee, err := h.AuthManager.GetUserByID(ctx, granteeID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return httperror.BadRequest(fmt.Sprintf("user %q not found", granteeID), err, nil)
		}
		return httperror.InternalError(ctx, "Cannot get the grantee", err, nil)
	}

	if walletGrantIsInert(grantee, role) {
		return httperror.BadRequest(inertGrantReason(role), nil, nil)
	}
	return nil
}

// GetDistributionWalletCapabilities returns what the caller may actually do on one account.
// Authorization is global role first and membership second, so there is no single "effective
// role" to report — the required-role matrix stays server-side and this is its computed
// projection, which is what the UI gates create/start/cancel affordances on.
func (h DistributionWalletsHandler) GetDistributionWalletCapabilities(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

	user, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			httperror.Unauthorized("", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot get user from context", err, nil).Render(rw)
		return
	}

	scope, scopeErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if scopeErr != nil {
		scopeErr.Render(rw)
		return
	}
	if !walletInReadScope(scope, walletID) {
		httperror.NotFound("distribution wallet not found", nil, nil).Render(rw)
		return
	}

	if _, walletErr := h.Service.GetWallet(ctx, walletID); walletErr != nil {
		if errors.Is(walletErr, data.ErrRecordNotFound) {
			httperror.NotFound("distribution wallet not found", walletErr, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot load distribution wallet", walletErr, nil).Render(rw)
		return
	}

	// A nil scope is an Owner: tenant-wide, holds no membership rows, and the membership gate
	// never applies to them — so there is nothing to look up.
	var membershipRoles []data.UserRole
	if scope != nil {
		memberships, listErr := h.Models.WalletMemberships.ListByUser(ctx, h.Models.DBConnectionPool, user.ID)
		if listErr != nil {
			httperror.InternalError(ctx, "Cannot load wallet memberships", listErr, nil).Render(rw)
			return
		}
		for _, membership := range memberships {
			if membership.WalletID == walletID {
				membershipRoles = append(membershipRoles, membership.Role)
			}
		}
	}

	httpjson.Render(rw, WalletCapabilitiesResponse{
		WalletID:     walletID,
		Capabilities: walletCapabilitiesFor(user, membershipRoles),
	}, httpjson.JSON)
}

// WalletCapabilitiesResponse is one caller's computed capability set on one account. The keys
// are the walletCapabilityMatrix entries; a missing key means the capability does not exist in
// this build, not that it is denied.
type WalletCapabilitiesResponse struct {
	WalletID     string          `json:"wallet_id"`
	Capabilities map[string]bool `json:"capabilities"`
}

// DeleteDistributionWalletMembership revokes a membership; the append-only audit table keeps
// the full history.
func (h DistributionWalletsHandler) DeleteDistributionWalletMembership(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")
	membershipID := chi.URLParam(req, "membershipID")

	revoker, httpErr := h.ensureCallerIsOwner(ctx)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	if err := h.Service.RevokeMembership(ctx, walletID, membershipID, revoker.ID); err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("membership not found", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot revoke wallet membership", err, nil).Render(rw)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// DistributionWalletRequest is the request body to create a distribution wallet. The account
// type is fixed to DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT in v1 and immutable after creation.
type DistributionWalletRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// PostDistributionWallet creates and provisions a new distribution wallet: the wallet is
// independently funded and its secret material is isolated from sibling wallets.
func (h DistributionWalletsHandler) PostDistributionWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	if _, httpErr := h.ensureCallerIsOwner(ctx); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	var reqBody DistributionWalletRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(rw)
		return
	}

	wallet, err := h.Service.CreateWallet(ctx, data.DistributionWalletInsert{
		Name:        reqBody.Name,
		Description: reqBody.Description,
	})
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordAlreadyExists):
			httperror.Conflict("a distribution wallet with this name already exists", err, nil).Render(rw)
		case errors.Is(err, services.ErrDistributionWalletCapExceeded):
			httperror.BadRequest(services.ErrDistributionWalletCapExceeded.Error(), err, nil).Render(rw)
		case errors.Is(err, services.ErrUnsupportedDistributionWalletType):
			httperror.BadRequest(services.ErrUnsupportedDistributionWalletType.Error(), err, nil).Render(rw)
		case errors.Is(err, services.ErrTenantNotEligibleForMultiWallet):
			httperror.BadRequest(services.ErrTenantNotEligibleForMultiWallet.Error(), err, nil).Render(rw)
		case errors.Is(err, data.ErrMissingInput):
			httperror.BadRequest("name is required", err, nil).Render(rw)
		default:
			httperror.InternalError(ctx, "Cannot create distribution wallet", err, nil).Render(rw)
		}
		return
	}

	httpjson.RenderStatus(rw, http.StatusCreated, wallet, httpjson.JSON)
}

// GetDistributionWallets lists the tenant's distribution wallets. Archived wallets are hidden
// from this operational surface unless include_archived=true (audit/admin views).
func (h DistributionWalletsHandler) GetDistributionWallets(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	includeArchived := req.URL.Query().Get("include_archived") == "true"

	// Membership-filtered visibility (read taxonomy): Owners list every wallet; members list
	// only wallets they hold a membership on (the wallet picker's data source).
	scope, scopeErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if scopeErr != nil {
		scopeErr.Render(rw)
		return
	}

	wallets, err := h.Service.ListWallets(ctx, includeArchived)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve distribution wallets", err, nil).Render(rw)
		return
	}

	if scope != nil {
		visible := make([]data.DistributionWallet, 0, len(wallets))
		for i := range wallets {
			if walletInReadScope(scope, wallets[i].ID) {
				visible = append(visible, wallets[i])
			}
		}
		wallets = visible
	}

	httpjson.Render(rw, wallets, httpjson.JSON)
}

// PostArchiveDistributionWallet archives a wallet (no new disbursements; history intact).
// The default wallet must be promoted away first, and the tenant always keeps at least one
// active wallet.
func (h DistributionWalletsHandler) PostArchiveDistributionWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := chi.URLParam(req, "id")

	if _, httpErr := h.ensureCallerIsOwner(ctx); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	wallet, err := h.Service.ArchiveWallet(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			httperror.NotFound("distribution wallet not found or already archived", err, nil).Render(rw)
		case errors.Is(err, services.ErrCannotArchiveDefaultWallet):
			httperror.BadRequest(services.ErrCannotArchiveDefaultWallet.Error(), err, nil).Render(rw)
		case errors.Is(err, services.ErrCannotArchiveLastActiveWallet):
			httperror.BadRequest(services.ErrCannotArchiveLastActiveWallet.Error(), err, nil).Render(rw)
		default:
			httperror.InternalError(ctx, "Cannot archive distribution wallet", err, nil).Render(rw)
		}
		return
	}

	httpjson.Render(rw, wallet, httpjson.JSON)
}

// PostPromoteDistributionWalletToDefault atomically promotes an active wallet to default:
// demotes the old default, promotes the new one, and reassigns default-bound associations —
// all-or-nothing.
func (h DistributionWalletsHandler) PostPromoteDistributionWalletToDefault(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := chi.URLParam(req, "id")

	if _, httpErr := h.ensureCallerIsOwner(ctx); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	wallet, err := h.Service.PromoteToDefault(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrCannotPromoteWallet):
			httperror.BadRequest(services.ErrCannotPromoteWallet.Error(), err, nil).Render(rw)
		case errors.Is(err, data.ErrRecordNotFound):
			httperror.NotFound("distribution wallet not found", err, nil).Render(rw)
		default:
			httperror.InternalError(ctx, "Cannot promote distribution wallet to default", err, nil).Render(rw)
		}
		return
	}

	httpjson.Render(rw, wallet, httpjson.JSON)
}

// GetDistributionWallet returns one distribution wallet by id. Scope-checked here for the same
// reason as the membership list.
func (h DistributionWalletsHandler) GetDistributionWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := chi.URLParam(req, "id")

	if httpErr := h.ensureWalletInReadScope(ctx, id); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	wallet, err := h.Service.GetWallet(ctx, id)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("distribution wallet not found", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot retrieve distribution wallet", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, wallet, httpjson.JSON)
}
