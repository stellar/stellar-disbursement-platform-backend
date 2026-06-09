package httphandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

// DistributionWalletsHandler exposes Owner-only CRUD for the tenant's distribution wallets
// (the sending accounts — not the recipient wallet providers served by WalletsHandler).
type DistributionWalletsHandler struct {
	Service services.DistributionWalletManagementServiceInterface
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

	wallets, err := h.Service.ListWallets(ctx, includeArchived)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve distribution wallets", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, wallets, httpjson.JSON)
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
