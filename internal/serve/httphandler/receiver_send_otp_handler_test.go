package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

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
		VerificationField: data.VerificationTypeDateOfBirth,
	})

	_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet1.ID, data.RegisteredReceiversWalletStatus)
	_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet1.ID, data.RegisteredReceiversWalletStatus)

	mockMessageDispatcher := message.NewMockMessageDispatcher(t)
	reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}

	r.Post("/wallet-registration/otp", ReceiverSendOTPHandler{
		Models:             models,
		MessageDispatcher:  mockMessageDispatcher,
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

		mockMessageDispatcher.
			On("SendMessage",
				mock.Anything,
				mock.AnythingOfType("message.Message"),
				[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(nil).
			Once().
			Run(func(args mock.Arguments) {
				msg := args.Get(1).(message.Message)
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

		mockMessageDispatcher.
			On("SendMessage",
				mock.Anything,
				mock.AnythingOfType("message.Message"),
				[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
			Return(nil).
			Once().
			Run(func(args mock.Arguments) {
				msg := args.Get(1).(message.Message)
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

		mockMessageDispatcher.
			On("SendMessage",
				mock.Anything,
				mock.AnythingOfType("message.Message"),
				[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
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

	t.Run("returns 200 (DoB) - InternalServerError if phone number is not associated with receiver verification", func(t *testing.T) {
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

		wantsBody := `{
			"message":"if your phone number is registered, you'll receive an OTP",
			"verification_field":"DATE_OF_BIRTH"
		}`
		assert.Equal(t, http.StatusOK, resp.StatusCode)
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

	mockMessageDispatcher.AssertExpectations(t)
	reCAPTCHAValidator.AssertExpectations(t)
}

func Test_newReceiverSendOTPResponseBody(t *testing.T) {
	for _, otpType := range data.GetAllReceiverContactTypes() {
		for _, verificationType := range data.GetAllVerificationTypes() {
			t.Run(fmt.Sprintf("%s/%s", otpType, verificationType), func(t *testing.T) {
				gotBody := newReceiverSendOTPResponseBody(otpType, verificationType)
				wantBody := ReceiverSendOTPResponseBody{
					Message:           fmt.Sprintf("if your %s is registered, you'll receive an OTP", utils.HumanizeString(string(otpType))),
					VerificationField: verificationType,
				}
				require.Equal(t, wantBody, gotBody)
			})
		}
	}
}

func Test_ReceiverSendOTPHandler_sendOTP(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	organization, err := models.Organizations.Get(ctx)
	require.NoError(t, err)
	defaultOTPMessageTemplate := organization.OTPMessageTemplate

	phoneNumber := "+380443973607"
	email := "foobar@test.com"
	otp := "246810"

	testCases := []struct {
		name                   string
		overrideOrgOTPTemplate string
		wantMessage            string
		shouldDispatcherFail   bool
	}{
		{
			name:                   "dispacher fails",
			overrideOrgOTPTemplate: defaultOTPMessageTemplate,
			wantMessage:            fmt.Sprintf("246810 is your %s phone verification code. If you did not request this code, please ignore. Do not share your code with anyone.", organization.Name),
		},
		{
			name:                   "🎉 successful with default message",
			overrideOrgOTPTemplate: defaultOTPMessageTemplate,
			wantMessage:            fmt.Sprintf("246810 is your %s phone verification code. If you did not request this code, please ignore. Do not share your code with anyone.", organization.Name),
		},
		{
			name:                   "🎉 successful with custom message and pre-existing OTP tag",
			overrideOrgOTPTemplate: "Here's your code: {{.OTP}}.",
			wantMessage:            "Here's your code: 246810. If you did not request this code, please ignore. Do not share your code with anyone.",
		},
		{
			name:                   "🎉 successful with custom message and NO pre-existing OTP tag",
			overrideOrgOTPTemplate: "is your one-time password.",
			wantMessage:            "246810 is your one-time password. If you did not request this code, please ignore. Do not share your code with anyone.",
		},
	}

	for _, contactType := range data.GetAllReceiverContactTypes() {
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", contactType, tc.name), func(t *testing.T) {
				var expectedMsg message.Message
				var contactInfo string
				switch contactType {
				case data.ReceiverContactTypeSMS:
					expectedMsg = message.Message{ToPhoneNumber: phoneNumber, Message: tc.wantMessage}
					contactInfo = phoneNumber
				case data.ReceiverContactTypeEmail:
					expectedMsg = message.Message{ToEmail: email, Message: tc.wantMessage, Title: "Your One-Time Password: " + otp}
					contactInfo = email
				}

				mockMessageDispatcher := message.NewMockMessageDispatcher(t)
				mockCall := mockMessageDispatcher.
					On("SendMessage",
						mock.Anything,
						expectedMsg,
						[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail})
				if !tc.shouldDispatcherFail {
					mockCall.Return(nil).Once()
				} else {
					mockCall.Return(errors.New("error sending message")).Once()
				}

				handler := ReceiverSendOTPHandler{
					Models:            models,
					MessageDispatcher: mockMessageDispatcher,
				}

				err = models.Organizations.Update(ctx, &data.OrganizationUpdate{
					OTPMessageTemplate: &tc.overrideOrgOTPTemplate,
				})
				require.NoError(t, err)

				err := handler.sendOTP(ctx, contactType, contactInfo, otp)
				require.NoError(t, err)
			})
		}
	}
}

func Test_ReceiverSendOTPHandler_handleOTPForReceiver(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://correct.test", "correct.test", "wallet123://")
	receiverWithoutWalletInsert := &data.Receiver{
		PhoneNumber: "+141555550000",
		Email:       "without_wallet@test.com",
	}

	testCases := []struct {
		name                  string
		contactInfo           func(r data.Receiver, contactType data.ReceiverContactType) string
		dateOfBirth           string
		sep24ClientDomain     string
		prepareMocksFn        func(t *testing.T, mockMessageDispatcher *message.MockMessageDispatcher)
		assertLogsFn          func(t *testing.T, contactType data.ReceiverContactType, r data.Receiver, entries []logrus.Entry)
		wantVerificationField data.VerificationType
		wantHttpErr           func(contactType data.ReceiverContactType) *httperror.HTTPError
	}{
		{
			name: "🟡 false positive if GetLatestByContactInfo returns no results",
			contactInfo: func(r data.Receiver, contactType data.ReceiverContactType) string {
				return "not_found"
			},
			assertLogsFn: func(t *testing.T, contactType data.ReceiverContactType, r data.Receiver, entries []logrus.Entry) {
				contactTypeStr := utils.HumanizeString(string(contactType))
				truncatedContactInfo := utils.TruncateString("not_found", 3)
				wantLog := fmt.Sprintf("cannot find ANY receiver verification for %s %s: %v", contactTypeStr, truncatedContactInfo, data.ErrRecordNotFound)
				assert.Contains(t, entries[0].Message, wantLog)
			},
			wantVerificationField: data.VerificationTypeDateOfBirth,
		},
		{
			name: "🟡 false positive if UpdateOTPByReceiverContactInfoAndWalletDomain doesn't find a {<contactInfo>,client_domain} match (client_domain)",
			contactInfo: func(r data.Receiver, contactType data.ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			sep24ClientDomain: "incorrect.test",
			assertLogsFn: func(t *testing.T, contactType data.ReceiverContactType, r data.Receiver, entries []logrus.Entry) {
				contactTypeStr := utils.HumanizeString(string(contactType))
				truncatedContactInfo := utils.TruncateString(r.ContactByType(contactType), 3)
				wantLog := fmt.Sprintf("could not find a match between %s (%s) and client domain (%s)", contactTypeStr, truncatedContactInfo, "incorrect.test")
				assert.Contains(t, entries[0].Message, wantLog)
			},
			wantVerificationField: data.VerificationTypeDateOfBirth,
		},
		{
			name: "🟡 false positive if UpdateOTPByReceiverContactInfoAndWalletDomain doesn't find a {<contactInfo>,client_domain} match (<contactInfo>)",
			contactInfo: func(_ data.Receiver, contactType data.ReceiverContactType) string {
				return receiverWithoutWalletInsert.ContactByType(contactType)
			},
			sep24ClientDomain: "correct.test",
			assertLogsFn: func(t *testing.T, contactType data.ReceiverContactType, _ data.Receiver, entries []logrus.Entry) {
				contactTypeStr := utils.HumanizeString(string(contactType))
				truncatedContactInfo := utils.TruncateString(receiverWithoutWalletInsert.ContactByType(contactType), 3)
				wantLog := fmt.Sprintf("could not find a match between %s (%s) and client domain (%s)", contactTypeStr, truncatedContactInfo, "correct.test")
				assert.Contains(t, entries[0].Message, wantLog)
			},
			wantVerificationField: data.VerificationTypeDateOfBirth,
		},
		{
			name: "🔴 error if sendOTP fails",
			contactInfo: func(r data.Receiver, contactType data.ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			sep24ClientDomain: "correct.test",
			prepareMocksFn: func(t *testing.T, mockMessageDispatcher *message.MockMessageDispatcher) {
				mockMessageDispatcher.
					On("SendMessage",
						mock.Anything,
						mock.AnythingOfType("message.Message"),
						[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
					Return(errors.New("error sending message")).
					Once()
			},
			wantVerificationField: data.VerificationTypeDateOfBirth,
			wantHttpErr: func(contactType data.ReceiverContactType) *httperror.HTTPError {
				contactTypeStr := utils.HumanizeString(string(contactType))
				err := fmt.Errorf("sending OTP message: %w", fmt.Errorf("cannot send OTP message through %s: %w", contactTypeStr, errors.New("error sending message")))
				return httperror.InternalError(ctx, "Failed to send OTP message, reason: "+err.Error(), err, nil)
			},
		},
		{
			name: "🟢 successful",
			contactInfo: func(r data.Receiver, contactType data.ReceiverContactType) string {
				return r.ContactByType(contactType)
			},
			sep24ClientDomain: "correct.test",
			prepareMocksFn: func(t *testing.T, mockMessageDispatcher *message.MockMessageDispatcher) {
				mockMessageDispatcher.
					On("SendMessage",
						mock.Anything,
						mock.AnythingOfType("message.Message"),
						[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
					Return(nil).
					Once()
			},
			wantVerificationField: data.VerificationTypePin,
			wantHttpErr:           nil,
		},
	}

	for _, contactType := range data.GetAllReceiverContactTypes() {
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", contactType, tc.name), func(t *testing.T) {
				receiverWithWalletInsert := &data.Receiver{}
				switch contactType {
				case data.ReceiverContactTypeSMS:
					receiverWithWalletInsert.PhoneNumber = "+141555551111"
				case data.ReceiverContactTypeEmail:
					receiverWithWalletInsert.Email = "with_wallet@test.com"
				}

				defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)

				handler := ReceiverSendOTPHandler{Models: models}
				if tc.prepareMocksFn != nil {
					mockMessageDispatcher := message.NewMockMessageDispatcher(t)
					tc.prepareMocksFn(t, mockMessageDispatcher)
					handler.MessageDispatcher = mockMessageDispatcher
				}

				// Setup receiver with Verification but without wallet:
				receiverWithoutWallet := data.CreateReceiverFixture(t, ctx, dbConnectionPool, receiverWithoutWalletInsert)
				_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
					ReceiverID:        receiverWithoutWallet.ID,
					VerificationField: data.VerificationTypePin,
					VerificationValue: "123456",
				})

				// Setup receiver with Verification AND wallet:
				receiverWithWallet := data.CreateReceiverFixture(t, ctx, dbConnectionPool, receiverWithWalletInsert)
				_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
					ReceiverID:        receiverWithWallet.ID,
					VerificationField: data.VerificationTypePin,
					VerificationValue: "123456",
				})
				_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverWithWallet.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

				getEntries := log.DefaultLogger.StartTest(logrus.DebugLevel)

				contactInfo := tc.contactInfo(*receiverWithWallet, contactType)
				verificationField, httpErr := handler.handleOTPForReceiver(ctx, contactType, contactInfo, tc.sep24ClientDomain)
				if tc.wantHttpErr != nil {
					wantHTTPErr := tc.wantHttpErr(contactType)
					require.NotNil(t, httpErr)
					assert.Equal(t, *wantHTTPErr, *httpErr)
					assert.Equal(t, tc.wantVerificationField, verificationField)
				} else {
					require.Nil(t, httpErr)
					assert.Equal(t, tc.wantVerificationField, verificationField)
				}

				entries := getEntries()
				if tc.assertLogsFn != nil {
					tc.assertLogsFn(t, contactType, *receiverWithWallet, entries)
				}
			})
		}
	}
}
