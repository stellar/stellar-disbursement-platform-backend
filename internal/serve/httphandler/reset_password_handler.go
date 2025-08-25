package httphandler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	authUtils "github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

// ResetPasswordHandler resets the user password by receiving a valid reset token
// and the new password.
type ResetPasswordHandler struct {
	AuthManager       auth.AuthManager
	PasswordValidator *authUtils.PasswordValidator
}

type ResetPasswordRequest struct {
	Password   string `json:"password"`
	ResetToken string `json:"reset_token"`
}

func (h ResetPasswordHandler) validateRequest(req ResetPasswordRequest) *httperror.HTTPError {
	v := validators.NewValidator()
	if validatePasswordError := h.PasswordValidator.ValidatePassword(req.Password); validatePasswordError != nil {
		for k, msg := range validatePasswordError.FailedValidations() {
			v.AddError(k, msg)
		}
	}

	v.Check(req.ResetToken != "", "reset_token", "reset token is required")

	if v.HasErrors() {
		return httperror.BadRequest("", nil, v.Errors)
	}

	return nil
}

// ServeHTTP implements the http.Handler interface.
func (h ResetPasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Step 1: Decode and validate the incoming request
	var resetPasswordRequest ResetPasswordRequest
	err := json.NewDecoder(r.Body).Decode(&resetPasswordRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}
	if httpErr := h.validateRequest(resetPasswordRequest); httpErr != nil {
		httpErr.Render(w)
		return
	}

	// Step 2: Reset password with a valid token
	err = h.AuthManager.ResetPassword(ctx, resetPasswordRequest.ResetToken, resetPasswordRequest.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrExpiredResetPasswordToken):
			httperror.BadRequest("Reset password token expired, please request a new token through the forgot password flow.", err, nil).Render(w)
		case errors.Is(err, auth.ErrInvalidResetPasswordToken):
			httperror.BadRequest("Invalid reset password token.", err, nil).Render(w)
		default:
			httperror.InternalError(ctx, "Cannot reset password", err, nil).Render(w)
		}
		return
	}

	truncatedToken := utils.TruncateString(resetPasswordRequest.ResetToken, len(resetPasswordRequest.ResetToken)/4)
	log.Ctx(ctx).Infof("[ResetUserPassword] - Successfully reset password for user with token %s", truncatedToken)
	httpjson.RenderStatus(w, http.StatusOK, nil, httpjson.JSON)
}
