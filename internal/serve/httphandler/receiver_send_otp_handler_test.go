package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockMessengerClient struct {
	mock.Mock
}

func (m *mockMessengerClient) SendMessage(message message.Message) error {
	return m.Called(message).Error(0)
}

func (mc *mockMessengerClient) MessengerType() message.MessengerType {
	args := mc.Called()
	return args.Get(0).(message.MessengerType)
}

func Test_ReceiverSendOTPHandler_ServeHTTP(t *testing.T) {
	r := chi.NewRouter()

	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	wallet1 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://home.page", "home.page", "wallet123://")

	_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.RegisteredReceiversWalletStatus)
	_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet1.ID, data.RegisteredReceiversWalletStatus)

	mockMessenger := mockMessengerClient{}
	reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}

	r.Post("/wallet-registration/otp", ReceiverSendOTPHandler{
		Models:             models,
		SMSMessengerClient: &mockMessenger,
		ReCAPTCHAValidator: reCAPTCHAValidator,
	}.ServeHTTP)

	requestSendOTP := ReceiverSendOTPRequest{
		PhoneNumber:    receiver1.PhoneNumber,
		ReCAPTCHAToken: "XyZ",
	}
	reqBody, err := json.Marshal(requestSendOTP)
	require.NoError(t, err)

	t.Run("returns 401 - Unauthorized if the token is not in the request context", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()
		req, err := http.NewRequest("POST", "/wallet-registration/otp", strings.NewReader(string(reqBody)))
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns 401 - Unauthorized if the token is in the request context but it's not valid", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()
		req, err := http.NewRequest("POST", "/wallet-registration/otp", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		invalidClaims := &anchorplatform.SEP24JWTClaims{}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, invalidClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Not authorized."}`, string(respBody))
	})

	t.Run("returns 400 - BadRequest with a wrong request body", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()
		invalidRequest := `{"recaptcha_token": "XyZ"}`

		req, err := http.NewRequest("POST", "/wallet-registration/otp", strings.NewReader(invalidRequest))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		invalidClaims := &anchorplatform.SEP24JWTClaims{}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, invalidClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error":"request invalid","extras":{"phone_number":"phone_number is required"}}`, string(respBody))
	})

	t.Run("returns 200 - Ok if the token is in the request context and body it's valid", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()
		req, err := http.NewRequest("POST", "/wallet-registration/otp", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet1.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		mockMessenger.On("SendMessage", mock.AnythingOfType("message.Message")).
			Return(nil).
			Once()

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "/json; charset=utf-8")
		assert.JSONEq(t, string(respBody), `{"message":"if your phone number is registered, you'll receive an OTP"}`)
	})

	t.Run("returns 500 - InternalServerError when something goes wrong when sending the SMS", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
			Once()
		req, err := http.NewRequest("POST", "/wallet-registration/otp", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet1.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		mockMessenger.On("SendMessage", mock.AnythingOfType("message.Message")).
			Return(errors.New("error sending message")).
			Once()

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "/json; charset=utf-8")
		assert.JSONEq(t, string(respBody), `{"error":"Cannot send OTP message"}`)
	})

	t.Run("returns 500 - InternalServerError when unable to validate recaptcha", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(false, errors.New("error requesting verify reCAPTCHA token")).
			Once()

		req, err := http.NewRequest(http.MethodPost, "/wallet-registration/otp", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet1.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

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
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	t.Run("returns 400 - BadRequest when recaptcha token is invalid", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(false, nil).
			Once()

		req, err := http.NewRequest(http.MethodPost, "/wallet-registration/otp", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet1.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "reCAPTCHA token is invalid"
			}
		`
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, wantsBody, string(respBody))
	})

	mockMessenger.AssertExpectations(t)
	reCAPTCHAValidator.AssertExpectations(t)
}
