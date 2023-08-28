package httphandler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
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

	// validate request
	v := validators.NewValidator()

	v.Check(resetPasswordRequest.Password != "", "password", "password is required")
	v.Check(resetPasswordRequest.ResetToken != "", "reset_token", "reset token is required")

	if v.HasErrors() {
		httperror.BadRequest("request invalid", err, v.Errors).Render(w)
		return
	}

	ctx := r.Context()

	// Reset password email with a valid token
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
