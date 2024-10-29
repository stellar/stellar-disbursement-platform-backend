package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const mfaMessageTitle = "Verification code to access your account"

type LoginRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	ReCAPTCHAToken string `json:"recaptcha_token"`
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

func (h LoginHandler) validateRequest(req LoginRequest, headers http.Header) *httperror.HTTPError {
	lv := validators.NewValidator()

	lv.Check(req.Email != "", "email", "email is required")
	lv.Check(req.Password != "", "password", "password is required")
	lv.Check(h.ReCAPTCHADisabled || req.ReCAPTCHAToken != "", "recaptcha_token", "reCAPTCHA token is required")

	deviceID := headers.Get(DeviceIDHeader)
	lv.Check(h.MFADisabled || deviceID != "", DeviceIDHeader, "Device-ID header is required")

	if lv.HasErrors() {
		return httperror.BadRequest("", nil, lv.Errors)
	}

	return nil
}

func (h LoginHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Step 1: Decode and validate the incoming request
	var reqBody LoginRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		err = fmt.Errorf("decoding the request body: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}
	if httpErr := h.validateRequest(reqBody, req.Header); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	truncatedEmail := utils.TruncateString(reqBody.Email, 3)

	// Step 2: Run the reCAPTCHA validation if it is enabled
	if !h.ReCAPTCHADisabled {
		// validating reCAPTCHA Token
		isValid, err := h.ReCAPTCHAValidator.IsTokenValid(ctx, reqBody.ReCAPTCHAToken)
		if err != nil {
			httperror.Unauthorized("Cannot validate reCAPTCHA token", err, nil).Render(rw)
			return
		}

		if !isValid {
			log.Ctx(ctx).Errorf("reCAPTCHA token is invalid for request with email %s", truncatedEmail)
			httperror.BadRequest("reCAPTCHA token invalid", nil, nil).Render(rw)
			return
		}
	}

	// Step 3: Authenticate the user
	token, err := h.AuthManager.Authenticate(ctx, reqBody.Email, reqBody.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		httperror.Unauthorized("", err, map[string]interface{}{"details": "Incorrect email or password"}).Render(rw)
		return
	}
	if err != nil {
		log.Ctx(ctx).Errorf("authenticating user with email %s: %s", truncatedEmail, err)
		httperror.InternalError(ctx, "Cannot authenticate user credentials", err, nil).Render(rw)
		return
	}

	user, err := h.AuthManager.GetUser(ctx, token)
	if err != nil {
		log.Ctx(ctx).Errorf("getting user with email %s: %s", truncatedEmail, err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	// Step 4.A: If MFA is disabled, return the token
	if h.MFADisabled {
		log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", user.ID)
		httpjson.RenderStatus(rw, http.StatusOK, LoginResponse{Token: token}, httpjson.JSON)
		return
	}

	// Step 4.B: If MFA is enabled, check if the device is remembered
	deviceID := req.Header.Get(DeviceIDHeader)
	var isRemembered bool
	if isRemembered, err = h.AuthManager.MFADeviceRemembered(ctx, deviceID, user.ID); err != nil {
		err = fmt.Errorf("checking if device is remembered for user with email %s: %w", truncatedEmail, err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	} else if isRemembered {
		log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", user.ID)
		httpjson.RenderStatus(rw, http.StatusOK, LoginResponse{Token: token}, httpjson.JSON)
		return
	}

	// Step 4.C: If MFA is enabled and the device is not remembered, send the MFA code
	code, err := h.AuthManager.GetMFACode(ctx, deviceID, user.ID)
	if err != nil {
		log.Ctx(ctx).Errorf("getting MFA code for user with email %s: %s", truncatedEmail, err.Error())
		httperror.InternalError(ctx, "Cannot get MFA code", err, nil).Render(rw)
		return
	}
	if httpErr := h.sendMFAEmail(ctx, user, code); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"message": "MFA code sent to email. Check your inbox and spam folders."}, httpjson.JSON)
}

func (h LoginHandler) sendMFAEmail(ctx context.Context, user *auth.User, code string) *httperror.HTTPError {
	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		return httperror.InternalError(ctx, "Cannot fetch organization", err, nil)
	}

	msgTemplate := htmltemplate.StaffMFAEmailMessageTemplate{
		MFACode:          code,
		OrganizationName: organization.Name,
	}
	msgContent, err := htmltemplate.ExecuteHTMLTemplateForStaffMFAEmailMessage(msgTemplate)
	if err != nil {
		return httperror.InternalError(ctx, "Cannot execute mfa message template", err, nil)
	}

	msg := message.Message{
		ToEmail: user.Email,
		Title:   mfaMessageTitle,
		Body:    msgContent,
	}
	if err = h.MessengerClient.SendMessage(msg); err != nil {
		err = fmt.Errorf("sending mfa code for email %s: %w", user.Email, err)
		return httperror.InternalError(ctx, "", err, nil)
	}

	return nil
}
