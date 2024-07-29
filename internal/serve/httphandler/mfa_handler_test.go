package httphandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

const mfaEndpoint = "/mfa"

func Test_MFAHandler_ServeHTTP(t *testing.T) {
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
	reCAPTCHAValidatorMock := &validators.ReCAPTCHAValidatorMock{}
	mfaManagerMock := &auth.MFAManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomAuthenticatorOption(authenticatorMock),
		auth.WithCustomJWTManagerOption(jwtManagerMock),
		auth.WithCustomRoleManagerOption(roleManagerMock),
		auth.WithCustomMFAManagerOption(mfaManagerMock),
	)

	mfaHandler := MFAHandler{
		AuthManager:        authManager,
		ReCAPTCHAValidator: reCAPTCHAValidatorMock,
		Models:             models,
		ReCAPTCHADisabled:  false,
	}

	userToken := "token123"
	user := &auth.User{
		ID:    "userID",
		Email: "email@email.com",
	}

	deviceID := "safari-xyz"
	mfaCode := "123456"
	validateMFASetup := func(deviceID, mfaCode string, rememberMe bool) {
		userRoleLookupSetup(jwtManagerMock, authenticatorMock, roleManagerMock, user, []string{data.OwnerUserRole.String()}, userToken)
		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, mfaCode).
			Return(user.ID, nil).
			Once()
		if rememberMe {
			mfaManagerMock.
				On("RememberDevice", mock.Anything, deviceID, mfaCode).
				Return(nil).
				Once()
		}
		jwtManagerMock.
			On("GenerateToken", mock.Anything, user, mock.AnythingOfType("time.Time")).
			Return(userToken, nil).
			On("GetUserFromToken", mock.Anything, userToken).
			Return(user, nil).
			Once()
	}

	reCAPTCHAToken := "token"
	t.Run("Test handler with invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, nil)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
	})

	t.Run("Test Device ID header is empty", func(t *testing.T) {
		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken}
		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "Device-ID header is required")
	})

	t.Run("Test MFA code is empty", func(t *testing.T) {
		body := MFARequest{ReCAPTCHAToken: reCAPTCHAToken}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "MFA Code is required")
	})

	t.Run("Test MFA code is invalid", func(t *testing.T) {
		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken}
		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("", auth.ErrMFACodeInvalid).
			Once()

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusUnauthorized, rw.Code)
		require.Contains(t, rw.Body.String(), "Not authorized.")
	})

	t.Run("Test MFA validation failed", func(t *testing.T) {
		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken}
		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("", errors.New("weird error happened")).
			Once()

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot authenticate user")
	})

	t.Run("Test MFA remember me failed", func(t *testing.T) {
		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken, RememberMe: true}
		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("userID", nil).
			Once()

		mfaManagerMock.
			On("RememberDevice", mock.Anything, deviceID, "123456").
			Return(errors.New("weird error happened")).
			Once()

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot authenticate user")
	})

	t.Run("Test MFA get user failed", func(t *testing.T) {
		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken, RememberMe: true}
		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("userID", nil).
			Once()

		mfaManagerMock.
			On("RememberDevice", mock.Anything, deviceID, "123456").
			Return(nil).
			Once()

		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(nil, errors.New("weird error happened")).
			Once()

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot authenticate user")
	})

	t.Run("Test handler with unexpected reCAPTCHA error", func(t *testing.T) {
		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken}
		validateMFASetup(deviceID, body.MFACode, body.RememberMe)
		userRoleLookupSetup(jwtManagerMock, authenticatorMock, roleManagerMock, user, []string{data.OwnerUserRole.String()}, userToken)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, reCAPTCHAToken).
			Return(false, errors.New("unexpected error")).
			Once()

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot validate reCAPTCHA token")
	})

	t.Run("Test handler with invalid reCAPTCHA token", func(t *testing.T) {
		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken}
		validateMFASetup(deviceID, body.MFACode, body.RememberMe)
		userRoleLookupSetup(jwtManagerMock, authenticatorMock, roleManagerMock, user, []string{data.OwnerUserRole.String()}, userToken)
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, reCAPTCHAToken).
			Return(false, nil).
			Once()

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "reCAPTCHA token invalid")
	})

	t.Run("Test handler without reCAPTCHA token when user role can bypass check", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		body := MFARequest{MFACode: mfaCode, ReCAPTCHAToken: reCAPTCHAToken, RememberMe: true}
		validateMFASetup(deviceID, mfaCode, body.RememberMe)
		userRoleLookupSetup(jwtManagerMock, authenticatorMock, roleManagerMock, user, []string{data.APIUserRole.String()}, userToken)

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
		require.JSONEq(t, `{"token": "token123"}`, rw.Body.String())

		// validate logs
		require.Contains(t, buf.String(), "[UserLogin] - Logged in user with account ID userID")
	})

	t.Run("Test MFA validation successful", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		usersRoles := make([][]string, 2)
		// role cannot bypass reCAPTCHA
		usersRoles[0] = []string{data.OwnerUserRole.String()}
		// role can bypass reCAPTCHA
		usersRoles[1] = []string{data.OwnerUserRole.String(), data.APIUserRole.String()}
		for _, userRoles := range usersRoles {
			body := MFARequest{MFACode: mfaCode, RememberMe: true}
			validateMFASetup(deviceID, mfaCode, body.RememberMe)
			userRoleLookupSetup(jwtManagerMock, authenticatorMock, roleManagerMock, user, userRoles, userToken)
			if !slices.Contains(userRoles, data.APIUserRole.String()) {
				body.ReCAPTCHAToken = reCAPTCHAToken
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, reCAPTCHAToken).
					Return(true, nil).
					Once()
			}

			req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
			req.Header.Set(DeviceIDHeader, deviceID)
			rw := httptest.NewRecorder()

			mfaHandler.ServeHTTP(rw, req)

			require.Equal(t, http.StatusOK, rw.Code)
			require.JSONEq(t, `{"token": "token123"}`, rw.Body.String())

			// validate logs
			require.Contains(t, buf.String(), "[UserLogin] - Logged in user with account ID userID")
		}
	})

	authenticatorMock.AssertExpectations(t)
	reCAPTCHAValidatorMock.AssertExpectations(t)
}

func requestToJSON(t *testing.T, req interface{}) io.Reader {
	body, err := json.Marshal(req)
	require.NoError(t, err)
	return bytes.NewReader(body)
}
