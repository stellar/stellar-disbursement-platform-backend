package httphandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
		ReCAPTCHAEnabled:   true,
	}

	deviceID := "safari-xyz"

	t.Run("Test handler with invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, nil)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
	})

	t.Run("Test handler with unexpected reCAPTCHA error", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(false, errors.New("unexpected error")).
			Once()

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token"}
		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot validate reCAPTCHA token")
	})

	t.Run("Test handler with invalid reCAPTCHA token", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(false, nil).
			Once()

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token"}
		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "reCAPTCHA token invalid")
	})

	t.Run("Test Device ID header is empty", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token"}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "Device-ID header is required")
	})

	t.Run("Test MFA code is empty", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		body := MFARequest{ReCAPTCHAToken: "token"}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusBadRequest, rw.Code)
		require.Contains(t, rw.Body.String(), "MFA Code is required")
	})

	t.Run("Test MFA code is invalid", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("", auth.ErrMFACodeInvalid).
			Once()

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token"}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusUnauthorized, rw.Code)
		require.Contains(t, rw.Body.String(), "Not authorized.")
	})

	t.Run("Test MFA validation failed", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("", errors.New("weird error happened")).
			Once()

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token"}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot authenticate user")
	})

	t.Run("Test MFA remember me failed", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("userID", nil).
			Once()

		mfaManagerMock.
			On("RememberDevice", mock.Anything, deviceID, "123456").
			Return(errors.New("weird error happened")).
			Once()

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token", RememberMe: true}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot authenticate user")
	})

	t.Run("Test MFA get user failed", func(t *testing.T) {
		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

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

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token", RememberMe: true}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusInternalServerError, rw.Code)
		require.Contains(t, rw.Body.String(), "Cannot authenticate user")
	})

	t.Run("Test MFA validation successful", func(t *testing.T) {
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.SetLevel(log.InfoLevel)

		reCAPTCHAValidatorMock.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		mfaManagerMock.
			On("ValidateMFACode", mock.Anything, deviceID, "123456").
			Return("userID", nil).
			Once()

		mfaManagerMock.
			On("RememberDevice", mock.Anything, deviceID, "123456").
			Return(nil).
			Once()

		user := &auth.User{
			ID:    "user-id",
			Email: "email@email.com",
		}

		authenticatorMock.
			On("GetUser", mock.Anything, "userID").
			Return(user, nil).
			Once()

		roleManagerMock.
			On("GetUserRoles", mock.Anything, user).
			Return([]string{"role1"}, nil).
			Once()

		jwtManagerMock.
			On("GenerateToken", mock.Anything, user, mock.AnythingOfType("time.Time")).
			Return("token123", nil).
			On("ValidateToken", mock.Anything, "token123").
			Return(true, nil).
			On("GetUserFromToken", mock.Anything, "token123").
			Return(user, nil).
			Once()

		body := MFARequest{MFACode: "123456", ReCAPTCHAToken: "token", RememberMe: true}

		req := httptest.NewRequest(http.MethodPost, mfaEndpoint, requestToJSON(t, &body))
		req.Header.Set(DeviceIDHeader, deviceID)
		rw := httptest.NewRecorder()

		mfaHandler.ServeHTTP(rw, req)

		require.Equal(t, http.StatusOK, rw.Code)
		require.JSONEq(t, `{"token": "token123"}`, rw.Body.String())

		// validate logs
		require.Contains(t, buf.String(), "[UserLogin] - Logged in user with account ID user-id")
	})

	authenticatorMock.AssertExpectations(t)
	reCAPTCHAValidatorMock.AssertExpectations(t)
}

func requestToJSON(t *testing.T, req interface{}) io.Reader {
	body, err := json.Marshal(req)
	require.NoError(t, err)
	return bytes.NewReader(body)
}
