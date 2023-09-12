package httphandler

import (
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
	ReCAPTCHAEnabled   bool
}

const DeviceIDHeader = "Device-ID"

func (h MFAHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody MFARequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		log.Ctx(ctx).Errorf("decoding the request body: %s", err.Error())
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	// validating reCAPTCHA Token
	if h.ReCAPTCHAEnabled {
		isValid, recaptchaErr := h.ReCAPTCHAValidator.IsTokenValid(ctx, reqBody.ReCAPTCHAToken)
		if recaptchaErr != nil {
			httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", recaptchaErr, nil).Render(rw)
			return
		}

		if !isValid {
			log.Ctx(ctx).Errorf("reCAPTCHA token is invalid for request with email")
			httperror.BadRequest("reCAPTCHA token invalid", nil, nil).Render(rw)
			return
		}
	}

	if reqBody.MFACode == "" {
		extras := map[string]interface{}{"mfa_code": "MFA Code is required"}
		httperror.BadRequest("Request invalid", nil, extras).Render(rw)
		return
	}

	deviceID := req.Header.Get(DeviceIDHeader)
	if deviceID == "" {
		httperror.BadRequest("Device-ID header is required", nil, nil).Render(rw)
		return
	}

	token, err := h.AuthManager.AuthenticateMFA(ctx, deviceID, reqBody.MFACode, reqBody.RememberMe)
	if err != nil {
		if errors.Is(err, auth.ErrMFACodeInvalid) {
			httperror.Unauthorized("", err, nil).Render(rw)
			return
		}
		log.Ctx(ctx).Errorf("error authenticating user: %s", err.Error())
		httperror.InternalError(ctx, "Cannot authenticate user", err, nil).Render(rw)
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
