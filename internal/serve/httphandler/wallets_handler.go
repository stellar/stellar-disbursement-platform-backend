package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type WalletsHandler struct {
	Models                *data.Models
	NetworkType           utils.NetworkType
	AssetResolver         *services.AssetResolver
	EnableEmbeddedWallets bool
}

// GetWallets returns a list of wallets
func (h WalletsHandler) GetWallets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	filters, err := h.parseFilters(r)
	if err != nil {
		extras := map[string]interface{}{"validation_error": err.Error()}
		httperror.BadRequest("Error parsing request filters", nil, extras).Render(w)
		return
	}

	wallets, err := h.Models.Wallets.FindWallets(ctx, filters...)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve list of wallets", err, nil).Render(w)
		return
	}
	httpjson.Render(w, wallets, httpjson.JSON)
}

func (h WalletsHandler) parseFilters(r *http.Request) ([]data.Filter, error) {
	filters := []data.Filter{}
	filterParams := map[string]data.FilterKey{
		"enabled":      data.FilterEnabledWallets,
		"user_managed": data.FilterUserManaged,
	}

	for param, filterType := range filterParams {
		paramValue, err := utils.ParseBoolQueryParam(r, param)
		if err != nil {
			return nil, fmt.Errorf("invalid '%s' parameter value", param)
		}
		if paramValue != nil {
			filters = append(filters, data.NewFilter(filterType, *paramValue))
		}
	}
	return filters, nil
}

func (h WalletsHandler) PostWallets(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody *validators.WalletRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	validator := validators.NewWalletValidator()
	reqBody = validator.ValidateCreateWalletRequest(ctx, reqBody, h.NetworkType.IsPubnet())
	if validator.HasErrors() {
		httperror.BadRequest("invalid request body", nil, validator.Errors).Render(rw)
		return
	}

	// Resolve asset references to IDs
	var assetIDs []string
	var err error

	if len(reqBody.Assets) > 0 {
		assetIDs, err = h.AssetResolver.ResolveAssetReferences(ctx, reqBody.Assets)
		if err != nil {
			httperror.BadRequest("failed to resolve asset references", err, nil).Render(rw)
			return
		}
	} else if len(reqBody.AssetsIDs) > 0 {
		if err = h.AssetResolver.ValidateAssetIDs(ctx, reqBody.AssetsIDs); err != nil {
			httperror.BadRequest("invalid asset ID", err, nil).Render(rw)
			return
		}
		assetIDs = reqBody.AssetsIDs
	}

	walletInsert := data.WalletInsert{
		Name:              reqBody.Name,
		Homepage:          reqBody.Homepage,
		SEP10ClientDomain: reqBody.SEP10ClientDomain,
		DeepLinkSchema:    reqBody.DeepLinkSchema,
		AssetsIDs:         assetIDs,
		Enabled:           *reqBody.Enabled,
	}

	wallet, err := h.Models.Wallets.Insert(ctx, walletInsert)
	if err != nil {
		h.handleWalletError(ctx, rw, err, "failed to create wallet")
		return
	}

	wallet.Assets, err = h.Models.Wallets.GetAssets(ctx, wallet.ID)
	if err != nil {
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusCreated, wallet, httpjson.JSON)
}

func (c WalletsHandler) DeleteWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	walletID := chi.URLParam(req, "id")

	_, err := c.Models.Wallets.SoftDelete(ctx, walletID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusNoContent, nil, httpjson.JSON)
}

func (h WalletsHandler) PatchWallets(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody *validators.PatchWalletRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	validator := validators.NewWalletValidator()
	reqBody = validator.ValidatePatchWalletRequest(ctx, reqBody, h.NetworkType.IsPubnet())
	if validator.HasErrors() {
		httperror.BadRequest("invalid request body", nil, validator.Errors).Render(rw)
		return
	}

	walletID := chi.URLParam(req, "id")

	// Check if trying to enable an embedded wallet without the feature flag
	if reqBody.Enabled != nil && *reqBody.Enabled {
		wallet, err := h.Models.Wallets.Get(ctx, walletID)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				httperror.NotFound("", err, nil).Render(rw)
				return
			}
			httperror.InternalError(ctx, "failed to retrieve wallet", err, nil).Render(rw)
			return
		}

		if wallet.Embedded && !h.EnableEmbeddedWallets {
			extra := map[string]interface{}{"validation_error": "embedded wallets cannot be enabled when --enable-embedded-wallets is false"}
			httperror.BadRequest("cannot enable embedded wallet provider", nil, extra).Render(rw)
			return
		}
	}

	update := data.WalletUpdate{
		Name:              reqBody.Name,
		Homepage:          reqBody.Homepage,
		SEP10ClientDomain: reqBody.SEP10ClientDomain,
		DeepLinkSchema:    reqBody.DeepLinkSchema,
		Enabled:           reqBody.Enabled,
	}

	if reqBody.Assets != nil {
		assetIDs, err := h.AssetResolver.ResolveAssetReferences(ctx, *reqBody.Assets)
		if err != nil {
			httperror.BadRequest("failed to resolve asset references", err, nil).Render(rw)
			return
		}
		update.AssetsIDs = &assetIDs
	}
	wallet, err := h.Models.Wallets.Update(ctx, walletID, update)
	if err != nil {
		h.handleWalletError(ctx, rw, err, "failed to update wallet")
		return
	}

	httpjson.Render(rw, wallet, httpjson.JSON)
}

func (h WalletsHandler) handleWalletError(ctx context.Context, rw http.ResponseWriter, err error, defaultMsg string) {
	switch {
	case errors.Is(err, data.ErrRecordNotFound):
		httperror.NotFound("", err, nil).Render(rw)
	case errors.Is(err, data.ErrInvalidAssetID):
		httperror.BadRequest(data.ErrInvalidAssetID.Error(), err, nil).Render(rw)
	case errors.Is(err, data.ErrWalletNameAlreadyExists):
		httperror.Conflict(data.ErrWalletNameAlreadyExists.Error(), err, nil).Render(rw)
	case errors.Is(err, data.ErrWalletHomepageAlreadyExists):
		httperror.Conflict(data.ErrWalletHomepageAlreadyExists.Error(), err, nil).Render(rw)
	case errors.Is(err, data.ErrWalletDeepLinkSchemaAlreadyExists):
		httperror.Conflict(data.ErrWalletDeepLinkSchemaAlreadyExists.Error(), err, nil).Render(rw)
	default:
		httperror.InternalError(ctx, defaultMsg, err, nil).Render(rw)
	}
}
