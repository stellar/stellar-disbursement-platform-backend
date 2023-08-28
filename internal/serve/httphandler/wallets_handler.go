package httphandler

import (
	"errors"
	"net/http"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type WalletsHandler struct {
	Models *data.Models
}

// GetWallets returns a list of wallets
func (c WalletsHandler) GetWallets(w http.ResponseWriter, r *http.Request) {
	wallets, err := c.Models.Wallets.GetAll(r.Context())
	if err != nil {
		httperror.InternalError(r.Context(), "Cannot retrieve list of wallets", err, nil).Render(w)
		return
	}
	httpjson.Render(w, wallets, httpjson.JSON)
}

func (c WalletsHandler) PostWallets(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody validators.CreateWalletRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	validator := validators.NewWalletValidator()
	validator.ValidateCreateWalletRequest(&reqBody)
	if validator.HasErrors() {
		httperror.BadRequest("invalid request body", nil, validator.Errors).Render(rw)
		return
	}

	wallet, err := c.Models.Wallets.Insert(ctx, data.WalletInsert{
		Name:              reqBody.Name,
		Homepage:          reqBody.Homepage,
		SEP10ClientDomain: reqBody.SEP10ClientDomain,
		DeepLinkSchema:    reqBody.DeepLinkSchema,
		AssetsIDs:         reqBody.AssetsIDs,
		CountriesCodes:    reqBody.CountriesCodes,
	})
	if err != nil {
		switch {
		case errors.Is(err, data.ErrWalletNameAlreadyExists):
			httperror.Conflict(data.ErrWalletNameAlreadyExists.Error(), err, nil).Render(rw)
			return
		case errors.Is(err, data.ErrWalletHomepageAlreadyExists):
			httperror.Conflict(data.ErrWalletHomepageAlreadyExists.Error(), err, nil).Render(rw)
			return
		case errors.Is(err, data.ErrWalletDeepLinkSchemaAlreadyExists):
			httperror.Conflict(data.ErrWalletDeepLinkSchemaAlreadyExists.Error(), err, nil).Render(rw)
			return
		case errors.Is(err, data.ErrInvalidCountryCode):
			httperror.Conflict(data.ErrInvalidCountryCode.Error(), err, nil).Render(rw)
			return
		case errors.Is(err, data.ErrInvalidAssetID):
			httperror.Conflict(data.ErrInvalidAssetID.Error(), err, nil).Render(rw)
			return
		}

		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	wallet.Countries, err = c.Models.Wallets.GetCountries(ctx, wallet.ID)
	if err != nil {
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	wallet.Assets, err = c.Models.Wallets.GetAssets(ctx, wallet.ID)
	if err != nil {
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusCreated, wallet, httpjson.JSON)
}
