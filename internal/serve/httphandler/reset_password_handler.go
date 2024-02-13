package httphandler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
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

// ServeHTTP implements the http.Handler interface.
func (h ResetPasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var resetPasswordRequest ResetPasswordRequest

	err := json.NewDecoder(r.Body).Decode(&resetPasswordRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	// validate password
	badRequestExtras := map[string]interface{}{}
	err = h.PasswordValidator.ValidatePassword(resetPasswordRequest.Password)
	if err != nil {
		var validatePasswordError *authUtils.ValidatePasswordError
		if errors.As(err, &validatePasswordError) {
			for k, v := range validatePasswordError.FailedValidations() {
				badRequestExtras[k] = v
			}
			log.Ctx(ctx).Errorf("validating password in ResetPasswordHandler.ServeHTTP: %v", err)
		} else {
			httperror.InternalError(ctx, "Cannot update user password", err, nil).Render(w)
			return
		}
	}
	// validate reset token
	if resetPasswordRequest.ResetToken == "" {
		badRequestExtras["reset_token"] = "reset token is required"
	}
	// return 400 if there are any errors
	if len(badRequestExtras) > 0 {
		httperror.BadRequest("request invalid", err, badRequestExtras).Render(w)
		return
	}

	// Reset password with a valid token
	err = h.AuthManager.ResetPassword(ctx, resetPasswordRequest.ResetToken, resetPasswordRequest.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidResetPasswordToken) {
			httperror.BadRequest("invalid reset password token", err, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Cannot reset password", err, nil).Render(w)
		return
	}

	log.Infof("[ResetUserPassword] - Reset password for user with token %s",
		utils.TruncateString(resetPasswordRequest.ResetToken, len(resetPasswordRequest.ResetToken)/4))
	httpjson.RenderStatus(w, http.StatusOK, nil, httpjson.JSON)
}
