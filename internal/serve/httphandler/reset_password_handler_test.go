package httphandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

func Test_ResetPasswordHandlerPost(t *testing.T) {
	const url = "/reset-password"
	const method = "POST"

	authenticatorMock := &auth.AuthenticatorMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
	)
	pwValidator, _ := utils.GetPasswordValidatorInstance()
	handler := &ResetPasswordHandler{
		AuthManager:       authManager,
		PasswordValidator: pwValidator,
	}

	t.Run("Should return http status 200 on a valid request", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		requestBody := `{ "password": "!1Az?2By.3Cx", "reset_token": "goodtoken" }`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(method, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ResetPassword", req.Context(), "goodtoken", "!1Az?2By.3Cx").
			Return(nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// validate logs
		require.Contains(t, buf.String(), "[ResetUserPassword] - Reset password for user with token go...en")
	})

	t.Run("Should return an error with an invalid token", func(t *testing.T) {
		requestBody := `{"password":"!1Az?2By.3Cx","reset_token":"badtoken"}`

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest(method, url, strings.NewReader(requestBody))

		authenticatorMock.
			On("ResetPassword", req.Context(), "badtoken", "!1Az?2By.3Cx").
			Return(auth.ErrInvalidResetPasswordToken).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `{"error": "invalid reset password token"}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("Should require both password and reset_token params", func(t *testing.T) {
		requestBody := `{}`

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest(method, url, strings.NewReader(requestBody))

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `
			{
				"error":"request invalid",
				"extras": {
					"digit":"password must contain at least one numberical digit",
					"length":"password length must be between 12 and 36 characters",
					"lowercase":"password must contain at least one lowercase letter",
					"reset_token":"reset token is required",
					"special character":"password must contain at least one special character",
					"uppercase":"password must contain at least one uppercase letter",
					"reset_token":"reset token is required"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	authenticatorMock.AssertExpectations(t)
}
