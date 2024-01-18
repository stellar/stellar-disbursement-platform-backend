package httphandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const forgotPasswordMessageTitle = "Reset Account Password"

// ForgotPasswordHandler searches for the user that is requesting a password reset
// and sends an email with a link to access the password reset page.
type ForgotPasswordHandler struct {
	AuthManager        auth.AuthManager
	MessengerClient    message.MessengerClient
	UIBaseURL          string
	Models             *data.Models
	ReCAPTCHAValidator validators.ReCAPTCHAValidator
	ReCAPTCHADisabled  bool
}

type ForgotPasswordRequest struct {
	Email          string `json:"email"`
	ReCAPTCHAToken string `json:"recaptcha_token"`
}

type ForgotPasswordResponseBody struct {
	Message string `json:"message"`
}

// ServeHTTP implements the http.Handler interface.
func (h ForgotPasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var forgotPasswordRequest ForgotPasswordRequest

	err := json.NewDecoder(r.Body).Decode(&forgotPasswordRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	ctx := r.Context()

	if !h.ReCAPTCHADisabled {
		// validating reCAPTCHA Token
		isValid, recaptchaErr := h.ReCAPTCHAValidator.IsTokenValid(ctx, forgotPasswordRequest.ReCAPTCHAToken)
		if recaptchaErr != nil {
			httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", recaptchaErr, nil).Render(w)
			return
		}

		if !isValid {
			log.Ctx(ctx).Errorf("reCAPTCHA token is invalid for request with email %s", utils.TruncateString(forgotPasswordRequest.Email, 3))
			httperror.BadRequest("reCAPTCHA token invalid", nil, nil).Render(w)
			return
		}
	}

	// validate request
	v := validators.NewValidator()

	v.Check(forgotPasswordRequest.Email != "", "email", "email is required")

	if v.HasErrors() {
		httperror.BadRequest("request invalid", err, v.Errors).Render(w)
		return
	}

	resetToken, err := h.AuthManager.ForgotPassword(ctx, forgotPasswordRequest.Email)
	// if we don't find the user by email, we just return an ok response
	// to prevent malicious client from searching accounts in the system
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			log.Ctx(ctx).Errorf("error in forgot password handler, email not found: %s", forgotPasswordRequest.Email)
		} else if errors.Is(err, auth.ErrUserHasValidToken) {
			log.Ctx(ctx).Errorf("error in forgot password handler, user has a valid token")
		} else {
			httperror.InternalError(ctx, "", err, nil).Render(w)
			return
		}
	}

	if err == nil {
		organization, err := h.Models.Organizations.Get(ctx)
		if err != nil {
			err = fmt.Errorf("error getting organization data: %w", err)
			httperror.InternalError(ctx, "", err, nil).Render(w)
			return
		}

		resetPasswordLink, err := url.JoinPath(h.UIBaseURL, "reset-password")
		if err != nil {
			err = fmt.Errorf("error getting reset password link: %w", err)
			log.Ctx(ctx).Error(err)
			httperror.InternalError(ctx, "", err, nil).Render(w)
			return
		}

		forgotPasswordData := htmltemplate.ForgotPasswordMessageTemplate{
			ResetToken:        resetToken,
			ResetPasswordLink: resetPasswordLink,
			OrganizationName:  organization.Name,
		}
		messageContent, err := htmltemplate.ExecuteHTMLTemplateForForgotPasswordMessage(forgotPasswordData)
		if err != nil {
			err = fmt.Errorf("error executing forgot password message template: %w", err)
			httperror.InternalError(ctx, "", err, nil).Render(w)
			return
		}

		msg := message.Message{
			ToEmail: forgotPasswordRequest.Email,
			Title:   forgotPasswordMessageTitle,
			Message: messageContent,
		}
		err = h.MessengerClient.SendMessage(msg)
		if err != nil {
			err = fmt.Errorf("error sending forgot password email for email %s: %w", forgotPasswordRequest.Email, err)
			httperror.InternalError(ctx, "", err, nil).Render(w)
			return
		}
	}

	responseBody := ForgotPasswordResponseBody{
		Message: "Password reset requested. If the email is registered, you'll receive a reset link shortly. Check your inbox and spam folders.",
	}

	httpjson.RenderStatus(w, http.StatusOK, responseBody, httpjson.JSON)
}
