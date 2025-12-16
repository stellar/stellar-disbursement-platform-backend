package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	Verification EmbeddedWalletVerificationDetails `json:"verification"`
	Wallet       *EmbeddedWalletDetails            `json:"wallet,omitempty"`
}

type EmbeddedWalletVerificationDetails struct {
	IsPending    bool        `json:"is_pending"`
	PendingAsset *data.Asset `json:"pending_asset,omitempty"`
}

type EmbeddedWalletDetails struct {
	SupportedAssets []SupportedAsset               `json:"supported_assets"`
	ReceiverContact *EmbeddedWalletReceiverContact `json:"receiver_contact"`
}

type EmbeddedWalletReceiverContact struct {
	Type  data.ReceiverContactType `json:"type"`
	Value string                   `json:"value"`
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

	contractAddress = strings.TrimSpace(contractAddress)
	if contractAddress == "" {
		httperror.Unauthorized("", services.ErrMissingContractAddress, nil).Render(rw)
		return
	}

	isPending, err := h.EmbeddedWalletService.IsVerificationPending(ctx, contractAddress)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrMissingContractAddress):
			httperror.Unauthorized("", err, nil).Render(rw)
		case errors.Is(err, data.ErrRecordNotFound):
			httperror.NotFound("receiver wallet not found", err, nil).Render(rw)
		default:
			httperror.InternalError(ctx, "Failed to evaluate verification requirement", err, nil).Render(rw)
		}
		return
	}

	var pendingAsset *data.Asset
	if isPending {
		pendingAsset, err = h.EmbeddedWalletService.GetPendingDisbursementAsset(ctx, contractAddress)
		if err != nil {
			switch {
			case errors.Is(err, services.ErrMissingContractAddress):
				httperror.Unauthorized("", err, nil).Render(rw)
			default:
				httperror.InternalError(ctx, "Failed to retrieve pending disbursement asset", err, nil).Render(rw)
			}
			return
		}
	}

	resp := EmbeddedWalletProfileResponse{
		Verification: EmbeddedWalletVerificationDetails{
			IsPending:    isPending,
			PendingAsset: pendingAsset,
		},
	}

	if !isPending {
		supportedAssets, err := h.getSupportedAssets(ctx)
		if err != nil {
			httperror.InternalError(ctx, "Failed to retrieve supported assets", err, nil).Render(rw)
			return
		}

		receiverContact, err := h.getReceiverContact(ctx, contractAddress)
		if err != nil {
			switch {
			case errors.Is(err, services.ErrMissingContractAddress):
				httperror.Unauthorized("", err, nil).Render(rw)
			case errors.Is(err, data.ErrRecordNotFound):
				httperror.NotFound("receiver not found", err, nil).Render(rw)
			default:
				httperror.InternalError(ctx, "Failed to retrieve receiver contact info", err, nil).Render(rw)
			}
			return
		}

		resp.Wallet = &EmbeddedWalletDetails{
			SupportedAssets: supportedAssets,
			ReceiverContact: receiverContact,
		}
	}

	httpjson.Render(rw, resp, httpjson.JSON)
}

func (h EmbeddedWalletProfileHandler) getSupportedAssets(ctx context.Context) ([]SupportedAsset, error) {
	wallets, err := h.Models.Wallets.FindWallets(ctx, data.Filter{Key: data.FilterEmbedded, Value: true})
	if err != nil {
		return nil, fmt.Errorf("finding wallets: %w", err)
	}

	if len(wallets) != 1 {
		return nil, fmt.Errorf("expected exactly one embedded wallet, found %d", len(wallets))
	}

	assets, err := h.Models.Wallets.GetAssets(ctx, wallets[0].ID)
	if err != nil {
		return nil, fmt.Errorf("getting wallet supported assets: %w", err)
	}

	supportedAssets := make([]SupportedAsset, 0, len(assets))
	for _, a := range assets {
		supportedAssets = append(supportedAssets, SupportedAsset{
			Code:   a.Code,
			Issuer: a.Issuer,
		})
	}

	return supportedAssets, nil
}

func (h EmbeddedWalletProfileHandler) getReceiverContact(ctx context.Context, contractAddress string) (*EmbeddedWalletReceiverContact, error) {
	receiver, err := h.EmbeddedWalletService.GetReceiverContact(ctx, contractAddress)
	if err != nil {
		return nil, fmt.Errorf("getting receiver contact: %w", err)
	}

	switch {
	case receiver.Email != "":
		return &EmbeddedWalletReceiverContact{Type: data.ReceiverContactTypeEmail, Value: receiver.Email}, nil
	case receiver.PhoneNumber != "":
		return &EmbeddedWalletReceiverContact{Type: data.ReceiverContactTypeSMS, Value: receiver.PhoneNumber}, nil
	default:
		return nil, fmt.Errorf("receiver contact type not supported")
	}
}
