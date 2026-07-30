package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	ctxHelper "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// DistributionWalletsHandler exposes Owner-only CRUD for the tenant's distribution wallets
// (the sending accounts — not the recipient wallet providers served by WalletsHandler).
type DistributionWalletsHandler struct {
	Service                    services.DistributionWalletManagementServiceInterface
	AuthManager                auth.AuthManager
	Models                     *data.Models
	DistributionAccountService services.DistributionAccountServiceInterface
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

func (h DistributionWalletsHandler) walletBalances(ctx context.Context, wallet *data.DistributionWallet) (map[string]string, *httperror.HTTPError) {
	if wallet.Address == nil || *wallet.Address == "" {
		// Pending-activation wallets have no on-chain account yet.
		return map[string]string{}, nil
	}

	account := schema.TransactionAccount{
		Address: *wallet.Address,
		Type:    wallet.AccountType,
		Status:  wallet.AccountStatus,
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

// GetDistributionWalletMemberships lists a wallet's memberships (admin/audit surface —
// archived wallets remain queryable).
func (h DistributionWalletsHandler) GetDistributionWalletMemberships(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

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
// wallets remain queryable).
func (h DistributionWalletsHandler) GetDistributionWalletAudit(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")

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

	grantor, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			httperror.Unauthorized("", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot get user from context", err, nil).Render(rw)
		return
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

// DeleteDistributionWalletMembership revokes a membership; the append-only audit table keeps
// the full history.
func (h DistributionWalletsHandler) DeleteDistributionWalletMembership(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	walletID := chi.URLParam(req, "id")
	membershipID := chi.URLParam(req, "membershipID")

	revoker, err := ctxHelper.GetUserFromContext(ctx, h.AuthManager)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			httperror.Unauthorized("", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Cannot get user from context", err, nil).Render(rw)
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

// GetDistributionWallet returns one distribution wallet by id.
func (h DistributionWalletsHandler) GetDistributionWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := chi.URLParam(req, "id")

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
