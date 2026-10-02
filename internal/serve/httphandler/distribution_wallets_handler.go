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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// DistributionWalletsHandler exposes CRUD for the tenant's distribution wallets (the sending accounts —
// not the recipient wallet providers served by WalletsHandler): Owner-only for JWTs, scope-bound for API keys.
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
// taxonomy exactly like /statistics: owners and developers sum every wallet (the tenant-wide
// view); members sum only the wallets they hold memberships on. No caller ever sees a
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
		// walletInReadScope returns true for a nil (tenant-wide) scope, so no extra guard.
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

// GetDistributionWalletMemberships lists a wallet's memberships (admin/audit surface — archived wallets
// remain queryable). JWTs are Owner-gated at the route; API keys are bound to their scope here.
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

// GetDistributionWalletAudit returns a wallet's grant/revoke history from the append-only audit table, newest first.
// Archived wallets remain queryable; gated as GetDistributionWalletMemberships.
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

	// Scope first, so an out-of-scope id cannot be probed through the body or grantee checks below.
	if httpErr := h.ensureWalletInReadScope(ctx, walletID); httpErr != nil {
		httpErr.Render(rw)
		return
	}
	grantorID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "User identification error", err, nil).Render(rw)
		return
	}

	var reqBody WalletMembershipRequest
	if err = httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(rw)
		return
	}

	// Validate the role up front: an unknown role would otherwise pass the service checks, hit
	// the wallet_memberships CHECK constraint, and surface to the operator as a 500. Owner and
	// developer are tenant-wide and cannot be granted per wallet.
	role, roleErr := parseWalletScopableRole(reqBody.Role)
	if roleErr != nil {
		roleErr.Render(rw)
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

	membership, err := h.Service.GrantMembership(ctx, walletID, reqBody.UserID, role, grantorID)
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

// parseWalletScopableRole validates a role about to be granted on a wallet, or asked about hypothetically.
// Owner and developer are tenant-wide, so they are rejected alongside unknown values.
func parseWalletScopableRole(raw string) (data.UserRole, *httperror.HTTPError) {
	role := data.UserRole(raw)
	scopableRoles := data.GetWalletScopableRoles()
	if slices.Contains(scopableRoles, role) {
		return role, nil
	}

	return "", httperror.BadRequest(
		fmt.Sprintf("unexpected value for role=%q. Expect one of these values: %s", raw, scopableRoles),
		nil, nil,
	)
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

	if walletGrantIsInert(grantee) {
		granteeRole := data.OwnerUserRole
		if !grantee.IsOwner && len(grantee.Roles) > 0 {
			granteeRole = data.UserRole(grantee.Roles[0])
		}
		return httperror.BadRequest(inertGrantReason(role, granteeRole), nil, nil)
	}
	return nil
}

// GetDistributionWalletCapabilities returns what a user may actually do on one account.
// Authorization is global role first and membership second, so there is no single "effective
// role" to report — the required-role matrix stays server-side and this is its computed
// projection, which is what the UI gates create/start/cancel affordances on.
//
// Three readings, selected by query parameter:
//
//	(no parameters)     the CALLER's capabilities, from their own real memberships. Any business
//	                    role may ask, and the response shape is exactly what shipped.
//	?user_id=U          U's capabilities here, from U's real memberships.
//	?user_id=U&role=R   hypothetical: what U would have here if granted R. Computed from R
//	                    alone — not R unioned with U's existing rows — because the question the
//	                    grant picker asks is what THIS grant would yield, and a pre-existing row
//	                    would otherwise make an inert role look productive.
//
// The two parameterized readings disclose a third party's authority, so for JWTs they are Owner-only
// (the grant picker's data source); API keys reach them through read:distribution_wallets and scope.
// The owner check runs before the role is parsed, the wallet is loaded or the subject is looked up,
// so a non-owner turns none of those into an oracle. `role` without `user_id` has no subject to
// be hypothetical about and is a 400 rather than a silent fallback to the caller.
func (h DistributionWalletsHandler) GetDistributionWalletCapabilities(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

	subjectID := req.URL.Query().Get("user_id")
	rawRole := req.URL.Query().Get("role")

	// subject is the user the capabilities are computed for: the caller by default, the
	// requested user_id otherwise (resolved further down, once the wallet has been validated).
	var subject *auth.User
	if subjectID == "" && rawRole == "" {
		user, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				httperror.Unauthorized("", err, nil).Render(rw)
				return
			}
			httperror.InternalError(ctx, "Cannot get user from context", err, nil).Render(rw)
			return
		}
		subject = user
	} else {
		if ownerErr := h.ensureJWTCallerIsOwner(ctx); ownerErr != nil {
			ownerErr.Render(rw)
			return
		}
		if subjectID == "" {
			httperror.BadRequest("user_id is required when role is provided", nil, nil).Render(rw)
			return
		}
	}

	var hypotheticalRole data.UserRole
	if rawRole != "" {
		parsedRole, roleErr := parseWalletScopableRole(rawRole)
		if roleErr != nil {
			roleErr.Render(rw)
			return
		}
		hypotheticalRole = parsedRole
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

	// The subject is looked up only after the wallet has been shown to exist, mirroring the
	// grant handler's order: a request-supplied user_id is answered only to Owners or keys holding
	// read:distribution_wallets, and never before the path's wallet is established.
	if subjectID != "" {
		grantee, granteeErr := h.AuthManager.GetUserByID(ctx, subjectID)
		if granteeErr != nil {
			if errors.Is(granteeErr, auth.ErrUserNotFound) {
				httperror.BadRequest(fmt.Sprintf("user %q not found", subjectID), granteeErr, nil).Render(rw)
				return
			}
			httperror.InternalError(ctx, "Cannot get the subject user", granteeErr, nil).Render(rw)
			return
		}
		subject = grantee
	}

	// Tenant-wide users hold no membership rows, and a hypothetical reports only the proposed role.
	var membershipRoles []data.UserRole
	if hypotheticalRole != "" {
		membershipRoles = []data.UserRole{hypotheticalRole}
	} else if !services.IsTenantWideUser(subject) {
		roles, rolesErr := h.walletMembershipRoles(ctx, subject.ID, walletID)
		if rolesErr != nil {
			rolesErr.Render(rw)
			return
		}
		membershipRoles = roles
	}

	httpjson.Render(rw, WalletCapabilitiesResponse{
		WalletID:     walletID,
		UserID:       subjectID,
		Role:         hypotheticalRole.String(),
		Capabilities: walletCapabilitiesFor(subject, membershipRoles),
	}, httpjson.JSON)
}

// walletMembershipRoles returns the roles a user actually holds on one wallet.
func (h DistributionWalletsHandler) walletMembershipRoles(ctx context.Context, userID, walletID string) ([]data.UserRole, *httperror.HTTPError) {
	memberships, err := h.Models.WalletMemberships.ListByUser(ctx, h.Models.DBConnectionPool, userID)
	if err != nil {
		return nil, httperror.InternalError(ctx, "Cannot load wallet memberships", err, nil)
	}

	var roles []data.UserRole
	for _, membership := range memberships {
		if membership.WalletID == walletID {
			roles = append(roles, membership.Role)
		}
	}
	return roles, nil
}

// ensureJWTCallerIsOwner mirrors an Owner route gate for a reading that shares an any-role route.
// API keys pass: RequirePermission has already checked them.
func (h DistributionWalletsHandler) ensureJWTCallerIsOwner(ctx context.Context) *httperror.HTTPError {
	if _, err := sdpcontext.GetAPIKeyFromContext(ctx); err == nil {
		return nil
	}

	user, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return httperror.Unauthorized("", err, nil)
		}
		return httperror.InternalError(ctx, "Cannot get user from context", err, nil)
	}
	if !user.IsOwner && !slices.Contains(user.Roles, string(data.OwnerUserRole)) {
		return httperror.Forbidden("", nil, nil)
	}
	return nil
}

// WalletCapabilitiesResponse is one subject's computed capability set on one account. The keys of
// Capabilities are the walletCapabilityMatrix entries; a missing key means the capability does
// not exist in this build, not that it is denied.
//
// UserID and Role echo the query parameters and are both absent from the caller's-own reading, so
// that response is byte-for-byte what shipped. A present Role marks the set as hypothetical: what
// the subject WOULD hold if granted that role here, not what they hold today.
type WalletCapabilitiesResponse struct {
	WalletID     string          `json:"wallet_id"`
	UserID       string          `json:"user_id,omitempty"`
	Role         string          `json:"role,omitempty"`
	Capabilities map[string]bool `json:"capabilities"`
}

// DeleteDistributionWalletMembership revokes a membership; the append-only audit table keeps
// the full history.
func (h DistributionWalletsHandler) DeleteDistributionWalletMembership(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")
	membershipID := chi.URLParam(req, "membershipID")

	if httpErr := h.ensureWalletInReadScope(ctx, walletID); httpErr != nil {
		httpErr.Render(rw)
		return
	}
	revokerID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "User identification error", err, nil).Render(rw)
		return
	}

	if err = h.Service.RevokeMembership(ctx, walletID, membershipID, revokerID); err != nil {
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

	// Membership-filtered visibility (read taxonomy): owners and developers list every wallet; members list
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

	if httpErr := h.ensureWalletInReadScope(ctx, id); httpErr != nil {
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

// PostPromoteDistributionWalletToDefault atomically demotes the old default, promotes this wallet and reassigns default-bound associations.
// A scoped caller also needs the demoted wallet in scope (403 otherwise).
func (h DistributionWalletsHandler) PostPromoteDistributionWalletToDefault(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := chi.URLParam(req, "id")

	scope, scopeErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if scopeErr != nil {
		scopeErr.Render(rw)
		return
	}
	if !walletInReadScope(scope, id) {
		httperror.NotFound("distribution wallet not found", nil, nil).Render(rw)
		return
	}

	wallet, err := h.Service.PromoteToDefault(ctx, id, scope)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrWalletActionForbidden):
			httperror.Forbidden(services.ErrWalletActionForbidden.Error(), err, nil).Render(rw)
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

// GetDistributionWallet returns one distribution wallet by id (the admin detail view), gated as
// GetDistributionWalletMemberships.
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
