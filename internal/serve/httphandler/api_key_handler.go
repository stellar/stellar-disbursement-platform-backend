package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

// APIKeyCacheInvalidator is implemented by whatever caches validated API keys in
// front of the DB (see middleware.APIKeyAuthenticator). It lets this handler force
// an immediate cache eviction whenever it changes a key's permissions/allowed_ips
// or deletes the key outright, so the change is enforced on the very next request.
// ctx is passed through so the invalidator can scope the cache entry to the
// request's tenant, matching how it was originally cached.
type APIKeyCacheInvalidator interface {
	Invalidate(ctx context.Context, id string)
}

type APIKeyHandler struct {
	Models      *data.Models
	AuthManager auth.AuthManager
	// CacheInvalidator is optional. If nil, no cache invalidation is attempted
	// (e.g. in tests that exercise the handler without the auth middleware's cache).
	CacheInvalidator APIKeyCacheInvalidator
}

type CreateAPIKeyRequest struct {
	Name                  string                  `json:"name"`
	Permissions           []data.APIKeyPermission `json:"permissions"`
	ExpiryDate            *time.Time              `json:"expiry_date,omitempty"`
	AllowedIPs            any                     `json:"allowed_ips,omitempty"` // Can be a string or array of strings
	DistributionWalletIDs []string                `json:"distribution_wallet_ids,omitempty"`
}

// normalizeWalletIDs sorts and dedupes a requested scope, never returning nil: slices.Sorted over
// an empty request yields nil, which downstream reads as "leave the scope untouched" — the opposite
// of the explicit [] the caller sent.
func normalizeWalletIDs(requested []string) []string {
	normalized := slices.Compact(slices.Sorted(slices.Values(requested)))
	if normalized == nil {
		return []string{}
	}
	return normalized
}

// grantableWallets returns the wallets the caller may put on a key, for creation and for edits.
//
// Only active wallets can be added, mirroring the enforce_wallet_membership_wallet_active trigger
// on wallet_memberships: an archived account takes no new grants, whether the grantee is a person
// or a key. alreadyHeld (the key's current scope, empty on creation) is admitted regardless, so an
// account archived after the fact stays on its key — keeping it grants nothing new, and refusing it
// would make an unrelated permissions edit impossible to save. Such a wallet goes on serving reads
// and failing writes, exactly as it does for a membership that predates the archive.
//
// Minting via API key is capped at the acting key's own scope, so a key cannot mint a broader one.
func (h APIKeyHandler) grantableWallets(ctx context.Context, alreadyHeld []string) ([]string, *httperror.HTTPError) {
	wallets, err := h.Models.DistributionWallets.GetAll(ctx, h.Models.DBConnectionPool, false)
	if err != nil {
		return nil, httperror.InternalError(ctx, "Cannot list distribution wallets", err, nil)
	}
	activeIDs := make([]string, 0, len(wallets))
	for _, wallet := range wallets {
		activeIDs = append(activeIDs, wallet.ID)
	}

	withinReach := func(reach []string) []string {
		grantable := slices.Clone(alreadyHeld)
		for _, id := range reach {
			if slices.Contains(activeIDs, id) && !slices.Contains(grantable, id) {
				grantable = append(grantable, id)
			}
		}
		return grantable
	}

	if actingKey, keyErr := sdpcontext.GetAPIKeyFromContext(ctx); keyErr == nil && actingKey != nil {
		return withinReach(actingKey.WalletScope()), nil
	}

	scope, httpErr := resolveWalletReadScope(ctx, h.AuthManager, h.Models)
	if httpErr != nil {
		return nil, httpErr
	}
	if scope == nil {
		// Owners hold no membership rows, so their reach is the active set itself.
		return withinReach(activeIDs), nil
	}
	return withinReach(scope), nil
}

type UpdateAPIKeyRequest struct {
	Permissions []data.APIKeyPermission `json:"permissions"`
	AllowedIPs  any                     `json:"allowed_ips,omitempty"` // Can be a string or array of strings
	// Omitted leaves the key's wallet scope untouched; an explicit list replaces it.
	DistributionWalletIDs []string `json:"distribution_wallet_ids,omitempty"`
}

func (h APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateAPIKeyRequest
	if err := httpdecode.DecodeJSON(r, &req); err != nil {
		httperror.BadRequest("Invalid request body", err, nil).Render(w)
		return
	}

	v := validators.NewValidator()

	v.Check(req.Name != "", "name", "name is required")
	v.Check(len(req.Permissions) > 0, "permissions", "API key must have at least one permission assigned")
	if err := data.ValidatePermissions(req.Permissions); err != nil {
		v.AddError("permissions", err.Error())
	}

	allowedIPs, err := parseAllowedIPs(req.AllowedIPs)
	if err != nil {
		v.AddError("allowed_ips", err.Error())
	} else if validationErr := data.ValidateAllowedIPs(allowedIPs); validationErr != nil {
		v.AddError("allowed_ips", validationErr.Error())
	}

	if req.ExpiryDate != nil && req.ExpiryDate.Before(time.Now()) {
		v.AddError("expiry_date", "expiry date must be in the future")
	}

	if v.HasErrors() {
		httperror.BadRequest("Request validation failed", nil, v.Errors).Render(w)
		return
	}

	// Omitting the field entirely inherits the creator's own access, so a client written before
	// wallet scoping existed still gets a working key. An explicit [] means "no wallet access" and
	// is honored as written.
	grantable, scopeErr := h.grantableWallets(ctx, nil)
	if scopeErr != nil {
		scopeErr.Render(w)
		return
	}

	var walletIDs []string
	if req.DistributionWalletIDs == nil {
		walletIDs = grantable
	} else {
		walletIDs = normalizeWalletIDs(req.DistributionWalletIDs)
		for _, walletID := range walletIDs {
			if !slices.Contains(grantable, walletID) {
				httperror.Forbidden(
					"a key cannot be scoped to a distribution account the creator has no access to", nil, nil).Render(w)
				return
			}
		}
	}

	userID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	apiKey, err := h.Models.APIKeys.Insert(
		ctx,
		req.Name,
		req.Permissions,
		allowedIPs,
		walletIDs,
		req.ExpiryDate,
		userID,
	)
	if err != nil {
		httperror.InternalError(ctx, "Failed to create API key", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusCreated, apiKey, httpjson.JSON)
}

func (h APIKeyHandler) GetAPIKeyByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := chi.URLParam(r, "id")

	userID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	key, err := h.Models.APIKeys.GetByID(ctx, keyID, userID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("API key not found", nil, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Failed to retrieve API key", err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, key, httpjson.JSON)
}

func (h APIKeyHandler) GetAllAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	apiKeys, err := h.Models.APIKeys.GetAll(ctx, userID)
	if err != nil {
		httperror.InternalError(ctx, "Failed to retrieve API keys", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, apiKeys, httpjson.JSON)
}

func (h APIKeyHandler) UpdateKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := chi.URLParam(r, "id")

	var req UpdateAPIKeyRequest
	if err := httpdecode.DecodeJSON(r, &req); err != nil {
		httperror.BadRequest("Invalid request body", err, nil).Render(w)
		return
	}

	if len(req.Permissions) == 0 {
		httperror.BadRequest("permissions must be non-empty", nil, nil).Render(w)
		return
	}
	if err := data.ValidatePermissions(req.Permissions); err != nil {
		httperror.BadRequest("Invalid permissions", err, nil).Render(w)
		return
	}

	ips, err := parseAllowedIPs(req.AllowedIPs)
	if err != nil {
		httperror.BadRequest("Invalid allowed_ips", err, nil).Render(w)
		return
	}

	if validationErr := data.ValidateAllowedIPs(ips); validationErr != nil {
		httperror.BadRequest("Invalid allowed_ips", validationErr, nil).Render(w)
		return
	}

	userID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	// Omitting the field leaves the key's scope untouched, so there is nothing to authorize.
	var walletIDs []string
	if req.DistributionWalletIDs != nil {
		current, getErr := h.Models.APIKeys.GetByID(ctx, keyID, userID)
		if getErr != nil {
			if errors.Is(getErr, data.ErrRecordNotFound) {
				httperror.NotFound("API key not found", nil, nil).Render(w)
			} else {
				httperror.InternalError(ctx, "Failed to retrieve API key", getErr, nil).Render(w)
			}
			return
		}

		walletIDs = normalizeWalletIDs(req.DistributionWalletIDs)
		grantable, scopeErr := h.grantableWallets(ctx, current.WalletScope())
		if scopeErr != nil {
			scopeErr.Render(w)
			return
		}
		for _, walletID := range walletIDs {
			if !slices.Contains(grantable, walletID) {
				httperror.Forbidden(
					"a key cannot be scoped to a distribution account the editor has no access to", nil, nil).Render(w)
				return
			}
		}
	}

	updated, err := h.Models.APIKeys.Update(ctx, keyID, userID, req.Permissions, ips, walletIDs)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("API key not found", nil, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Failed to update API key", err, nil).Render(w)
		}
		return
	}

	// The permissions/allowed_ips/wallet scope just written must be enforced on the very next
	// request that uses this key, not whenever the auth cache's TTL happens to
	// expire - so evict it now.
	if h.CacheInvalidator != nil {
		h.CacheInvalidator.Invalidate(ctx, keyID)
	}

	httpjson.RenderStatus(w, http.StatusOK, updated, httpjson.JSON)
}

func (h APIKeyHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := chi.URLParam(r, "id")

	userID, err := sdpcontext.GetUserIDFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "User identification error", nil, nil).Render(w)
		return
	}

	if err := h.Models.APIKeys.Delete(ctx, keyID, userID); err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("API key not found", nil, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Failed to delete API key", err, nil).Render(w)
		}
		return
	}

	// A deleted key must stop working on the very next request too - a cached,
	// pre-delete validation result must not keep authorizing it.
	if h.CacheInvalidator != nil {
		h.CacheInvalidator.Invalidate(ctx, keyID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseAllowedIPs converts the allowed_ips field from the request into a string slice.
func parseAllowedIPs(input any) ([]string, error) {
	if input == nil {
		return []string{}, nil
	}

	if strArray, ok := input.([]string); ok {
		return strArray, nil
	}

	if arr, ok := input.([]any); ok {
		strArray := make([]string, 0, len(arr))
		for i, item := range arr {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("item at position %d must be a string", i)
			}
			strArray = append(strArray, str)
		}
		return strArray, nil
	}

	if str, ok := input.(string); ok {
		return []string{str}, nil
	}

	return nil, fmt.Errorf("must be a string or array of strings")
}
