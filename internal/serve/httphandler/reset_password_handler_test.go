package httphandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

func Test_ResetPasswordHandler_validateRequest(t *testing.T) {
	pwValidator, err := utils.GetPasswordValidatorInstance()
	require.NoError(t, err)
	handler := ResetPasswordHandler{
		PasswordValidator: pwValidator,
	}

	type Req struct {
		body ResetPasswordRequest
	}
	testCases := []struct {
		name     string
		req      Req
		expected *httperror.HTTPError
	}{
		{
			name: "🔴 invalid body fields",
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"digit":             "password must contain at least one numberical digit",
				"length":            "password length must be between 12 and 36 characters",
				"lowercase":         "password must contain at least one lowercase letter",
				"reset_token":       "reset token is required",
				"special character": "password must contain at least one special character",
				"uppercase":         "password must contain at least one uppercase letter",
			}),
		},
		{
			name: "🟢 valid request",
			req: Req{
				body: ResetPasswordRequest{
					Password:   "!1Az?2By.3Cx",
					ResetToken: "goodtoken",
				},
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := handler.validateRequest(tc.req.body)
			if tc.expected == nil {
				require.Nil(t, err)
			} else {
				require.Equal(t, tc.expected, err)
			}
		})
	}
}

func Test_ResetPasswordHandlerPost(t *testing.T) {
	const url = "/reset-password"
	const method = "POST"

	authenticatorMock := &auth.AuthenticatorMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
	)
	pwValidator, err := utils.GetPasswordValidatorInstance()
	require.NoError(t, err)
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
		require.Contains(t, buf.String(), "[ResetUserPassword] - Successfully reset password for user with token go...en")
	})

	t.Run("Should return an error with an invalid token", func(t *testing.T) {
		requestBody := `{"password":"!1Az?2By.3Cx","reset_token":"badtoken"}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(method, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ResetPassword", req.Context(), "badtoken", "!1Az?2By.3Cx").
			Return(auth.ErrInvalidResetPasswordToken).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `{"error": "Invalid reset password token."}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("Should return an error with an expired token", func(t *testing.T) {
		requestBody := `{"password":"!1Az?2By.3Cx","reset_token":"expiredtoken"}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(method, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ResetPassword", req.Context(), "expiredtoken", "!1Az?2By.3Cx").
			Return(auth.ErrExpiredResetPasswordToken).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `{"error": "Reset password token expired, please request a new token through the forgot password flow."}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("Should require both password and reset_token params", func(t *testing.T) {
		requestBody := `{}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(method, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `
			{
				"error":"The request was invalid in some way.",
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
