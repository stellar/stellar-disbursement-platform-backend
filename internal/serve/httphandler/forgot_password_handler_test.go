package httphandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

func Test_ForgotPasswordHandler_validateRequest(t *testing.T) {
	dbConnectionPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

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
			handler: ForgotPasswordHandler{
				ReCAPTCHADisabled: false,
				Models:            models,
			},
			expected: httperror.BadRequest("", nil, map[string]any{
				"email":           "email is required",
				"recaptcha_token": "reCAPTCHA token is required",
			}),
		},
		{
			name: "🔴 invalid body fields with reCAPTCHA disabled",
			handler: ForgotPasswordHandler{
				ReCAPTCHADisabled: true,
				Models:            models,
			},
			expected: httperror.BadRequest("", nil, map[string]any{
				"email": "email is required",
			}),
		},
		{
			name: "🟢 valid request with reCAPTCHA enabled",
			handler: ForgotPasswordHandler{
				ReCAPTCHADisabled: false,
				Models:            models,
			},
			req: Req{
				body: ForgotPasswordRequest{
					Email:          "foobar@test.com",
					ReCAPTCHAToken: "XyZ",
				},
			},
			expected: nil,
		},
		{
			name: "🟢 valid request with reCAPTCHA disabled",
			handler: ForgotPasswordHandler{
				ReCAPTCHADisabled: true,
				Models:            models,
			},
			req: Req{
				body: ForgotPasswordRequest{
					Email: "foobar@test.com",
				},
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			captchaDisabled := tc.handler.ReCAPTCHADisabled
			err := models.Organizations.Update(ctx, &data.OrganizationUpdate{
				CAPTCHADisabled: &captchaDisabled,
			})
			require.NoError(t, err)

			err = tc.handler.validateRequest(ctx, tc.req.body)
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
	tnt := schema.Tenant{SDPUIBaseURL: &uiBaseURL}
	defaultSuccessishBody := `{"message": "If the email you provided is associated with an account, you'll receive a password reset link shortly. Check your inbox and spam folders."}`

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
					On("ForgotPassword", mock.Anything, "foobar@test.com").
					Return("", "", auth.ErrUserNotFound).
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
					On("ForgotPassword", mock.Anything, "foobar@test.com").
					Return("", "", auth.ErrUserHasValidToken).
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
					On("ForgotPassword", mock.Anything, "foobar@test.com").
					Return("resetToken", "foobar@test.com", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything, mock.Anything).
					Return(errors.New("unexpected error")).
					Once()
				authManagerMock.
					On("InvalidateResetToken", mock.Anything, "resetToken").
					Return(nil).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Failed to reset password"}`,
		},
		{
			name:      "🔴[500] when the SendMessage method fails and the token cannot be invalidated",
			hasTenant: true,
			reqBody:   `{"email": "foobar@test.com","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("ForgotPassword", mock.Anything, "foobar@test.com").
					Return("resetToken", "foobar@test.com", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything, mock.Anything).
					Return(errors.New("unexpected error")).
					Once()
				authManagerMock.
					On("InvalidateResetToken", mock.Anything, "resetToken").
					Return(errors.New("db is gone")).
					Once()
			},
			wantStatusCode:   http.StatusInternalServerError,
			wantResponseBody: `{"error": "Failed to reset password"}`,
		},
		{
			name:      "🟢[200] sends the message to the stored email, not the submitted one",
			hasTenant: true,
			reqBody:   `{"email": " Foobar@Test.com ","recaptcha_token":"token"}`,
			prepareMocks: func(t *testing.T, reCAPTCHAValidatorMock *validators.ReCAPTCHAValidatorMock, authManagerMock *auth.AuthManagerMock, messengerClientMock *message.MessengerClientMock) {
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()
				authManagerMock.
					On("ForgotPassword", mock.Anything, " Foobar@Test.com ").
					Return("resetToken", "foobar@test.com", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything, mock.MatchedBy(func(msg message.Message) bool {
						return msg.ToEmail == "foobar@test.com"
					})).
					Return(nil).
					Once()
			},
			wantStatusCode:   http.StatusOK,
			wantResponseBody: defaultSuccessishBody,
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
					On("ForgotPassword", mock.Anything, "foobar@test.com").
					Return("resetToken", "foobar@test.com", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything, mock.Anything).
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
					On("ForgotPassword", mock.Anything, "foobar@test.com").
					Return("resetToken", "foobar@test.com", nil).
					Once()
				messengerClientMock.
					On("SendMessage", mock.Anything, mock.Anything).
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
				ctx = sdpcontext.SetTenantInContext(ctx, &tnt)
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

// Concurrent requests for one email used to hold a pool connection each while the leader waited for a second one,
// and without the per-user row lock several of them issued a token and sent an email.
func Test_ForgotPasswordHandler_ServeHTTP_concurrentRequestsDoNotExhaustThePool(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN) // default pool: 20 connections
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	passwordEncrypter := auth.NewDefaultPasswordEncrypter()
	authManager := auth.NewAuthManager(auth.WithDefaultAuthenticatorOption(dbConnectionPool, passwordEncrypter, time.Hour))
	user := auth.CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypter, false)

	messengerClientMock := message.NewMessengerClientMock(t)
	messengerClientMock.On("SendMessage", mock.Anything, mock.Anything).Return(nil)

	uiBaseURL := "https://sdp.com"
	tnt := schema.Tenant{SDPUIBaseURL: &uiBaseURL}
	h := ForgotPasswordHandler{
		Models:            models,
		ReCAPTCHADisabled: true,
		AuthManager:       authManager,
		MessengerClient:   messengerClientMock,
	}

	const requests = 30 // more than the pool holds
	statuses := make([]int, requests)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range requests {
		req, reqErr := http.NewRequestWithContext(sdpcontext.SetTenantInContext(ctx, &tnt), http.MethodPost, "/forgot-password", strings.NewReader(`{"email": "`+user.Email+`"}`))
		require.NoError(t, reqErr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rw := httptest.NewRecorder()
			h.ServeHTTP(rw, req)
			statuses[i] = rw.Code
		}()
	}
	close(start)

	// An unrelated query through the same pool must get a connection while the burst runs.
	victimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var one int
	require.NoError(t, dbConnectionPool.GetContext(victimCtx, &one, "SELECT 1"))

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("concurrent forgot-password requests did not drain: pool deadlock")
	}

	for i, code := range statuses {
		assert.Equalf(t, http.StatusOK, code, "request %d", i)
	}

	var validTokens int
	require.NoError(t, dbConnectionPool.GetContext(ctx, &validTokens, "SELECT count(*) FROM auth_user_password_reset WHERE auth_user_id = $1 AND is_valid = true", user.ID))
	assert.Equal(t, 1, validTokens)
	messengerClientMock.AssertNumberOfCalls(t, "SendMessage", 1)

	sqlDB, err := dbConnectionPool.SqlDB(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, sqlDB.Stats().InUse)
}
