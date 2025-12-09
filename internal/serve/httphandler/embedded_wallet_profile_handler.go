package httphandler

import (
	"errors"
	"net/http"

	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type EmbeddedWalletProfileHandler struct {
	EmbeddedWalletService services.EmbeddedWalletServiceInterface
}

type EmbeddedWalletProfileResponse struct {
	IsVerificationPending bool        `json:"is_verification_pending"`
	PendingAsset          *data.Asset `json:"pending_asset,omitempty"`
}

func (h EmbeddedWalletProfileHandler) GetProfile(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	contractAddress, err := sdpcontext.GetWalletContractAddressFromContext(ctx)
	if err != nil {
		httperror.Unauthorized("", err, nil).Render(rw)
		return
	}

	isPending, err := h.EmbeddedWalletService.IsVerificationPending(ctx, contractAddress)
	if err != nil {
		if errors.Is(err, services.ErrInvalidContractAddress) {
			httperror.Unauthorized("", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to evaluate verification requirement", err, nil).Render(rw)
		}
		return
	}

	asset, err := h.EmbeddedWalletService.GetPendingDisbursementAsset(ctx, contractAddress)
	if err != nil {
		httperror.InternalError(ctx, "Failed to retrieve pending disbursement asset", err, nil).Render(rw)
		return
	}

	resp := EmbeddedWalletProfileResponse{IsVerificationPending: isPending, PendingAsset: asset}
	httpjson.Render(rw, resp, httpjson.JSON)
}
