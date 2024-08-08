package httphandler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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

func authenticateSetup(
	t *testing.T,
	authenticatorMock *auth.AuthenticatorMock,
	jwtManagerMock *auth.JWTManagerMock,
	user *auth.User, password, userToken string,
) {
	t.Helper()
	authenticatorMock.
		On("ValidateCredentials", mock.Anything, user.Email, password).
		Return(user, nil).
		Once()
	jwtManagerMock.
		On("GenerateToken", mock.Anything, user, mock.AnythingOfType("time.Time")).
		Return(userToken, nil).
		Once()
}

func Test_LoginRequest_validate(t *testing.T) {
	lr := LoginRequest{
		Email:          "",
		Password:       "",
		ReCAPTCHAToken: "",
	}

	extras := map[string]interface{}{"email": "email is required", "password": "password is required"}
	expectedErr := httperror.BadRequest("Request invalid", nil, extras)

	err := lr.validate()
	assert.Equal(t, expectedErr, err)

	lr = LoginRequest{
		Email:          "email@email.com",
		Password:       "",
		ReCAPTCHAToken: "XyZ",
	}

	extras = map[string]interface{}{"password": "password is required"}
	expectedErr = httperror.BadRequest("Request invalid", nil, extras)

	err = lr.validate()
	assert.Equal(t, expectedErr, err)
}

func Test_LoginHandler(t *testing.T) {
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
	const email = "testuser@email.com"
	const password = "pass1234"
	const defaultReCAPTCHAToken = "XyZ"
	defaultReqBody := fmt.Sprintf(
		`{"email": "%s", "password": "%s", "recaptcha_token": "%s"}`, email, password, defaultReCAPTCHAToken)
	user := &auth.User{
		ID:    "user-ID",
		Email: email,
		Roles: []string{data.OwnerUserRole.String()},
	}
	const defaultUserToken = "token123"

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
				"error": "Request invalid",
				"extras": {
					"email": "email is required",
					"password": "password is required"
				}
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))

		req, err = http.NewRequest(http.MethodPost, url, strings.NewReader(`{"email": "testuser"}`))
		require.NoError(t, err)

		w = httptest.NewRecorder()

		r.ServeHTTP(w, req)

		resp = w.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody = `
			{
				"error": "Request invalid",
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
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, defaultReCAPTCHAToken).
			Return(true, nil).
			Once()
		authenticatorMock.
			On("ValidateCredentials", mock.Anything, email, password).
			Return(nil, errors.New("unexpected error")).
			Once()

		r.Post(url, handler.ServeHTTP)

		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(defaultReqBody))
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
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, defaultReCAPTCHAToken).
			Return(true, nil).
			Once()
		authenticatorMock.
			On("ValidateCredentials", mock.Anything, email, password).
			Return(nil, auth.ErrInvalidCredentials).
			Once()

		r.Post(url, handler.ServeHTTP)

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(defaultReqBody))
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
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, defaultReCAPTCHAToken).
			Return(false, errors.New("error requesting verify reCAPTCHA token")).
			Once()

		r.Post(url, handler.ServeHTTP)

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(defaultReqBody))
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
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, defaultReCAPTCHAToken).
			Return(false, nil).
			Once()

		r.Post(url, handler.ServeHTTP)

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(defaultReqBody))
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

		usersRoles := [][]string{
			{data.OwnerUserRole.String()},                            // API roles cannot bypass reCAPTCHA
			{data.OwnerUserRole.String(), data.APIUserRole.String()}, // API roles can bypass reCAPTCHA
		}
		for _, userRoles := range usersRoles {
			targetUser := &auth.User{
				ID:    "user-ID",
				Email: email,
				Roles: userRoles,
			}
			authenticatorMock.
				On("GetUserByEmail", mock.Anything, user.Email).
				Return(targetUser, nil).
				Once()
			reqBody := `
				{
					"email": "testuser@email.com",
					"password": "pass1234"
				}`
			if !slices.Contains(userRoles, data.APIUserRole.String()) { // <-------- bypasses recaptcha when APIUserRole is present.
				reCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, defaultReCAPTCHAToken).
					Return(true, nil).
					Once()
				reqBody = defaultReqBody
			}
			authenticateSetup(t, authenticatorMock, jwtManagerMock, targetUser, password, defaultUserToken)

			r.Post(url, handler.ServeHTTP)

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
		}
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
	reCAPTCHAValidator.AssertExpectations(t)
}

func Test_LoginHandlerr_ServeHTTP_MFA(t *testing.T) {
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
	const userToken = "token123"
	const password = "pass1234"
	const deviceID = "safari-xyz"
	const mfaCode = "123123"

	t.Run("error when deviceID header is empty", func(t *testing.T) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		authenticateSetup(t, authenticatorMock, jwtManagerMock, user, password, userToken)

		body := LoginRequest{Email: user.Email, Password: password}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "Device-ID header is required")
	})

	t.Run("error validating MFA device", func(t *testing.T) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		authenticateSetup(t, authenticatorMock, jwtManagerMock, user, password, userToken)
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, user.ID).
			Return(false, errors.New("weird error happened")).
			Once()

		body := LoginRequest{Email: user.Email, Password: password}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "An internal error occurred while processing this request")
	})

	t.Run("when device is remembered, return token", func(t *testing.T) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		authenticateSetup(t, authenticatorMock, jwtManagerMock, user, password, userToken)
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, user.ID).
			Return(true, nil).
			Once()

		body := LoginRequest{Email: user.Email, Password: password}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
		require.JSONEq(t, fmt.Sprintf(`{"token": "%s"}`, userToken), rw.Body.String())
	})

	t.Run("error generating MFA code", func(t *testing.T) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		authenticateSetup(t, authenticatorMock, jwtManagerMock, user, password, userToken)
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, user.ID).
			Return(false, nil).
			Once()
		mfaManagerMock.
			On("GenerateMFACode", mock.Anything, deviceID, user.ID).
			Return("", errors.New("some weird error")).
			Once()

		body := LoginRequest{Email: user.Email, Password: password}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot get MFA code")
	})

	t.Run("error when code returned is empty", func(t *testing.T) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		authenticateSetup(t, authenticatorMock, jwtManagerMock, user, password, userToken)
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, user.ID).
			Return(false, nil).
			Once()
		mfaManagerMock.
			On("GenerateMFACode", mock.Anything, deviceID, user.ID).
			Return("", nil).
			Once()

		body := LoginRequest{Email: user.Email, Password: password}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot get MFA code")
	})

	t.Run("error sending MFA message", func(t *testing.T) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		authenticateSetup(t, authenticatorMock, jwtManagerMock, user, password, userToken)
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, user.ID).
			Return(false, nil).
			Once()
		mfaManagerMock.
			On("GenerateMFACode", mock.Anything, deviceID, user.ID).
			Return(mfaCode, nil).
			Once()
		messengerClientMock.
			On("SendMessage", mock.Anything).
			Return(errors.New("weird error sending message")).
			Once()

		body := LoginRequest{Email: user.Email, Password: password}
		req := httptest.NewRequest(http.MethodPost, "/login", requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		loginHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "An internal error occurred while processing this request")
	})

	t.Run("🎉  Successful login", func(t *testing.T) {
		authenticatorMock.
			On("GetUserByEmail", mock.Anything, user.Email).
			Return(user, nil).
			Once()
		authenticateSetup(t, authenticatorMock, jwtManagerMock, user, password, userToken)
		mfaManagerMock.
			On("MFADeviceRemembered", mock.Anything, deviceID, user.ID).
			Return(false, nil).
			Once()
		mfaManagerMock.
			On("GenerateMFACode", mock.Anything, deviceID, user.ID).
			Return(mfaCode, nil).
			Once()

		content, err := htmltemplate.ExecuteHTMLTemplateForMFAMessage(htmltemplate.MFAMessageTemplate{
			OrganizationName: "MyCustomAid",
			MFACode:          mfaCode,
		})
		require.NoError(t, err)

		msg := message.Message{
			ToEmail: user.Email,
			Title:   mfaMessageTitle,
			Message: content,
		}
		messengerClientMock.
			On("SendMessage", msg).
			Return(nil).
			Once()

		body := LoginRequest{Email: user.Email, Password: password}
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
