package httphandler

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type WalletCreationHandler struct {
	embeddedWalletService services.EmbeddedWalletServiceInterface
}

type CreateWalletRequest struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
}

func (r CreateWalletRequest) Validate() *httperror.HTTPError {
	validator := validators.NewValidator()
	validator.Check(len(r.Token) > 0, "token", "token should not be empty")
	validator.Check(len(r.PublicKey) > 0, "public_key", "public_key should not be empty")
	if _, err := hex.DecodeString(r.PublicKey); err != nil {
		validator.AddError("public_key", "public_key should be a valid hex string")
	}

	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}

	return nil
}

type WalletResponse struct {
	ContractAddress string                    `json:"contract_address,omitempty"`
	Status          data.EmbeddedWalletStatus `json:"status"`
}

func (h WalletCreationHandler) getTenantFromContext(ctx http.Request) (*tenant.Tenant, *httperror.HTTPError) {
	currentTenant, err := tenant.GetTenantFromContext(ctx.Context())
	if err != nil || currentTenant == nil {
		return nil, httperror.InternalError(ctx.Context(), "Failed to load tenant from context", err, nil)
	}
	return currentTenant, nil
}

func (h WalletCreationHandler) CreateWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody CreateWalletRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.Validate(); err != nil {
		err.Render(rw)
		return
	}

	currentTenant, httpErr := h.getTenantFromContext(*req)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	err := h.embeddedWalletService.CreateWallet(ctx, currentTenant.ID, reqBody.Token, reqBody.PublicKey)
	if err != nil {
		switch err {
		case services.ErrCreateWalletInvalidToken:
			httperror.BadRequest("Invalid token", err, nil).Render(rw)
		case services.ErrCreateWalletInvalidStatus:
			httperror.BadRequest("Wallet status is not pending for token", err, nil).Render(rw)
		default:
			httperror.InternalError(ctx, "Failed to create wallet", err, nil).Render(rw)
		}
		return
	}

	resp := WalletResponse{
		Status: data.PendingWalletStatus,
	}
	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}

func (h WalletCreationHandler) GetWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	token := strings.TrimSpace(req.URL.Query().Get("token"))

	currentTenant, httpErr := h.getTenantFromContext(*req)
	if httpErr != nil {
		httpErr.Render(rw)
		return
	}

	wallet, err := h.embeddedWalletService.GetWallet(ctx, currentTenant.ID, token)
	if err != nil {
		if err == services.ErrGetWalletInvalidToken {
			httperror.BadRequest("Invalid token", err, nil).Render(rw)
			return
		}
		httperror.InternalError(ctx, "Failed to get wallet", err, nil).Render(rw)
		return
	}

	resp := WalletResponse{
		ContractAddress: wallet.ContractAddress,
		Status:          wallet.WalletStatus,
	}
	httpjson.RenderStatus(rw, http.StatusOK, resp, httpjson.JSON)
}
