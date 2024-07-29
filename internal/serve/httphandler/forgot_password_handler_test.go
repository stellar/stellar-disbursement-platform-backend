package httphandler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	urllib "net/url"
	"slices"
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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
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
	roleManagerMock := &auth.RoleManagerMock{}
	reCAPTCHAValidatorMock := &validators.ReCAPTCHAValidatorMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
		auth.WithCustomRoleManagerOption(roleManagerMock),
	)

	messengerClientMock := &message.MessengerClientMock{}
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
	const email = "valid@email.com"
	defaultUserRoles := []string{data.OwnerUserRole.String()}
	user := &auth.User{
		ID:    "userID",
		Email: email,
	}
	const reCAPTCHAToken = "validToken"
	defaultReqBody := fmt.Sprintf(`
				{ 
					"email": "%s",
					"recaptcha_token": "%s"
				}`, user.Email, reCAPTCHAToken)

	userByEmailSetup := func(user *auth.User, userRoles []string) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		roleManagerMock.
			On("GetUserRoles", mock.Anything, user).
			Return(userRoles, nil).
			Once()
	}
	t.Run("Should return http status 200 on a valid request", func(t *testing.T) {
		usersRoles := make([][]string, 2)
		// role cannot bypass reCAPTCHA
		usersRoles[0] = []string{data.OwnerUserRole.String()}
		// role can bypass reCAPTCHA
		usersRoles[1] = []string{data.OwnerUserRole.String(), data.APIUserRole.String()}
		for _, userRoles := range usersRoles {
			requestBody := fmt.Sprintf(`
				{ 
					"email": "%s"
				}`, user.Email)

			authenticatorMock.
				On("ForgotPassword", mock.Anything, email).
				Return("resetToken", nil).
				Once()
			userByEmailSetup(user, userRoles)
			if !slices.Contains(userRoles, data.APIUserRole.String()) {
				requestBody = defaultReqBody
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, reCAPTCHAToken).
					Return(true, nil).
					Once()
			}
			rr := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(requestBody))
			require.NoError(t, err)

			resetPasswordLink, err := urllib.JoinPath(uiBaseURL, "reset-password")
			require.NoError(t, err)

			content, err := htmltemplate.ExecuteHTMLTemplateForForgotPasswordMessage(htmltemplate.ForgotPasswordMessageTemplate{
				ResetToken:        "resetToken",
				ResetPasswordLink: resetPasswordLink,
				OrganizationName:  "MyCustomAid",
			})
			require.NoError(t, err)

			msg := message.Message{
				ToEmail: email,
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
		}
	})

	t.Run("Should return http status 500 when the reset password link is invalid", func(t *testing.T) {
		tntInvalidUIBaseURL := tenant.Tenant{
			SDPUIBaseURL: &[]string{"%invalid%"}[0],
		}
		ctxTenantWithInvalidUIBaseURL := tenant.SaveTenantInContext(context.Background(), &tntInvalidUIBaseURL)

		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(
			ctxTenantWithInvalidUIBaseURL,
			http.MethodPost,
			url,
			strings.NewReader(defaultReqBody),
		)
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), "valid@email.com").
			Return("resetToken", nil).
			Once()
		userByEmailSetup(user, defaultUserRoles)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, reCAPTCHAToken).
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

		invalidUser := &auth.User{
			ID:    "userID",
			Email: "not_found@email.com",
		}
		authenticatorMock.
			On("ForgotPassword", req.Context(), invalidUser.Email).
			Return("", auth.ErrUserNotFound).
			Once()
		userByEmailSetup(invalidUser, defaultUserRoles)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, reCAPTCHAToken).
			Return(true, nil).
			Once()

		http.HandlerFunc(handler.ServeHTTP).ServeHTTP(rr, req)

		resp := rr.Result()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Should return an http status ok even if the user has a valid token", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(defaultReqBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), user.Email).
			Return("", auth.ErrUserHasValidToken).
			Once()
		userByEmailSetup(user, defaultUserRoles)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, reCAPTCHAToken).
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
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(defaultReqBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), user.Email).
			Return("resetToken", nil).
			Once()
		userByEmailSetup(user, defaultUserRoles)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, reCAPTCHAToken).
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
			ToEmail: user.Email,
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
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(defaultReqBody))
		require.NoError(t, err)

		authenticatorMock.
			On("ForgotPassword", req.Context(), user.Email).
			Return("", errors.New("unexpected error")).
			Once()
		userByEmailSetup(user, defaultUserRoles)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, reCAPTCHAToken).
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
		rr := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(defaultReqBody))
		require.NoError(t, err)

		userByEmailSetup(user, defaultUserRoles)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", req.Context(), reCAPTCHAToken).
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

		userByEmailSetup(user, defaultUserRoles)
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
	roleManagerMock.AssertExpectations(t)
	messengerClientMock.AssertExpectations(t)
	reCAPTCHAValidatorMock.AssertExpectations(t)
}
