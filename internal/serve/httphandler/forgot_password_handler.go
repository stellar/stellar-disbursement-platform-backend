package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const forgotPasswordMessageTitle = "Reset Account Password"

// ForgotPasswordHandler searches for the user that is requesting a password reset
// and sends an email with a link to access the password reset page.
type ForgotPasswordHandler struct {
	AuthManager        auth.AuthManager
	MessengerClient    message.MessengerClient
	Models             *data.Models
	ReCAPTCHAValidator validators.ReCAPTCHAValidator
	ReCAPTCHADisabled  bool
}

type ForgotPasswordRequest struct {
	Email          string `json:"email"`
	ReCAPTCHAToken string `json:"recaptcha_token"`
}

func (h ForgotPasswordHandler) validateRequest(ctx context.Context, req ForgotPasswordRequest) *httperror.HTTPError {
	v := validators.NewValidator()
	v.Check(req.Email != "", "email", "email is required")

	captchaDisabled := IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	})
	v.Check(captchaDisabled || req.ReCAPTCHAToken != "", "recaptcha_token", "reCAPTCHA token is required")

	if v.HasErrors() {
		return httperror.BadRequest("", nil, v.Errors)
	}

	return nil
}

// ServeHTTP implements the http.Handler interface.
func (h ForgotPasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Step 1: Get tenant from context
	tnt, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		err = fmt.Errorf("getting tenant from context: %w", err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	// Step 2: Decode and validate the incoming request
	var forgotPasswordRequest ForgotPasswordRequest
	err = json.NewDecoder(r.Body).Decode(&forgotPasswordRequest)
	if err != nil {
		httperror.BadRequest("", err, nil).Render(w)
		return
	}
	if httpErr := h.validateRequest(ctx, forgotPasswordRequest); httpErr != nil {
		httpErr.Render(w)
		return
	}

	truncatedEmail := utils.TruncateString(forgotPasswordRequest.Email, 3)

	// Step 3: Run the reCAPTCHA validation if it is enabled
	if !IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	}) {
		// validating reCAPTCHA Token
		isValid, recaptchaErr := h.ReCAPTCHAValidator.IsTokenValid(ctx, forgotPasswordRequest.ReCAPTCHAToken)
		if recaptchaErr != nil {
			httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", recaptchaErr, nil).Render(w)
			return
		}

		if !isValid {
			log.Ctx(ctx).Errorf("reCAPTCHA token is invalid for request with email %s", truncatedEmail)
			httperror.BadRequest("reCAPTCHA token invalid", nil, nil).Render(w)
			return
		}
	}

	// Step 4: Find the user by email and send the forgot password message with no DB connection held
	resetToken, storedEmail, err := h.AuthManager.ForgotPassword(ctx, forgotPasswordRequest.Email)
	switch {
	case errors.Is(err, auth.ErrUserNotFound):
		// if we don't find the user by email, we just return an ok response
		// to prevent malicious client from searching accounts in the system
		log.Ctx(ctx).Errorf("in forgot password handler, email not found: %s", truncatedEmail)
	case errors.Is(err, auth.ErrUserHasValidToken):
		log.Ctx(ctx).Errorf("in forgot password handler, a valid reset token already exists")
	case err != nil:
		httperror.InternalError(ctx, "Failed to reset password", fmt.Errorf("resetting password: %w", err), nil).Render(w)
		return
	default:
		sendErr := h.SendForgotPasswordMessage(ctx, *tnt.SDPUIBaseURL, storedEmail, resetToken)
		if sendErr != nil {
			// Revoke the undelivered token so the user is not locked out by the cooldown on retry.
			if invErr := h.AuthManager.InvalidateResetToken(ctx, resetToken); invErr != nil {
				log.Ctx(ctx).Errorf("invalidating reset token after a failed send: %v", invErr)
			}
			httperror.InternalError(ctx, "Failed to reset password", fmt.Errorf("sending forgot password message: %w", sendErr), nil).Render(w)
			return
		}
	}

	responseBody := map[string]string{
		"message": "If the email you provided is associated with an account, you'll receive a password reset link shortly. Check your inbox and spam folders.",
	}
	httpjson.RenderStatus(w, http.StatusOK, responseBody, httpjson.JSON)
}

func (h ForgotPasswordHandler) SendForgotPasswordMessage(ctx context.Context, uiBaseURL, email, resetToken string) error {
	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("getting organization data: %w", err)
	}

	resetPasswordURL, err := url.Parse(uiBaseURL)
	if err != nil {
		return fmt.Errorf("parsing reset password link: %w", err)
	}
	resetPasswordURL.Path = path.Join(resetPasswordURL.Path, "reset-password")
	resetPasswordURL.RawQuery = url.Values{"token": {resetToken}}.Encode()

	forgotPasswordData := htmltemplate.StaffForgotPasswordEmailMessageTemplate{
		ResetPasswordLink: resetPasswordURL.String(),
		OrganizationName:  organization.Name,
	}
	messageContent, err := htmltemplate.ExecuteHTMLTemplateForStaffForgotPasswordEmailMessage(forgotPasswordData)
	if err != nil {
		return fmt.Errorf("executing forgot password message template: %w", err)
	}

	msg := message.Message{
		ToEmail: email,
		Title:   forgotPasswordMessageTitle,
		Body:    messageContent,
		Type:    message.MessageTypeUserForgotPassword,
		TemplateVariables: map[message.TemplateVariable]string{
			message.TemplateVarResetPasswordLink: resetPasswordURL.String(),
			message.TemplateVarOrgName:           organization.Name,
		},
	}
	err = h.MessengerClient.SendMessage(ctx, msg)
	if err != nil {
		return fmt.Errorf("sending forgot password email for %s: %w", utils.TruncateString(email, 3), err)
	}

	return nil
}
