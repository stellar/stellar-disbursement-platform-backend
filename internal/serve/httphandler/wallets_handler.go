package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type WalletsHandler struct {
	Models              *data.Models
	NetworkType         utils.NetworkType
	WalletAssetResolver *services.WalletAssetResolver
}

// GetWallets returns a list of wallets
func (h WalletsHandler) GetWallets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	filters, err := h.parseFilters(ctx, r)
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

func (h WalletsHandler) parseFilters(ctx context.Context, r *http.Request) ([]data.Filter, error) {
	filters := []data.Filter{}
	boolFilterParams := map[string]data.FilterKey{
		"enabled":         data.FilterEnabledWallets,
		"user_managed":    data.FilterUserManaged,
		"include_deleted": data.FilterIncludeDeleted,
	}

	for param, filterType := range boolFilterParams {
		paramValue, err := utils.ParseBoolQueryParam(r, param)
		if err != nil {
			return nil, fmt.Errorf("invalid '%s' parameter value", param)
		}
		if paramValue != nil {
			filters = append(filters, data.NewFilter(filterType, *paramValue))
		}
	}

	supportedAssetsParam := r.URL.Query().Get("supported_assets")
	if supportedAssetsParam != "" {
		f, err := h.parseSupportedAssetsParam(ctx, supportedAssetsParam)
		if err != nil {
			return nil, fmt.Errorf("parsing supported_assets parameter: %w", err)
		}
		if !utils.IsEmpty(f) {
			filters = append(filters, f)
		}
	}

	return filters, nil
}

const maxSupportedAssets = 20

// parseSupportedAssetsParam parses the supported_assets query parameter, validates it and returns a Filter.
func (h WalletsHandler) parseSupportedAssetsParam(ctx context.Context, supportedAssetsParam string) (data.Filter, error) {
	if supportedAssetsParam == "" {
		return data.Filter{}, nil
	}

	assetStrings := strings.Split(supportedAssetsParam, ",")
	var assets []string
	for _, assetStr := range assetStrings {
		asset := strings.TrimSpace(assetStr)
		if asset != "" {
			assets = append(assets, asset)
		}
	}

	if len(assets) > maxSupportedAssets {
		return data.Filter{}, fmt.Errorf("too many assets specified (max %d)", maxSupportedAssets)
	}

	if len(assets) > 0 {
		if err := h.validateAssetReferences(ctx, assets); err != nil {
			return data.Filter{}, fmt.Errorf("invalid asset reference in supported_assets: %w", err)
		}

		return data.NewFilter(data.FilterSupportedAssets, assets), nil
	}
	return data.Filter{}, nil
}

// validateAssetReferences validates that asset references (codes or IDs) exist in the database
func (h WalletsHandler) validateAssetReferences(ctx context.Context, assetReferences []string) error {
	for _, ref := range assetReferences {
		exists, err := h.Models.Assets.ExistsByCodeOrID(ctx, ref)
		if err != nil {
			return fmt.Errorf("validating asset '%s': %w", ref, err)
		}
		if !exists {
			return fmt.Errorf("asset '%s' not found", ref)
		}
	}
	return nil
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
		assetIDs, err = h.WalletAssetResolver.ResolveAssetReferences(ctx, reqBody.Assets)
		if err != nil {
			httperror.BadRequest("failed to resolve asset references", err, nil).Render(rw)
			return
		}
	} else if len(reqBody.AssetsIDs) > 0 {
		if err = h.WalletAssetResolver.ValidateAssetIDs(ctx, reqBody.AssetsIDs); err != nil {
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

func (h WalletsHandler) DeleteWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	walletID := chi.URLParam(req, "id")

	_, err := h.Models.Wallets.SoftDelete(ctx, walletID)
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

	update := data.WalletUpdate{
		Name:              reqBody.Name,
		Homepage:          reqBody.Homepage,
		SEP10ClientDomain: reqBody.SEP10ClientDomain,
		DeepLinkSchema:    reqBody.DeepLinkSchema,
		Enabled:           reqBody.Enabled,
	}

	if reqBody.Assets != nil {
		assetIDs, err := h.WalletAssetResolver.ResolveAssetReferences(ctx, *reqBody.Assets)
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
