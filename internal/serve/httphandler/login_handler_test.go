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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func Test_LoginHandler_validateRequest(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	type Req struct {
		body    LoginRequest
		headers map[string]string
	}
	testCases := []struct {
		name     string
		handler  LoginHandler
		req      Req
		expected *httperror.HTTPError
	}{
		{
			name: "🔴 invalid body and headers fields",
			handler: LoginHandler{
				ReCAPTCHADisabled: false,
				MFADisabled:       false,
				Models:            models,
			},
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"email":           "email is required",
				"password":        "password is required",
				"recaptcha_token": "reCAPTCHA token is required",
				"Device-ID":       "Device-ID header is required",
			}),
		},
		{
			name: "🔴 invalid body fields with reCAPTCHA and MFA disabled",
			handler: LoginHandler{
				ReCAPTCHADisabled: true,
				MFADisabled:       true,
				Models:            models,
			},
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"email":    "email is required",
				"password": "password is required",
			}),
		},
		{
			name: "🟢 valid request with mfa & reCAPTCHA enabled",
			handler: LoginHandler{
				ReCAPTCHADisabled: false,
				MFADisabled:       false,
				Models:            models,
			},
			req: Req{
				body: LoginRequest{
					Email:          "foobar@test.com",
					Password:       "password",
					ReCAPTCHAToken: "XyZ",
				},
				headers: map[string]string{DeviceIDHeader: "safari-xyz"},
			},
			expected: nil,
		},
		{
			name: "🟢 valid request with mfa & reCAPTCHA disabled",
			req: Req{
				body: LoginRequest{
					Email:    "foobar@test.com",
					Password: "password",
				},
			},
			handler: LoginHandler{
				ReCAPTCHADisabled: true,
				MFADisabled:       true,
				Models:            models,
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mfaDisabled := tc.handler.MFADisabled
			captchaDisabled := tc.handler.ReCAPTCHADisabled
			err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
				MFADisabled:     &mfaDisabled,
				CAPTCHADisabled: &captchaDisabled,
			})
			require.NoError(t, err)

			headers := http.Header{}
			for k, v := range tc.req.headers {
				headers.Set(k, v)
			}

			err = tc.handler.validateRequest(ctx, tc.req.body, headers)
			if tc.expected == nil {
				require.Nil(t, err)
			} else {
				require.Equal(t, tc.expected, err)
			}
		})
	}
}

func Test_LoginHandler_ServeHTTP(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	type Req struct {
		body    string
		headers map[string]string
	}
	defaultValidRequest := Req{
		body: `{
			"email": "foobar@test.com",
			"password": "pass1234",
			"recaptcha_token": "XyZ"
		}`,
		headers: map[string]string{DeviceIDHeader: "safari-xyz"},
	}
	usr := auth.User{ID: "user-ID"}

	testCases := []struct {
		name              string
		ReCAPTCHADisabled bool
		MFAADisabled      bool
		prepareMocks      func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock)
		req               Req
		wantStatusCode    int
		wantResponseBody  string
	}{
		{
			name:             "🔴[400] invalid body",
			req:              Req{body: "invalid json"},
			wantStatusCode:   http.StatusBadRequest,
			wantResponseBody: `{"error":"The request was invalid in some way."}`,
		},
		{
			name:              "🔴[400](ReCAPTCHADisabled=false,MFADisabled=false) missing fields",
			ReCAPTCHADisabled: false,
			MFAADisabled:      false,
			req:               Req{body: "{}"},
			wantStatusCode:    http.StatusBadRequest,
			wantResponseBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"email": "email is required",
					"password": "password is required",
					"recaptcha_token":"reCAPTCHA token is required",
					"Device-ID":"Device-ID header is required"
				}
			}`,
		},
		{
			name:              "🔴[400](ReCAPTCHADisabled=true,MFADisabled=true) missing fields",
			ReCAPTCHADisabled: true,
			MFAADisabled:      true,
			req:               Req{body: "{}"},
			wantStatusCode:    http.StatusBadRequest,
			wantResponseBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"email": "email is required",
					"password": "password is required"
				}
			}`,
		},
		{
			name: "🔴[401] reCaptcha validation returns an error",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(false, errors.New("error requesting verify reCAPTCHA token")).
					Once()
			},
			wantStatusCode:   http.StatusUnauthorized,
			wantResponseBody: `{"error": "Cannot validate reCAPTCHA token"}`,
		},
		{
			name: "🔴[400] reCAPTCHA is not valid",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(false, nil).
					Once()
			},
			wantStatusCode:   http.StatusBadRequest,
			wantResponseBody: `{"error": "reCAPTCHA token invalid"}`,
		},
		{
			name: "🔴[401] invalid crecentials",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("", auth.ErrInvalidCredentials).
					Once()
			},
			wantStatusCode: http.StatusUnauthorized,
			wantResponseBody: `{
				"error": "Not authorized.",
				"extras": {
					"details": "Incorrect email or password"
				}
			}`,
		},
		{
			name: "🔴[500] authentication throws unexpected error",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("", errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Cannot authenticate user credentials"}`,
		},
		{
			name:              "🟢[200](ReCAPTCHADisabled=false,MFADisabled=true) successful login",
			ReCAPTCHADisabled: false,
			MFAADisabled:      true,
			req:               defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
		{
			name:              "🟢[200](ReCAPTCHADisabled=true,MFADisabled=true) successful login",
			ReCAPTCHADisabled: true,
			MFAADisabled:      true,
			req: Req{
				body:    `{"email": "foobar@test.com","password": "pass1234"}`,
				headers: map[string]string{DeviceIDHeader: "safari-xyz"},
			},
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
		{
			name: "🔴[500] MFA throws unexpected error checking if the device is remembered",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
				authManagerMock.
					On("MFADeviceRemembered", mock.Anything, "safari-xyz", "user-ID").
					Return(false, errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Cannot check if MFA code is remembered"}`,
		},
		{
			name: "🟢[200](ReCAPTCHADisabled=false,MFADisabled=false) successful login when MFA device is remembered",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
				authManagerMock.
					On("MFADeviceRemembered", mock.Anything, "safari-xyz", "user-ID").
					Return(true, nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
		{
			name: "🔴[500] MFA throws unexpected error checking when getting new MFA code",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
				authManagerMock.
					On("MFADeviceRemembered", mock.Anything, "safari-xyz", "user-ID").
					Return(false, nil).
					Once()
				authManagerMock.
					On("GetMFACode", mock.Anything, "safari-xyz", "user-ID").
					Return("", errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Cannot get MFA code"}`,
		},
		{
			name: "🔴[500] failed to send MFA code",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
				authManagerMock.
					On("MFADeviceRemembered", mock.Anything, "safari-xyz", "user-ID").
					Return(false, nil).
					Once()
				authManagerMock.
					On("GetMFACode", mock.Anything, "safari-xyz", "user-ID").
					Return("123456", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything, mock.Anything).
					Return(errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Failed to send send MFA code"}`,
		},
		{
			name: "🟢[200](ReCAPTCHADisabled=false,MFADisabled=false) MFA code was sent",
			req:  defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
				authManagerMock.
					On("MFADeviceRemembered", mock.Anything, "safari-xyz", "user-ID").
					Return(false, nil).
					Once()
				authManagerMock.
					On("GetMFACode", mock.Anything, "safari-xyz", "user-ID").
					Return("123456", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything, mock.Anything).
					Return(nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"message": "MFA code sent to email. Check your inbox and spam folders."}`,
		},
		{
			name:              "🟢[200] successful login with nil org settings - falls back to env settings (CAPTCHA disabled)",
			ReCAPTCHADisabled: true,
			MFAADisabled:      false,
			req: Req{
				body:    `{"email": "foobar@test.com","password": "pass1234"}`,
				headers: map[string]string{DeviceIDHeader: "safari-xyz"},
			},
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				ctx := context.Background()
				err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
					Name: "Test Org",
				})
				require.NoError(t, err)

				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
				authManagerMock.
					On("MFADeviceRemembered", mock.Anything, "safari-xyz", "user-ID").
					Return(true, nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
		{
			name:              "🟢[200] successful login with nil org settings - falls back to env settings (MFA disabled)",
			ReCAPTCHADisabled: false,
			MFAADisabled:      true,
			req:               defaultValidRequest,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				ctx := context.Background()
				err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
					Name: "Test Org",
				})
				require.NoError(t, err)

				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "XyZ").
					Return(true, nil).
					Once()
				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
		{
			name:              "🟢[200] both org settings nil - falls back to both env settings disabled",
			ReCAPTCHADisabled: true,
			MFAADisabled:      true,
			req: Req{
				body:    `{"email": "foobar@test.com","password": "pass1234"}`,
				headers: map[string]string{DeviceIDHeader: "safari-xyz"},
			},
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				ctx := context.Background()
				err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
					Name: "Test Org",
				})
				require.NoError(t, err)

				authManagerMock.
					On("Authenticate", mock.Anything, "foobar@test.com", "pass1234").
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUser", mock.Anything, "token").
					Return(&usr, nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reCAPTCHAValidatorMock := validators.NewReCAPTCHAValidatorMock(t)
			authManagerMock := auth.NewAuthManagerMock(t)
			messengerClientMock := message.NewMessengerClientMock(t)

			ctx := context.Background()
			mfaDisabled := tc.MFAADisabled
			captchaDisabled := tc.ReCAPTCHADisabled
			err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
				MFADisabled:     &mfaDisabled,
				CAPTCHADisabled: &captchaDisabled,
			})
			require.NoError(t, err)

			if tc.prepareMocks != nil {
				tc.prepareMocks(t, reCAPTCHAValidatorMock, authManagerMock, messengerClientMock)
			}

			h := LoginHandler{
				Models:             models,
				MFADisabled:        tc.MFAADisabled,
				ReCAPTCHADisabled:  tc.ReCAPTCHADisabled,
				ReCAPTCHAValidator: reCAPTCHAValidatorMock,
				AuthManager:        authManagerMock,
				MessengerClient:    messengerClientMock,
			}

			req, err := http.NewRequest(http.MethodPost, "/login", strings.NewReader(tc.req.body))
			for k, v := range tc.req.headers {
				req.Header.Set(k, v)
			}
			require.NoError(t, err)
			rw := httptest.NewRecorder()

			h.ServeHTTP(rw, req)

			assert.Equal(t, tc.wantStatusCode, rw.Code)
			assert.JSONEq(t, tc.wantResponseBody, rw.Body.String())
		})
	}
}
