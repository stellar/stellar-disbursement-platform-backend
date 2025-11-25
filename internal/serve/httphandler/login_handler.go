package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

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

func (h LoginHandler) validateRequest(ctx context.Context, req LoginRequest, headers http.Header) *httperror.HTTPError {
	lv := validators.NewValidator()

	lv.Check(req.Email != "", "email", "email is required")
	lv.Check(req.Password != "", "password", "password is required")

	captchaDisabled := IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	})
	lv.Check(captchaDisabled || req.ReCAPTCHAToken != "", "recaptcha_token", "reCAPTCHA token is required")

	mfaDisabled := IsMFADisabled(ctx, MFAConfig{
		Models:      h.Models,
		MFADisabled: h.MFADisabled,
	})
	deviceID := headers.Get(DeviceIDHeader)
	lv.Check(mfaDisabled || deviceID != "", DeviceIDHeader, "Device-ID header is required")

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
	if httpErr := h.validateRequest(ctx, reqBody, req.Header); httpErr != nil {
		httpErr.Render(rw)
		return
	}

	truncatedEmail := utils.TruncateString(reqBody.Email, 3)

	// Step 2: Run the reCAPTCHA validation if it is enabled
	if !IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	}) {
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

	// 4: Handle MFA logic as needed
	canSkipMFA, httpErr := h.handleMFA(ctx, req, user)
	switch {
	case httpErr != nil: // If an error occurred, render it
		httpErr.Render(rw)
	case canSkipMFA: // MFA can be skipped, log the user in
		log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", user.ID)
		httpjson.RenderStatus(rw, http.StatusOK, LoginResponse{Token: token}, httpjson.JSON)
	default: // MFA is required, send response about MFA code
		httpjson.RenderStatus(rw,
			http.StatusOK,
			map[string]string{"message": "MFA code sent to email. Check your inbox and spam folders."},
			httpjson.JSON)
	}
}

// handleMFA handles the MFA logic for the login flow.
func (h LoginHandler) handleMFA(ctx context.Context, req *http.Request, user *auth.User) (canSkipMFA bool, httpErr *httperror.HTTPError) {
	truncatedEmail := utils.TruncateString(user.Email, 3)
	// 1: If MFA is disabled, return the token
	if IsMFADisabled(ctx, MFAConfig{
		Models:      h.Models,
		MFADisabled: h.MFADisabled,
	}) {
		log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", user.ID)
		return true, nil
	}

	// 2: If MFA is enabled, check if the device is remembered
	deviceID := req.Header.Get(DeviceIDHeader)
	if isRemembered, err := h.AuthManager.MFADeviceRemembered(ctx, deviceID, user.ID); err != nil {
		err = fmt.Errorf("checking if device is remembered for user with email %s: %w", truncatedEmail, err)
		return false, httperror.InternalError(ctx, "Cannot check if MFA code is remembered", err, nil)
	} else if isRemembered {
		log.Ctx(ctx).Infof("[UserLogin] - Logged in user with account ID %s", user.ID)
		return true, nil
	}

	// 3: If MFA is enabled and the device is not remembered, send the MFA code
	code, err := h.AuthManager.GetMFACode(ctx, deviceID, user.ID)
	if err != nil {
		err = fmt.Errorf("getting MFA code for user with email %s: %w", truncatedEmail, err)
		return false, httperror.InternalError(ctx, "Cannot get MFA code", err, nil)
	}
	if err = h.sendMFAEmail(ctx, user, code); err != nil {
		return false, httperror.InternalError(ctx, "Failed to send send MFA code", err, nil)
	}

	return false, nil
}

func (h LoginHandler) sendMFAEmail(ctx context.Context, user *auth.User, code string) error {
	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("fetching organization: %w", err)
	}

	msgTemplate := htmltemplate.StaffMFAEmailMessageTemplate{
		MFACode:          code,
		OrganizationName: organization.Name,
	}
	msgContent, err := htmltemplate.ExecuteHTMLTemplateForStaffMFAEmailMessage(msgTemplate)
	if err != nil {
		return fmt.Errorf("executing MFA message template: %w", err)
	}

	msg := message.Message{
		ToEmail: user.Email,
		Title:   mfaMessageTitle,
		Body:    msgContent,
		Type:    message.MessageTypeUserMFA,
		TemplateVariables: map[message.TemplateVariable]string{
			message.TemplateVarMFACode: code,
			message.TemplateVarOrgName: organization.Name,
		},
	}
	if err = h.MessengerClient.SendMessage(ctx, msg); err != nil {
		return fmt.Errorf("sending MFA code to email %s: %w", user.Email, err)
	}

	return nil
}
