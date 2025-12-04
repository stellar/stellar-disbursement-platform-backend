package httphandler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/wallet"
)

const (
	// WalletTokenExpiration defines how long wallet JWT tokens are valid
	WalletTokenExpiration = 15 * time.Minute
)

type PasskeyHandler struct {
	WebAuthnService       wallet.WebAuthnServiceInterface
	WalletJWTManager      wallet.WalletJWTManager
	EmbeddedWalletService services.EmbeddedWalletServiceInterface
}

type StartPasskeyRegistrationRequest struct {
	Token string `json:"token"`
}

func (r StartPasskeyRegistrationRequest) Validate() *httperror.HTTPError {
	validator := validators.NewValidator()
	validator.Check(len(strings.TrimSpace(r.Token)) > 0, "token", "token is required")

	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}

	return nil
}

type PasskeyRegistrationResponse struct {
	Token        string `json:"token"`
	CredentialID string `json:"credential_id"`
	PublicKey    string `json:"public_key"`
}

type PasskeyAuthenticationResponse struct {
	Token           string `json:"token"`
	CredentialID    string `json:"credential_id"`
	ContractAddress string `json:"contract_address"`
}

func (h PasskeyHandler) StartPasskeyRegistration(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody StartPasskeyRegistrationRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.Validate(); err != nil {
		err.Render(rw)
		return
	}

	credentialCreation, err := h.WebAuthnService.StartPasskeyRegistration(ctx, reqBody.Token)
	if err != nil {
		if errors.Is(err, wallet.ErrInvalidToken) {
			httperror.BadRequest("Invalid token", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrWalletAlreadyExists) {
			httperror.Conflict("Wallet already exists for this token", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to start passkey registration", err, nil).Render(rw)
		}
		return
	}

	httpjson.Render(rw, credentialCreation, httpjson.JSON)
}

func (h PasskeyHandler) FinishPasskeyRegistration(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token := req.URL.Query().Get("token")
	if token == "" {
		httperror.BadRequest("Token query parameter is required", nil, nil).Render(rw)
		return
	}

	credential, err := h.WebAuthnService.FinishPasskeyRegistration(ctx, token, req)
	if err != nil {
		if errors.Is(err, wallet.ErrInvalidToken) {
			httperror.BadRequest("Invalid token", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrWalletAlreadyExists) {
			httperror.Conflict("Wallet already exists", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrSessionNotFound) {
			httperror.BadRequest("Session not found or expired", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrSessionTypeMismatch) {
			httperror.BadRequest("Invalid session type", err, nil).Render(rw)
		} else if errors.Is(err, protocol.ErrChallengeMismatch) || errors.Is(err, protocol.ErrVerification) {
			httperror.BadRequest("Registration verification failed", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to finish passkey registration", err, nil).Render(rw)
		}
		return
	}

	publicKeyHex, err := wallet.COSEKeyToUncompressedHex(credential.PublicKey)
	if err != nil {
		httperror.InternalError(ctx, "Failed to extract public key from credential", err, nil).Render(rw)
		return
	}

	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	expiresAt := time.Now().Add(WalletTokenExpiration)
	jwtToken, err := h.WalletJWTManager.GenerateToken(ctx, credentialID, "", expiresAt)
	if err != nil {
		httperror.InternalError(ctx, "Failed to generate token", err, nil).Render(rw)
		return
	}

	resp := PasskeyRegistrationResponse{
		Token:        jwtToken,
		CredentialID: credentialID,
		PublicKey:    publicKeyHex,
	}

	httpjson.RenderStatus(rw, http.StatusCreated, resp, httpjson.JSON)
}

func (h PasskeyHandler) StartPasskeyAuthentication(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	credentialAssertion, err := h.WebAuthnService.StartPasskeyAuthentication(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Failed to start passkey authentication", err, nil).Render(rw)
		return
	}

	httpjson.Render(rw, credentialAssertion, httpjson.JSON)
}

func (h PasskeyHandler) FinishPasskeyAuthentication(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	embeddedWallet, err := h.WebAuthnService.FinishPasskeyAuthentication(ctx, req)
	if err != nil {
		if errors.Is(err, wallet.ErrWalletNotReady) {
			httperror.BadRequest("Wallet not ready for authentication", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrSessionNotFound) {
			httperror.BadRequest("Session not found or expired", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrSessionTypeMismatch) {
			httperror.BadRequest("Invalid session type", err, nil).Render(rw)
		} else if errors.Is(err, protocol.ErrChallengeMismatch) || errors.Is(err, protocol.ErrVerification) {
			httperror.Unauthorized("Authentication verification failed", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to finish passkey authentication", err, nil).Render(rw)
		}
		return
	}

	expiresAt := time.Now().Add(WalletTokenExpiration)
	token, err := h.WalletJWTManager.GenerateToken(ctx, embeddedWallet.CredentialID, embeddedWallet.ContractAddress, expiresAt)
	if err != nil {
		httperror.InternalError(ctx, "Failed to generate authentication token", err, nil).Render(rw)
		return
	}

	resp := PasskeyAuthenticationResponse{
		Token:           token,
		CredentialID:    embeddedWallet.CredentialID,
		ContractAddress: embeddedWallet.ContractAddress,
	}

	httpjson.Render(rw, resp, httpjson.JSON)
}

type RefreshTokenRequest struct {
	Token string `json:"token"`
}

func (r RefreshTokenRequest) Validate() *httperror.HTTPError {
	validator := validators.NewValidator()
	validator.Check(len(strings.TrimSpace(r.Token)) > 0, "token", "token is required")

	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}

	return nil
}

type RefreshTokenResponse struct {
	Token                 string      `json:"token"`
	IsVerificationPending bool        `json:"is_verification_pending"`
	PendingAsset          *data.Asset `json:"pending_asset,omitempty"`
}

func (h PasskeyHandler) RefreshToken(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody RefreshTokenRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.Validate(); err != nil {
		err.Render(rw)
		return
	}

	credentialID, contractAddress, err := h.WalletJWTManager.ValidateToken(ctx, reqBody.Token)
	if err != nil {
		if errors.Is(err, wallet.ErrExpiredWalletToken) {
			httperror.Unauthorized("Token has expired", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrInvalidWalletToken) {
			httperror.Unauthorized("Invalid token", err, nil).Render(rw)
		} else if errors.Is(err, wallet.ErrMissingSubClaim) {
			httperror.Unauthorized("Invalid token claims", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to validate token", err, nil).Render(rw)
		}
		return
	}

	embeddedWallet, err := h.EmbeddedWalletService.GetWalletByCredentialID(ctx, credentialID)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentialID) {
			httperror.Unauthorized("Invalid credential ID", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to lookup wallet", err, nil).Render(rw)
		}
		return
	}
	if embeddedWallet.ContractAddress != "" {
		contractAddress = embeddedWallet.ContractAddress
	}
	isVerificationPending, err := h.IsVerificationPending(ctx, embeddedWallet)
	if err != nil {
		if errors.Is(err, services.ErrInvalidReceiverWalletID) {
			httperror.InternalError(ctx, "Receiver wallet not found", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to evaluate verification requirement", err, nil).Render(rw)
		}
		return
	}

	var asset *data.Asset
	if contractAddress != "" {
		asset, err = h.EmbeddedWalletService.GetPendingDisbursementAsset(ctx, contractAddress)
		if err != nil {
			httperror.InternalError(ctx, "Failed to retrieve pending disbursement asset", err, nil).Render(rw)
			return
		}
	}

	expiresAt := time.Now().Add(WalletTokenExpiration)
	refreshedToken, err := h.WalletJWTManager.GenerateToken(ctx, credentialID, contractAddress, expiresAt)
	if err != nil {
		httperror.InternalError(ctx, "Failed to generate token", err, nil).Render(rw)
		return
	}

	resp := RefreshTokenResponse{
		Token:                 refreshedToken,
		IsVerificationPending: isVerificationPending,
		PendingAsset:          asset,
	}

	httpjson.Render(rw, resp, httpjson.JSON)
}

func (h PasskeyHandler) IsVerificationPending(ctx context.Context, embeddedWallet *data.EmbeddedWallet) (bool, error) {
	if embeddedWallet == nil || !embeddedWallet.RequiresVerification {
		return false, nil
	}

	if embeddedWallet.ReceiverWalletID == "" {
		return false, nil
	}

	receiverWallet, err := h.EmbeddedWalletService.GetReceiverWalletByID(ctx, embeddedWallet.ReceiverWalletID)
	if err != nil {
		return false, fmt.Errorf("getting receiver wallet by ID: %w", err)
	}

	return receiverWallet.Status == data.ReadyReceiversWalletStatus, nil
}
