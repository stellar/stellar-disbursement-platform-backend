package httphandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ResetPasswordHandlerPost(t *testing.T) {
	const url = "/reset-password"
	const method = "POST"

	authenticatorMock := &auth.AuthenticatorMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
	)

	handler := &ResetPasswordHandler{
		AuthManager: authManager,
	}

	t.Run("Should return http status 200 on a valid request", func(t *testing.T) {
		requestBody := `{ "password": "password123", "reset_token": "goodtoken" }`

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest(method, url, strings.NewReader(requestBody))

		authenticatorMock.
			On("ResetPassword", req.Context(), "goodtoken", "password123").
			Return(nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Should return an error with an invalid token", func(t *testing.T) {
		requestBody := `{"password":"password123","reset_token":"badtoken"}`

		rr := httptest.NewRecorder()
		req, _ := http.NewRequest(method, url, strings.NewReader(requestBody))

		authenticatorMock.
			On("ResetPassword", req.Context(), "badtoken", "password123").
			Return(auth.ErrInvalidResetPasswordToken).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `
			{
				"error": "invalid reset password token"
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("Should require both password and reset_token params", func(t *testing.T) {
		requestBody := `{"password":""}`

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
					"password":"password is required",
					"reset_token":"reset token is required"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	authenticatorMock.AssertExpectations(t)
}
