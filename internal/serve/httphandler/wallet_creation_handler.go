package httphandler

import (
	"crypto/ecdh"
	"encoding/hex"
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
)

type WalletCreationHandler struct {
	EmbeddedWalletService services.EmbeddedWalletServiceInterface
}

type CreateWalletRequest struct {
	Token        string `json:"token"`
	PublicKey    string `json:"public_key"`
	CredentialID string `json:"credential_id"`
}

func (r CreateWalletRequest) Validate() *httperror.HTTPError {
	validator := validators.NewValidator()
	validator.Check(len(strings.TrimSpace(r.Token)) > 0, "token", "token should not be empty")
	validator.Check(len(strings.TrimSpace(r.PublicKey)) > 0, "public_key", "public_key should not be empty")
	validator.Check(len(strings.TrimSpace(r.CredentialID)) > 0, "credential_id", "credential_id should not be empty")
	validator.Check(len(r.CredentialID) <= data.MaxCredentialIDLength, "credential_id", fmt.Sprintf("credential_id must be %d characters or less", data.MaxCredentialIDLength))
	if pk, err := hex.DecodeString(r.PublicKey); err != nil {
		validator.AddError("public_key", "public_key is not a valid hex string")
	} else if !isValidP256PublicKey(pk) {
		validator.AddError("public_key", "public_key is not a valid uncompressed P256 public key")
	}

	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}

	return nil
}

func isValidP256PublicKey(pubKeyBytes []byte) bool {
	if len(pubKeyBytes) != 65 || pubKeyBytes[0] != 0x04 {
		return false
	}
	_, err := ecdh.P256().NewPublicKey(pubKeyBytes)
	return err == nil
}

type WalletResponse struct {
	ContractAddress string                    `json:"contract_address,omitempty"`
	Status          data.EmbeddedWalletStatus `json:"status"`
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

	err := h.EmbeddedWalletService.CreateWallet(ctx, reqBody.Token, reqBody.PublicKey, reqBody.CredentialID)
	if err != nil {
		if errors.Is(err, services.ErrInvalidToken) {
			httperror.BadRequest("Invalid token", err, nil).Render(rw)
		} else if errors.Is(err, services.ErrCreateWalletInvalidStatus) {
			httperror.BadRequest("Wallet status is not pending for token", err, nil).Render(rw)
		} else if errors.Is(err, services.ErrCredentialIDAlreadyExists) {
			httperror.Conflict("Credential ID already exists", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to create wallet", err, nil).Render(rw)
		}
		return
	}

	resp := WalletResponse{
		Status: data.PendingWalletStatus,
	}

	httpjson.RenderStatus(rw, http.StatusAccepted, resp, httpjson.JSON)
}

func (h WalletCreationHandler) GetWallet(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	credentialID := strings.TrimSpace(chi.URLParam(req, "credentialID"))
	if len(credentialID) == 0 {
		httperror.BadRequest("Credential ID is required", nil, nil).Render(rw)
		return
	}

	wallet, err := h.EmbeddedWalletService.GetWalletByCredentialID(ctx, credentialID)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentialID) {
			httperror.NotFound("Wallet not found", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to get wallet", err, nil).Render(rw)
		}
		return
	}

	resp := WalletResponse{
		ContractAddress: wallet.ContractAddress,
		Status:          wallet.WalletStatus,
	}
	httpjson.Render(rw, resp, httpjson.JSON)
}
