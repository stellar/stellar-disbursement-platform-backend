package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const mfaMessageTitle = "Verification code to access your account"

type LoginRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	ReCAPTCHAToken string `json:"recaptcha_token"`
}

func (r LoginRequest) validate() *httperror.HTTPError {
	validator := validators.NewValidator()

	validator.Check(r.Email != "", "email", "email is required")
	validator.Check(r.Password != "", "password", "password is required")

	if validator.HasErrors() {
		return httperror.BadRequest("Request invalid", nil, validator.Errors)
	}

	return nil
}

type LoginResponse struct {
	Token string `json:"token"`
}

type LoginHandler struct {
	AuthManager        auth.AuthManager
	ReCAPTCHAValidator validators.ReCAPTCHAValidator
	MessengerClient    message.MessengerClient
	Models             *data.Models
	ReCAPTCHADisabled  bool
	MFADisabled        bool
}

func (h LoginHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody LoginRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	if err := reqBody.validate(); err != nil {
		err.Render(rw)
		return
	}

	if !h.ReCAPTCHADisabled {
		// validating reCAPTCHA Token
		isValid, err := h.ReCAPTCHAValidator.IsTokenValid(ctx, reqBody.ReCAPTCHAToken)
		if err != nil {
			httperror.Unauthorized("Cannot validate reCAPTCHA token", err, nil).Render(rw)
			return
		}

		if !isValid {
			log.Ctx(ctx).Errorf("reCAPTCHA token is invalid for request with email %s", utils.TruncateString(reqBody.Email, 3))
			httperror.BadRequest("reCAPTCHA token invalid", nil, nil).Render(rw)
			return
		}
	}

	token, err := h.AuthManager.Authenticate(ctx, reqBody.Email, reqBody.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		httperror.Unauthorized("", err, map[string]interface{}{"details": "Incorrect email or password"}).Render(rw)
		return
	}
	if err != nil {
		log.Ctx(ctx).Errorf("error authenticating user with email %s: %s", utils.TruncateString(reqBody.Email, 3), err)
		httperror.InternalError(ctx, "Cannot authenticate user credentials", err, nil).Render(rw)
		return
	}

	user, err := h.AuthManager.GetUser(ctx, token)
	if err != nil {
		log.Ctx(ctx).Errorf("error getting user with email %s: %s", utils.TruncateString(reqBody.Email, 3), err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	if h.MFADisabled {
		log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", user.ID)
		httpjson.RenderStatus(rw, http.StatusOK, LoginResponse{Token: token}, httpjson.JSON)
		return
	}

	// 🔒 Handling MFA
	deviceID := req.Header.Get(DeviceIDHeader)
	if deviceID == "" {
		httperror.BadRequest("Device-ID header is required", nil, nil).Render(rw)
		return
	}

	isRemembered, err := h.AuthManager.MFADeviceRemembered(ctx, deviceID, user.ID)
	if err != nil {
		log.Ctx(ctx).Errorf("error checking if device is remembered for user with email %s: %s", utils.TruncateString(reqBody.Email, 3), err.Error())
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	if isRemembered {
		log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", user.ID)
		httpjson.RenderStatus(rw, http.StatusOK, LoginResponse{Token: token}, httpjson.JSON)
		return
	}

	// Get the MFA code for the user
	code, err := h.AuthManager.GetMFACode(ctx, deviceID, user.ID)
	if err != nil {
		log.Ctx(ctx).Errorf("error getting MFA code for user with email %s: %s", utils.TruncateString(reqBody.Email, 3), err.Error())
		httperror.InternalError(ctx, "Cannot get MFA code", err, nil).Render(rw)
		return
	}

	if code == "" {
		log.Ctx(ctx).Errorf("MFA code for user with email %s is empty", utils.TruncateString(reqBody.Email, 3))
		httperror.InternalError(ctx, "Cannot get MFA code", err, nil).Render(rw)
		return
	}

	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot fetch organization", err, nil).Render(rw)
		return
	}

	msgTemplate := htmltemplate.MFAMessageTemplate{
		MFACode:          code,
		OrganizationName: organization.Name,
	}
	msgContent, err := htmltemplate.ExecuteHTMLTemplateForMFAMessage(msgTemplate)
	if err != nil {
		httperror.InternalError(ctx, "Cannot execute mfa message template", err, nil).Render(rw)
		return
	}

	msg := message.Message{
		ToEmail: user.Email,
		Title:   mfaMessageTitle,
		Message: msgContent,
	}
	err = h.MessengerClient.SendMessage(msg)
	if err != nil {
		err = fmt.Errorf("error sending mfa code for email %s: %w", user.Email, err)
		log.Ctx(ctx).Error(err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "MFA code sent to email. Check your inbox and spam folders."}, httpjson.JSON)
}
