package httphandler

import (
	"context"
	"errors"
	"net/http"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type MFARequest struct {
	MFACode        string `json:"mfa_code"`
	RememberMe     bool   `json:"remember_me"`
	ReCAPTCHAToken string `json:"recaptcha_token"`
}

type MFAResponse struct {
	Token string `json:"token"`
}

type MFAHandler struct {
	AuthManager        auth.AuthManager
	ReCAPTCHAValidator validators.ReCAPTCHAValidator
	Models             *data.Models
	ReCAPTCHADisabled  bool
}

const DeviceIDHeader = "Device-ID"

func (h MFAHandler) validateRequest(ctx context.Context, req MFARequest, deviceID string) *httperror.HTTPError {
	lv := validators.NewValidator()

	lv.Check(req.MFACode != "", "mfa_code", "MFA Code is required")

	captchaDisabled := IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	})
	lv.Check(captchaDisabled || req.ReCAPTCHAToken != "", "recaptcha_token", "reCAPTCHA token is required")

	lv.Check(deviceID != "", DeviceIDHeader, DeviceIDHeader+" header is required")

	if lv.HasErrors() {
		return httperror.BadRequest("", nil, lv.Errors)
	}

	return nil
}

func (h MFAHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Step 1: Decode and validate the incoming request
	var reqBody MFARequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		log.Ctx(ctx).Errorf("decoding the request body: %s", err.Error())
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}
	deviceID := req.Header.Get(DeviceIDHeader)
	if httpErr := h.validateRequest(ctx, reqBody, deviceID); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	// Step 2: Run the reCAPTCHA validation if it is enabled
	if !IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	}) {
		isValid, err := h.ReCAPTCHAValidator.IsTokenValid(ctx, reqBody.ReCAPTCHAToken)
		if err != nil {
			httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", err, nil).Render(rw)
			return
		}

		if !isValid {
			log.Ctx(ctx).Errorf("reCAPTCHA token is invalid for request with device ID %s", deviceID)
			httperror.BadRequest("reCAPTCHA token invalid", nil, nil).Render(rw)
			return
		}
	}

	// Step 3: Authenticate the user with the MFA code
	token, err := h.AuthManager.AuthenticateMFA(ctx, deviceID, reqBody.MFACode, reqBody.RememberMe)
	if err != nil {
		if errors.Is(err, auth.ErrMFACodeInvalid) {
			httperror.Unauthorized("", err, nil).Render(rw)
		} else {
			log.Ctx(ctx).Errorf("authenticating user: %s", err.Error())
			httperror.InternalError(ctx, "Cannot authenticate user", err, nil).Render(rw)
		}
		return
	}

	userID, err := h.AuthManager.GetUserID(ctx, token)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get user ID", err, nil).Render(rw)
		return
	}
	log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", userID)
	httpjson.RenderStatus(rw, http.StatusOK, MFAResponse{Token: token}, httpjson.JSON)
}
