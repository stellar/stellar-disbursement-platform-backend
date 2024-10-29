package httphandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	urllib "net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_ForgotPasswordHandler_validateRequest(t *testing.T) {
	type Req struct {
		body ForgotPasswordRequest
	}
	testCases := []struct {
		name     string
		handler  ForgotPasswordHandler
		req      Req
		expected *httperror.HTTPError
	}{
		{
			name: "🔴 invalid body fields with reCAPTCHA enabled",
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"email":           "email is required",
				"recaptcha_token": "reCAPTCHA token is required",
			}),
		},
		{
			name: "🔴 invalid body fields with reCAPTCHA disabled",
			handler: ForgotPasswordHandler{
				ReCAPTCHADisabled: true,
			},
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"email": "email is required",
			}),
		},
		{
			name: "🟢 valid request with reCAPTCHA enabled",
			req: Req{
				body: ForgotPasswordRequest{
					Email:          "foobar@test.com",
					ReCAPTCHAToken: "XyZ",
				},
			},
			expected: nil,
		},
		{
			name: "🟢 valid request with mfa & reCAPTCHA disabled",
			req: Req{
				body: ForgotPasswordRequest{
					Email: "foobar@test.com",
				},
			},
			handler: ForgotPasswordHandler{
				ReCAPTCHADisabled: true,
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.handler.validateRequest(tc.req.body)
			if tc.expected == nil {
				require.Nil(t, err)
			} else {
				require.Equal(t, tc.expected, err)
			}
		})
	}
}

func Test_ForgotPasswordHandler_ServeHTTP(t *testing.T) {
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

	messengerClientMock := message.NewMessengerClientMock(t)
	handler := &ForgotPasswordHandler{
		AuthManager:        authManager,
		MessengerClient:    messengerClientMock,
		Models:             models,
		ReCAPTCHAValidator: reCAPTCHAValidatorMock,
		ReCAPTCHADisabled:  false,
	}

	uiBaseURL := "https://sdp.com"
	tnt := tenant.Tenant{
		SDPUIBaseURL: &uiBaseURL,
	}
	ctx := tenant.SaveTenantInContext(context.Background(), &tnt)

	t.Run("Should not create a token when email provider fails", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), mock.Anything, "valid@email.com").
			Return("resetToken", nil).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		messengerClientMock.
			On("SendMessage", mock.Anything).
			Return(errors.New("unexpected error")).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		// check that there are no tokens created in the database for this user
		sql := `SELECT * FROM auth_user_password_reset JOIN auth_users ON auth_user_password_reset.auth_user_id = auth_users.id WHERE auth_users.email = $1`
		rows, queryErr := dbConnectionPool.QueryContext(context.Background(), sql, "valid@email.com")
		require.NoError(t, queryErr)
		assert.False(t, rows.Next())
	})

	t.Run("Should return http status 200 on a valid request", func(t *testing.T) {
		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), mock.Anything, "valid@email.com").
			Return("resetToken", nil).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		resetPasswordLink, err := urllib.JoinPath(uiBaseURL, "reset-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForStaffForgotPasswordEmailMessage(htmltemplate.StaffForgotPasswordEmailMessageTemplate{
			ResetToken:        "resetToken",
			ResetPasswordLink: resetPasswordLink,
			OrganizationName:  "MyCustomAid",
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: "valid@email.com",
			Title:   forgotPasswordMessageTitle,
			Body:    content,
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
		tntInvalidUIBaseURL := tenant.Tenant{
			SDPUIBaseURL: &[]string{"%invalid%"}[0],
		}
		ctxTenantWithInvalidUIBaseURL := tenant.SaveTenantInContext(context.Background(), &tntInvalidUIBaseURL)

		requestBody := `
		{ 
			"email": "valid@email.com" ,
			"recaptcha_token": "validToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctxTenantWithInvalidUIBaseURL, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), mock.Anything, "valid@email.com").
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), mock.Anything, "not_found@email.com").
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), mock.Anything, "valid@email.com").
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)
		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expectedBody := `
			{
				"error":"The request was invalid in some way.",
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), mock.Anything, "valid@email.com").
			Return("resetToken", nil).
			Once()
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "validToken").
			Return(true, nil).
			Once()

		resetPasswordLink, err := urllib.JoinPath(uiBaseURL, "reset-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForStaffForgotPasswordEmailMessage(htmltemplate.StaffForgotPasswordEmailMessageTemplate{
			ResetToken:        "resetToken",
			ResetPasswordLink: resetPasswordLink,
			OrganizationName:  "MyCustomAid",
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: "valid@email.com",
			Title:   forgotPasswordMessageTitle,
			Body:    content,
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
				"error": "running atomic function in RunInTransactionWithResult: sending forgot password message: sending forgot password email for val...com: unexpected error"
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), mock.Anything, "valid@email.com").
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
				"error": "running atomic function in RunInTransactionWithResult: resetting password: calling authenticator's ForgotPassword: unexpected error"
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		reCAPTCHAValidatorMock.
			On("IsTokenValid", req.Context(), "validToken").
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
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

	t.Run("returns Unauthorized when tenant is not in the context", func(t *testing.T) {
		ctxWithoutTenant := context.Background()

		requestBody := `
		{
			"email": "valid@email.com" ,
			"recaptcha_token": "invalidToken"
		}`

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctxWithoutTenant, http.MethodPost, url, strings.NewReader(requestBody))
		require.NoError(t, err)

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	authenticatorMock.AssertExpectations(t)
	reCAPTCHAValidatorMock.AssertExpectations(t)
}
