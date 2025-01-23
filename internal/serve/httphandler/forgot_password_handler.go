package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
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

func (h ForgotPasswordHandler) validateRequest(req ForgotPasswordRequest) *httperror.HTTPError {
	v := validators.NewValidator()
	v.Check(req.Email != "", "email", "email is required")
	v.Check(h.ReCAPTCHADisabled || req.ReCAPTCHAToken != "", "recaptcha_token", "reCAPTCHA token is required")

	if v.HasErrors() {
		return httperror.BadRequest("", nil, v.Errors)
	}

	return nil
}

// ServeHTTP implements the http.Handler interface.
func (h ForgotPasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Step 1: Get tenant from context
	tnt, err := tenant.GetTenantFromContext(ctx)
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
	if httpErr := h.validateRequest(forgotPasswordRequest); httpErr != nil {
		httpErr.Render(w)
		return
	}

	truncatedEmail := utils.TruncateString(forgotPasswordRequest.Email, 3)

	// Step 3: Run the reCAPTCHA validation if it is enabled
	if !h.ReCAPTCHADisabled {
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

	// Step 4: Find the user by email and send the forgot password message
	err = db.RunInTransaction(ctx, h.Models.DBConnectionPool, nil, func(tx db.DBTransaction) error {
		resetToken, txErr := h.AuthManager.ForgotPassword(ctx, tx, forgotPasswordRequest.Email)
		if txErr != nil {
			return fmt.Errorf("resetting password: %w", txErr)
		}

		sendErr := h.SendForgotPasswordMessage(ctx, *tnt.SDPUIBaseURL, forgotPasswordRequest.Email, resetToken)
		if sendErr != nil {
			return fmt.Errorf("sending forgot password message: %w", sendErr)
		}

		return nil
	})
	if err != nil {
		// if we don't find the user by email, we just return an ok response
		// to prevent malicious client from searching accounts in the system
		if errors.Is(err, auth.ErrUserNotFound) {
			log.Ctx(ctx).Errorf("in forgot password handler, email not found: %s", truncatedEmail)
		} else if errors.Is(err, auth.ErrUserHasValidToken) {
			log.Ctx(ctx).Errorf("in forgot password handler, user has a valid token")
		} else {
			httperror.InternalError(ctx, err.Error(), err, nil).Render(w)
			return
		}
	}

	responseBody := map[string]string{
		"message": "Password reset requested. If the email is registered, you'll receive a reset link shortly. Check your inbox and spam folders.",
	}
	httpjson.RenderStatus(w, http.StatusOK, responseBody, httpjson.JSON)
}

func (h ForgotPasswordHandler) SendForgotPasswordMessage(ctx context.Context, uiBaseURL, email, resetToken string) error {
	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		return fmt.Errorf("getting organization data: %w", err)
	}

	resetPasswordLink, err := url.JoinPath(uiBaseURL, "reset-password")
	if err != nil {
		return fmt.Errorf("getting reset password link: %w", err)
	}

	forgotPasswordData := htmltemplate.StaffForgotPasswordEmailMessageTemplate{
		ResetToken:        resetToken,
		ResetPasswordLink: resetPasswordLink,
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
	}
	err = h.MessengerClient.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("sending forgot password email for %s: %w", utils.TruncateString(email, 3), err)
	}

	return nil
}
