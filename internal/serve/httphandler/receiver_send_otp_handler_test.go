package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
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

	phoneNumber := "+380443973607"
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	wallet1 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://home.page", "home.page", "wallet123://")
	data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
		ReceiverID:        receiver1.ID,
		VerificationField: data.VerificationFieldDateOfBirth,
	})

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
		req, err := http.NewRequest(http.MethodPost, "/wallet-registration/otp", strings.NewReader(string(reqBody)))
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
		req, err := http.NewRequest(http.MethodPost, "/wallet-registration/otp", strings.NewReader(string(reqBody)))
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
			Twice()
		invalidRequest := `{"recaptcha_token": "XyZ"}`

		req, err := http.NewRequest(http.MethodPost, "/wallet-registration/otp", strings.NewReader(invalidRequest))
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		invalidClaims := &anchorplatform.SEP24JWTClaims{}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, invalidClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error":"request invalid","extras":{"phone_number":"phone_number is required"}}`, string(respBody))

		req, err = http.NewRequest(http.MethodPost, "/wallet-registration/otp", strings.NewReader(`{"phone_number": "+55555555555", "recaptcha_token": "XyZ"}`))
		require.NoError(t, err)

		rr = httptest.NewRecorder()
		invalidClaims = &anchorplatform.SEP24JWTClaims{}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, invalidClaims))
		r.ServeHTTP(rr, req)

		resp = rr.Result()

		respBody, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "request invalid", "extras": {"phone_number": "invalid phone number provided"}}`, string(respBody))
	})

	t.Run("returns 200 - Ok if the token is in the request context and body is valid", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
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

		mockMessenger.On("SendMessage", mock.AnythingOfType("message.Message")).
			Return(nil).
			Once().
			Run(func(args mock.Arguments) {
				msg := args.Get(0).(message.Message)
				assert.Contains(t, msg.Message, "is your MyCustomAid phone verification code.")
				assert.Regexp(t, regexp.MustCompile(`^\d{6}\s.+$`), msg.Message)
			})

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "/json; charset=utf-8")
		assert.JSONEq(t, string(respBody), `{"message":"if your phone number is registered, you'll receive an OTP", "verification_field":"DATE_OF_BIRTH"}`)
	})

	t.Run("returns 200 - parses a custom OTP message template successfully", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
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

		// Set a custom message for the OTP message
		customOTPMessage := "Here's your code to complete your registration. MyOrg 👋"
		err = models.Organizations.Update(ctx, &data.OrganizationUpdate{OTPMessageTemplate: &customOTPMessage})
		require.NoError(t, err)

		mockMessenger.On("SendMessage", mock.AnythingOfType("message.Message")).
			Return(nil).
			Once().
			Run(func(args mock.Arguments) {
				msg := args.Get(0).(message.Message)
				assert.Contains(t, msg.Message, customOTPMessage)
				assert.Regexp(t, regexp.MustCompile(`^\d{6}\s.+$`), msg.Message)
			})

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "/json; charset=utf-8")
		assert.JSONEq(t, string(respBody), `{"message":"if your phone number is registered, you'll receive an OTP", "verification_field":"DATE_OF_BIRTH"}`)
	})

	t.Run("returns 500 - InternalServerError when something goes wrong when sending the SMS", func(t *testing.T) {
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
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

	t.Run("returns 500 - InternalServerError if phone number is not associated with receiver verification", func(t *testing.T) {
		requestSendOTP := ReceiverSendOTPRequest{
			PhoneNumber:    "+14152223333",
			ReCAPTCHAToken: "XyZ",
		}
		reqBody, _ = json.Marshal(requestSendOTP)

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "XyZ").
			Return(true, nil).
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

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		wantsBody := `
			{
				"error": "Cannot find latest receiver verification for receiver"
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
