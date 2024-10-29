package httphandler

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"
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
)

func Test_LoginHandler_validateRequest(t *testing.T) {
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
			},
			expected: httperror.BadRequest("", nil, map[string]interface{}{
				"email":    "email is required",
				"password": "password is required",
			}),
		},
		{
			name: "🟢 valid request with mfa & reCAPTCHA enabled",
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
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			for k, v := range tc.req.headers {
				headers.Set(k, v)
			}

			err := tc.handler.validateRequest(tc.req.body, headers)
			if tc.expected == nil {
				require.Nil(t, err)
			} else {
				require.Equal(t, tc.expected, err)
			}
		})
	}
}

func Test_LoginHandler_ServeHTTP(t *testing.T) {
	r := chi.NewRouter()

	authenticatorMock := &auth.AuthenticatorMock{}
	jwtManagerMock := &auth.JWTManagerMock{}
	roleManagerMock := &auth.RoleManagerMock{}
	reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
		auth.WithCustomJWTManagerOption(jwtManagerMock),
		auth.WithCustomRoleManagerOption(roleManagerMock),
	)

	handler := &LoginHandler{
		AuthManager:        authManager,
		ReCAPTCHAValidator: reCAPTCHAValidator,
		ReCAPTCHADisabled:  false,
		MFADisabled:        true,
	}

	const url = "/login"

	t.Run("returns error when body is invalid", func(t *testing.T) {
		r.Post(url, handler.ServeHTTP)

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{}`))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "The request was invalid in some way.",
				"extras": {
					"email": "email is required",
					"password": "password is required",
					"recaptcha_token":"reCAPTCHA token is required"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		req, err = http.NewRequest(http.MethodPost, url, strings.NewReader(`{"email": "testuser", "recaptcha_token": "XyZ"}`))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "The request was invalid in some way.",
				"extras": {
					"password": "password is required"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err = http.NewRequest(http.MethodPost, url, strings.NewReader(`"invalid"`))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `{"error": "The request was invalid in some way."}`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
		assert.Contains(t, buf.String(), "decoding the request body")
	})

	t.Run("returns error when an unexpected error occurs validating the credentials", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()

		authenticatorMock.
			On("ValidateCredentials", mock.Anything, "testuser", "pass1234").
			Return(nil, errors.New("unexpected error")).
			Once()

		r.Post(url, handler.ServeHTTP)

		reqBody := `
			{
				"email": "testuser",
				"password": "pass1234",
				"recaptcha_token": "XyZ"
			}
		`

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Cannot authenticate user credentials"
			}
		`
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
		assert.Contains(t, buf.String(), "Cannot authenticate user credentials")
	})

	t.Run("returns error when the credentials are incorrect", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()

		authenticatorMock.
			On("ValidateCredentials", mock.Anything, "testuser", "pass1234").
			Return(nil, auth.ErrInvalidCredentials).
			Once()

		r.Post(url, handler.ServeHTTP)

		reqBody := `
			{
				"email": "testuser",
				"password": "pass1234",
				"recaptcha_token": "XyZ"
			}
		`

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Not authorized.",
				"extras": {
					"details": "Incorrect email or password"
				}
			}
		`
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns error when unable to validate recaptcha", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(false, errors.New("error requesting verify reCAPTCHA token")).
			Once()

		r.Post(url, handler.ServeHTTP)

		reqBody := `
			{
				"email": "testuser",
				"password": "pass1234",
				"recaptcha_token": "XyZ"
			}
		`

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Cannot validate reCAPTCHA token"
			}
		`
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns error when recaptcha token is invalid", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(false, nil).
			Once()

		r.Post(url, handler.ServeHTTP)

		reqBody := `
			{
				"email": "testuser",
				"password": "pass1234",
				"recaptcha_token": "XyZ"
			}
		`

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "reCAPTCHA token invalid"
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns the token correctly", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		user := &auth.User{
			ID:    "user-ID",
			Email: "email",
		}

		authenticatorMock.
			On("ValidateCredentials", mock.Anything, "testuser", "pass1234").
			Return(user, nil).
			Once()
		roleManagerMock.
			On("GetUserRoles", mock.Anything, user).
			Return([]string{"role1"}, nil).
			Once()
		jwtManagerMock.
			On("GenerateToken", mock.Anything, user, mock.AnythingOfType("time.Time")).
			Return("token123", nil).
			Once()
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()
		jwtManagerMock.
			On("ValidateToken", mock.Anything, "token123").
			Return(true, nil).
			Once()
		jwtManagerMock.
			On("GetUserFromToken", mock.Anything, "token123").
			Return(user, nil).
			Once()
		authenticatorMock.
			On("GetUser", mock.Anything, "user-ID").
			Return(user, nil).
			Once()
		roleManagerMock.
			On("GetUserRoles", mock.Anything, user).
			Return([]string{"role1"}, nil).
			Once()

		r.Post(url, handler.ServeHTTP)

		reqBody := `
			{
				"email": "testuser",
				"password": "pass1234",
				"recaptcha_token": "XyZ"	
			}
		`

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(reqBody))
		require.NoError(t, err)

		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"token": "token123"}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "[UserLogin] - Logged in user with account ID user-ID")
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
	reCAPTCHAValidator.AssertExpectations(t)
}

func Test_LoginHandler_ServeHTTP_MFA(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	authenticatorMock := &auth.AuthenticatorMock{}
	jwtManagerMock := &auth.JWTManagerMock{}
	roleManagerMock := &auth.RoleManagerMock{}
	mfaManagerMock := &auth.MFAManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
		auth.WithCustomJWTManagerOption(jwtManagerMock),
		auth.WithCustomRoleManagerOption(roleManagerMock),
		auth.WithCustomMFAManagerOption(mfaManagerMock),
	)
	messengerClientMock := &message.MessengerClientMock{}
	loginHandler := &LoginHandler{
		AuthManager:       authManager,
		ReCAPTCHADisabled: true,
		MFADisabled:       false,
		Models:            models,
		MessengerClient:   messengerClientMock,
	}

	user := &auth.User{
		ID:    "userID",
		Email: "testuser@mail.com",
	}
	authenticatorMock.
		On("ValidateCredentials", mock.Anything, "testuser@mail.com", "pass1234").
		Return(user, nil)
	roleManagerMock.
		On("GetUserRoles", mock.Anything, user).
		Return([]string{"role1"}, nil)
	jwtManagerMock.
		On("GenerateToken", mock.Anything, user, mock.AnythingOfType("time.Time")).
		Return("token123", nil)
	jwtManagerMock.
		On("ValidateToken", mock.Anything, "token123").
		Return(true, nil)
	jwtManagerMock.
		On("GetUserFromToken", mock.Anything, "token123").
		Return(user, nil)

	deviceID := "safari-xyz"

	t.Run("error getting user from token", func(t *testing.T) {
		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(nil, errors.New("weird error happened")).
			Once()

		body := LoginRequest{Email: "testuser@mail.com", Password: "pass1234"}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "An internal error occurred while processing this request")
	})

	t.Run("error when deviceID header is empty", func(t *testing.T) {
		body := LoginRequest{Email: "testuser@mail.com", Password: "pass1234"}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "Device-ID header is required")
	})

	t.Run("error validating MFA device", func(t *testing.T) {
		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(user, nil).
			Once()
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, "userID").
			Return(false, errors.New("weird error happened")).
			Once()

		body := LoginRequest{Email: "testuser@mail.com", Password: "pass1234"}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "An internal error occurred while processing this request")
	})

	t.Run("when device is remembered, return token", func(t *testing.T) {
		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(user, nil).
			Once()
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, "userID").
			Return(true, nil).
			Once()

		body := LoginRequest{Email: "testuser@mail.com", Password: "pass1234"}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
		require.JSONEq(t, `{"token": "token123"}`, rw.Body.String())
	})

	t.Run("error generating MFA code", func(t *testing.T) {
		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(user, nil).
			Once()
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, "userID").
			Return(false, nil).
			Once()
		mfaManagerMock.
			On("GenerateMFACode", mock.Anything, deviceID, "userID").
			Return("", errors.New("some weird error")).
			Once()

		body := LoginRequest{Email: "testuser@mail.com", Password: "pass1234"}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot get MFA code")
	})

	t.Run("error sending MFA message", func(t *testing.T) {
		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(user, nil).
			Once()
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, "userID").
			Return(false, nil).
			Once()
		mfaManagerMock.
			On("GenerateMFACode", mock.Anything, deviceID, "userID").
			Return("123123", nil).
			Once()
		messengerClientMock.
			On("SendMessage", mock.Anything).
			Return(errors.New("weird error sending message")).
			Once()

		body := LoginRequest{Email: "testuser@mail.com", Password: "pass1234"}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "An internal error occurred while processing this request")
	})

	t.Run("🎉  Successful login", func(t *testing.T) {
		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(user, nil).
			Once()
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, "userID").
			Return(false, nil).
			Once()
		mfaManagerMock.
			On("GenerateMFACode", mock.Anything, deviceID, "userID").
			Return("123123", nil).
			Once()

		content, err := htmltemplate.ExecuteHTMLTemplateForStaffMFAEmailMessage(htmltemplate.StaffMFAEmailMessageTemplate{
			OrganizationName: "MyCustomAid",
			MFACode:          "123123",
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: "testuser@mail.com",
			Title:   mfaMessageTitle,
			Body:    content,
		}
		messengerClientMock.
			On("SendMessage", msg).
			Return(nil).
			Once()

		body := LoginRequest{Email: "testuser@mail.com", Password: "pass1234"}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
		require.JSONEq(t, `{"message": "MFA code sent to email. Check your inbox and spam folders."}`, rw.Body.String())
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
	mfaManagerMock.AssertExpectations(t)
	messengerClientMock.AssertExpectations(t)
}
