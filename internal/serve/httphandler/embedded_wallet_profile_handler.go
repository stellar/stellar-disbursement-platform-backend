package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type EmbeddedWalletProfileHandler struct {
	EmbeddedWalletService services.EmbeddedWalletServiceInterface
	Models                *data.Models
}

type EmbeddedWalletProfileResponse struct {
	IsVerificationPending bool        `json:"is_verification_pending"`
	PendingAsset          *data.Asset `json:"pending_asset,omitempty"`
}

type EmbeddedWalletAssetsResponse struct {
	Assets []SupportedAsset `json:"assets"`
}

type SupportedAsset struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
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

func (h EmbeddedWalletProfileHandler) GetAssets(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	wallets, err := h.Models.Wallets.FindWallets(ctx, data.Filter{Key: data.FilterEmbedded, Value: true})
	if err != nil {
		httperror.InternalError(ctx, "Failed to retrieve supported assets", err, nil).Render(rw)
		return
	}

	if len(wallets) != 1 {
		httperror.InternalError(ctx, "Failed to retrieve supported assets", fmt.Errorf("expected exactly one embedded wallet, found %d", len(wallets)), nil).Render(rw)
		return
	}

	assets, err := h.Models.Wallets.GetAssets(ctx, wallets[0].ID)
	if err != nil {
		httperror.InternalError(ctx, "Failed to retrieve supported assets", err, nil).Render(rw)
		return
	}

	supportedAssets := make([]SupportedAsset, 0, len(assets))
	for _, a := range assets {
		supportedAssets = append(supportedAssets, SupportedAsset{
			Code:   a.Code,
			Issuer: a.Issuer,
		})
	}

	resp := EmbeddedWalletAssetsResponse{
		Assets: supportedAssets,
	}
	httpjson.Render(rw, resp, httpjson.JSON)
}
