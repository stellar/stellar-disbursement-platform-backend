package httphandler

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/wallet"
)

type PasskeyHandler struct {
	WebAuthnService  wallet.WebAuthnServiceInterface
	WalletJWTManager wallet.WalletJWTManager
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

	resp := PasskeyRegistrationResponse{
		CredentialID: base64.RawURLEncoding.EncodeToString(credential.ID),
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

	expiresAt := time.Now().Add(15 * time.Minute)
	token, err := h.WalletJWTManager.GenerateToken(ctx, embeddedWallet.ContractAddress, expiresAt)
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
