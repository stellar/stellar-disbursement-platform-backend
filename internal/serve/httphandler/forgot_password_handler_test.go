package httphandler

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	urllib "net/url"
	"strings"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stretchr/testify/mock"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ForgotPasswordHandler(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	const url = "/forgot-password"

	authenticatorMock := &auth.AuthenticatorMock{}
	reCAPTCHAValidatorMock := &validators.ReCAPTCHAValidatorMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
	)

	uiBaseURL := "https://sdp.com"
	messengerClientMock := &message.MessengerClientMock{}
	handler := &ForgotPasswordHandler{
		AuthManager:        authManager,
		MessengerClient:    messengerClientMock,
		Models:             models,
		UIBaseURL:          uiBaseURL,
		ReCAPTCHAValidator: reCAPTCHAValidatorMock,
		ReCAPTCHADisabled:  false,
	}

	t.Run("Should return http status 200 on a valid request", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), "valid@email.com").
			Return("resetToken", nil).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		resetPasswordLink, err := urllib.JoinPath(uiBaseURL, "reset-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForForgotPasswordMessage(htmltemplate.ForgotPasswordMessageTemplate{
			ResetToken:        "resetToken",
			ResetPasswordLink: resetPasswordLink,
			OrganizationName:  "MyCustomAid",
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: "valid@email.com",
			Title:   forgotPasswordMessageTitle,
			Message: content,
		}
		messengerClientMock.
			On("SendMessage", msg).
			Return(nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Should return http status 500 when the reset password link is invalid", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), "valid@email.com").
			Return("resetToken", nil).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		http.HandlerFunc(ForgotPasswordHandler{
			AuthManager:        authManager,
			MessengerClient:    messengerClientMock,
			Models:             models,
			UIBaseURL:          "%invalid%",
			ReCAPTCHAValidator: reCAPTCHAValidatorMock,
			ReCAPTCHADisabled:  false,
		}.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("Should return an http status ok even if the email is not found", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "not_found@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), "not_found@email.com").
			Return("", auth.ErrUserNotFound).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Should return an http status ok even if the user has a valid token", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), "valid@email.com").
			Return("", auth.ErrUserHasValidToken).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Should require email param", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "",
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `
			{
				"error":"request invalid",
				"extras": {
					"email":"email is required"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("Should return http status 500 when error sending email", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com",
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), "valid@email.com").
			Return("resetToken", nil).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		resetPasswordLink, err := urllib.JoinPath(uiBaseURL, "reset-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForForgotPasswordMessage(htmltemplate.ForgotPasswordMessageTemplate{
			ResetToken:        "resetToken",
			ResetPasswordLink: resetPasswordLink,
			OrganizationName:  "MyCustomAid",
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: "valid@email.com",
			Title:   forgotPasswordMessageTitle,
			Message: content,
		}
		messengerClientMock.
			On("SendMessage", msg).
			Return(errors.New("unexpected error")).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `
			{
				"error": "An internal error occurred while processing this request."
			}
		`
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("Should return http status 500 when authenticator fails", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com",
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), "valid@email.com").
			Return("", errors.New("unexpected error")).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `
			{
				"error": "An internal error occurred while processing this request."
			}
		`
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, expectedBody, string(respBody))
	})

	t.Run("Should return http status 500 when reCAPTCHA validator returns an error", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(false, errors.New("error requesting verify reCAPTCHA token")).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		wantsBody := `
			{
				"error": "Cannot validate reCAPTCHA token"
			}
		`
		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("Should return http status 400 when reCAPTCHA token is invalid", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "invalidToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "invalidToken").
			Return(false, nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		wantsBody := `
			{
				"error": "reCAPTCHA token invalid"
			}
		`
		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	authenticatorMock.AssertExpectations(t)
	messengerClientMock.AssertExpectations(t)
	reCAPTCHAValidatorMock.AssertExpectations(t)
}
