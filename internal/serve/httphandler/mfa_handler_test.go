package httphandler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const mfaEndpoint = "/mfa"

func Test_MFAHandler_validateRequest(t *testing.T) {
	t.Parallel()
	type Req struct {
		body     MFARequest
		deviceID string
	}
	testCases := []struct {
		name     string
		handler  MFAHandler
		req      Req
		expected *httperror.HTTPError
	}{
		{
			name: "🔴 invalid body and headers fields",
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"mfa_code":        "MFA Code is required",
				"recaptcha_token": "reCAPTCHA token is required",
				"Device-ID":       "Device-ID header is required",
			}),
		},
		{
			name: "🔴 invalid body fields with reCAPTCHA disabled",
			handler: MFAHandler{
				ReCAPTCHADisabled: true,
			},
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"mfa_code":  "MFA Code is required",
				"Device-ID": "Device-ID header is required",
			}),
		},
		{
			name: "🟢 valid request with reCAPTCHA enabled",
			req: Req{
				body: MFARequest{
					MFACode:        "123456",
					ReCAPTCHAToken: "XyZ",
				},
				deviceID: "safari-xyz",
			},
			expected: nil,
		},
		{
			name: "🟢 valid request with reCAPTCHA disabled",
			req: Req{
				body: MFARequest{
					MFACode: "123456",
				},
				deviceID: "safari-xyz",
			},
			handler: MFAHandler{
				ReCAPTCHADisabled: true,
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.handler.validateRequest(tc.req.body, tc.req.deviceID)
			if tc.expected == nil {
				require.Nil(t, err)
			} else {
				require.Equal(t, tc.expected, err)
			}
		})
	}
}

func Test_MFAHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	dbConnectionPool := testutils.OpenTestDBConnectionPool(t)
	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	deviceID := "safari-xyz"

	testCases := []struct {
		name              string
		ReCAPTCHADisabled bool
		prepareMocks      func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock)
		reqBody           string
		deviceID          string
		wantStatusCode    int
		wantResponseBody  string
	}{
		{
			name:             "🔴[400] invalid body",
			reqBody:          "invalid json",
			wantStatusCode:   http.StatusBadRequest,
			wantResponseBody: `{"error":"The request was invalid in some way."}`,
		},
		{
			name:           "🔴[400] missing [mfa_code,recaptcha_token,Device-ID]",
			reqBody:        "{}",
			deviceID:       "",
			wantStatusCode: http.StatusBadRequest,
			wantResponseBody: `{
				"error":"The request was invalid in some way.",
				"extras": {
					"mfa_code": "MFA Code is required",
					"recaptcha_token": "reCAPTCHA token is required",
					"Device-ID": "Device-ID header is required"
				}
			}`,
		},
		{
			name:              "🔴[400](ReCAPTCHADisabled=true) missing [mfa_code,Device-ID]",
			ReCAPTCHADisabled: true,
			reqBody:           "{}",
			deviceID:          "",
			wantStatusCode:    http.StatusBadRequest,
			wantResponseBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"mfa_code": "MFA Code is required",
					"Device-ID": "Device-ID header is required"
				}
			}`,
		},
		{
			name:     "🔴[500] when reCAPTCHA validator throws an unexpected error",
			reqBody:  `{"mfa_code":"123456","recaptcha_token":"token"}`,
			deviceID: deviceID,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(false, errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Cannot validate reCAPTCHA token"}`,
		},
		{
			name:     "🔴[400] when reCAPTCHA token is deemed invalid",
			reqBody:  `{"mfa_code":"123456","recaptcha_token":"token"}`,
			deviceID: deviceID,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(false, nil).
					Once()
			},
			wantStatusCode:   http.StatusBadRequest,
			wantResponseBody: `{"error": "reCAPTCHA token invalid"}`,
		},
		{
			name:     "🔴[401] when mfa_code is invalid",
			reqBody:  `{"mfa_code":"123456","recaptcha_token":"token"}`,
			deviceID: deviceID,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("AuthenticateMFA", mock.Anything, deviceID, "123456", mock.AnythingOfType("bool")).
					Return("", auth.ErrMFACodeInvalid).
					Once()
			},
			wantStatusCode:   http.StatusUnauthorized,
			wantResponseBody: `{"error": "Not authorized."}`,
		},
		{
			name:     "🔴[500] when the MFA validation returns an unexpedted error",
			reqBody:  `{"mfa_code":"123456","recaptcha_token":"token"}`,
			deviceID: deviceID,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("AuthenticateMFA", mock.Anything, deviceID, "123456", mock.AnythingOfType("bool")).
					Return("", errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Cannot authenticate user"}`,
		},
		{
			name:     "🔴[500] when GetUserID returns an unexpedted error",
			reqBody:  `{"mfa_code":"123456","recaptcha_token":"token"}`,
			deviceID: deviceID,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("AuthenticateMFA", mock.Anything, deviceID, "123456", mock.AnythingOfType("bool")).
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUserID", mock.Anything, "token").
					Return("", errors.New("unexpected error")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Cannot get user ID"}`,
		},
		{
			name:     "🟢[200](ReCAPTCHADisabled=false) successfully validate MFA",
			reqBody:  `{"mfa_code":"123456","recaptcha_token":"token"}`,
			deviceID: deviceID,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("AuthenticateMFA", mock.Anything, deviceID, "123456", mock.AnythingOfType("bool")).
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUserID", mock.Anything, "token").
					Return("user_id", nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
		{
			name:              "🟢[200](ReCAPTCHADisabled=true) successfully validate MFA",
			ReCAPTCHADisabled: true,
			reqBody:           `{"mfa_code":"123456"}`,
			deviceID:          deviceID,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock) {
				authManagerMock.
					On("AuthenticateMFA", mock.Anything, deviceID, "123456", mock.AnythingOfType("bool")).
					Return("token", nil).
					Once()
				authManagerMock.
					On("GetUserID", mock.Anything, "token").
					Return("user_id", nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: `{"token": "token"}`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reCAPTCHAValidatorMock := validators.NewReCAPTCHAValidatorMock(t)
			authManager := auth.NewAuthManagerMock(t)
			if tc.prepareMocks != nil {
				tc.prepareMocks(t, reCAPTCHAValidatorMock, authManager)
			}

			mfaHandler := MFAHandler{
				AuthManager:        authManager,
				ReCAPTCHAValidator: reCAPTCHAValidatorMock,
				Models:             models,
				ReCAPTCHADisabled:  tc.ReCAPTCHADisabled,
			}

			req := httptest.NewRequest(http.MethodPost, mfaEndpoint, strings.NewReader(tc.reqBody))
			if tc.deviceID != "" {
				req.Header.Set(DeviceIDHeader, tc.deviceID)
			}
			rw := httptest.NewRecorder()

			mfaHandler.ServeHTTP(rw, req)

			assert.Equal(t, tc.wantStatusCode, rw.Code)
			assert.JSONEq(t, tc.wantResponseBody, rw.Body.String())
		})
	}
}
