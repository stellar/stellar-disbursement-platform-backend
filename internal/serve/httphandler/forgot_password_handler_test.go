package httphandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
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

	uiBaseURL := "https://sdp.com"
	tnt := tenant.Tenant{SDPUIBaseURL: &uiBaseURL}
	defaultSuccessishBody := `{"message": "Password reset requested. If the email is registered, you'll receive a reset link shortly. Check your inbox and spam folders."}`
	// ctxWithTenant := tenant.SaveTenantInContext(ctxWithoutTenant, &tnt)

	testCases := []struct {
		name              string
		hasTenant         bool
		ReCAPTCHADisabled bool
		prepareMocks      func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock)
		reqBody           string
		wantStatusCode    int
		wantResponseBody  string
	}{
		{
			name:             "🔴[401] no tenant in the context",
			hasTenant:        false,
			wantStatusCode:   http.StatusUnauthorized,
			wantResponseBody: `{"error":"Not authorized."}`,
		},
		{
			name:             "🔴[400] invalid body",
			hasTenant:        true,
			reqBody:          "invalid json",
			wantStatusCode:   http.StatusBadRequest,
			wantResponseBody: `{"error":"The request was invalid in some way."}`,
		},
		{
			name:              "🔴[400](ReCAPTCHADisabled=false) missing [email, recaptcha_token]",
			hasTenant:         true,
			ReCAPTCHADisabled: false,
			reqBody:           "{}",
			wantStatusCode:    http.StatusBadRequest,
			wantResponseBody: `{
				"error":"The request was invalid in some way.",
				"extras": {
					"email": "email is required",
					"recaptcha_token": "reCAPTCHA token is required"
				}
			}`,
		},
		{
			name:              "🔴[400](ReCAPTCHADisabled=true) missing [email]",
			hasTenant:         true,
			ReCAPTCHADisabled: true,
			reqBody:           "{}",
			wantStatusCode:    http.StatusBadRequest,
			wantResponseBody: `{
				"error":"The request was invalid in some way.",
				"extras": {
					"email": "email is required"
				}
			}`,
		},
		{
			name:      "🔴[500] when reCAPTCHA validator throws an unexpected error",
			hasTenant: true,
			reqBody:   `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(false, errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Cannot validate reCAPTCHA token"}`,
		},
		{
			name:      "🔴[400] when mfa_code is invalid",
			hasTenant: true,
			reqBody:   `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(false, nil).
					Once()
			},
			wantStatusCode:   http.StatusBadRequest,
			wantResponseBody: `{"error": "reCAPTCHA token invalid"}`,
		},
		{
			name:      "🟡[200] return Ok-ish even when user was not found",
			hasTenant: true,
			reqBody:   `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("ForgotPassword", mock.Anything, mock.Anything, "foobar@test.com").
					Return("", auth.ErrUserNotFound).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: defaultSuccessishBody,
		},
		{
			name:      "🟡[200] return Ok-ish if user already has a valid token",
			hasTenant: true,
			reqBody:   `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("ForgotPassword", mock.Anything, mock.Anything, "foobar@test.com").
					Return("", auth.ErrUserHasValidToken).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: defaultSuccessishBody,
		},
		{
			name:      "🔴[500] when the SendMessage method fails",
			hasTenant: true,
			reqBody:   `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("ForgotPassword", mock.Anything, mock.Anything, "foobar@test.com").
					Return("resetToken", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything).
					Return(errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Failed to reset password"}`,
		},
		{
			name:              "🟢[200](ReCAPTCHADisabled=false) successfully handle forgot password",
			ReCAPTCHADisabled: false,
			hasTenant:         true,
			reqBody:           `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("ForgotPassword", mock.Anything, mock.Anything, "foobar@test.com").
					Return("resetToken", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything).
					Return(nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: defaultSuccessishBody,
		},
		{
			name:              "🟢[200](ReCAPTCHADisabled=true) successfully handle forgot password",
			ReCAPTCHADisabled: true,
			hasTenant:         true,
			reqBody:           `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				authManagerMock.
					On("ForgotPassword", mock.Anything, mock.Anything, "foobar@test.com").
					Return("resetToken", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything).
					Return(nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: defaultSuccessishBody,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reCAPTCHAValidatorMock := validators.NewReCAPTCHAValidatorMock(t)
			authManagerMock := auth.NewAuthManagerMock(t)
			messengerClientMock := message.NewMessengerClientMock(t)
			if tc.prepareMocks != nil {
				tc.prepareMocks(t, reCAPTCHAValidatorMock, authManagerMock, messengerClientMock)
			}

			h := ForgotPasswordHandler{
				Models:             models,
				ReCAPTCHADisabled:  tc.ReCAPTCHADisabled,
				ReCAPTCHAValidator: reCAPTCHAValidatorMock,
				AuthManager:        authManagerMock,
				MessengerClient:    messengerClientMock,
			}

			ctx := context.Background()
			if tc.hasTenant {
				ctx = tenant.SaveTenantInContext(ctx, &tnt)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/forgot-password", strings.NewReader(tc.reqBody))
			require.NoError(t, err)
			rw := httptest.NewRecorder()

			h.ServeHTTP(rw, req)

			assert.Equal(t, tc.wantStatusCode, rw.Code)
			assert.JSONEq(t, tc.wantResponseBody, rw.Body.String())
		})
	}
}
