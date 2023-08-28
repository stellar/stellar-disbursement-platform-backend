package httphandler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	authUtils "github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

// ResetPasswordHandler resets the user password by receiving a valid reset token
// and the new password.
type ResetPasswordHandler struct {
	AuthManager auth.AuthManager
}

type ResetPasswordRequest struct {
	Password   string `json:"password"`
	ResetToken string `json:"reset_token"`
}

// ServeHTTP implements the http.Handler interface.
func (h ResetPasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resetPasswordRequest ResetPasswordRequest

	err := json.NewDecoder(r.Body).Decode(&resetPasswordRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	// validate password
	payloadErrorExtras := map[string]interface{}{}
	var validatePasswordError *authUtils.ValidatePasswordError
	err = authUtils.ValidatePassword(resetPasswordRequest.Password)
	if err != nil && errors.As(err, &validatePasswordError) {
		for k, v := range validatePasswordError.FailedValidations() {
			payloadErrorExtras[k] = v
		}
	}
	// validate reset token
	if resetPasswordRequest.ResetToken == "" {
		payloadErrorExtras["reset_token"] = "reset token is required"
	}
	// return 400 if there are any errors
	if len(payloadErrorExtras) > 0 {
		httperror.BadRequest("request invalid", err, payloadErrorExtras).Render(w)
		return
	}

	// Reset password with a valid token
	ctx := r.Context()
	err = h.AuthManager.ResetPassword(ctx, resetPasswordRequest.ResetToken, resetPasswordRequest.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidResetPasswordToken) {
			httperror.BadRequest("invalid reset password token", err, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Cannot reset password", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, nil, httpjson.JSON)
}
