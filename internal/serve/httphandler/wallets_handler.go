package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type WalletsHandler struct {
	Models      *data.Models
	NetworkType utils.NetworkType
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

	wallet, err := h.Models.Wallets.Insert(ctx, data.WalletInsert{
		Name:              reqBody.Name,
		Homepage:          reqBody.Homepage,
		SEP10ClientDomain: reqBody.SEP10ClientDomain,
		DeepLinkSchema:    reqBody.DeepLinkSchema,
		AssetsIDs:         reqBody.AssetsIDs,
	})
	if err != nil {
		switch {
		case errors.Is(err, data.ErrInvalidAssetID):
			httperror.BadRequest(data.ErrInvalidAssetID.Error(), err, nil).Render(rw)
			return
		case errors.Is(err, data.ErrWalletNameAlreadyExists):
			httperror.Conflict(data.ErrWalletNameAlreadyExists.Error(), err, nil).Render(rw)
			return
		case errors.Is(err, data.ErrWalletHomepageAlreadyExists):
			httperror.Conflict(data.ErrWalletHomepageAlreadyExists.Error(), err, nil).Render(rw)
			return
		case errors.Is(err, data.ErrWalletDeepLinkSchemaAlreadyExists):
			httperror.Conflict(data.ErrWalletDeepLinkSchemaAlreadyExists.Error(), err, nil).Render(rw)
			return
		}

		httperror.InternalError(ctx, "", err, nil).Render(rw)
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
	validator.ValidatePatchWalletRequest(reqBody)
	if validator.HasErrors() {
		httperror.BadRequest("invalid request body", nil, validator.Errors).Render(rw)
		return
	}

	walletID := chi.URLParam(req, "id")

	_, err := h.Models.Wallets.Update(ctx, walletID, *reqBody.Enabled)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("", err, nil).Render(rw)
			return
		}
		err = fmt.Errorf("updating wallet: %w", err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, map[string]string{"message": "wallet updated successfully"}, httpjson.JSON)
}
