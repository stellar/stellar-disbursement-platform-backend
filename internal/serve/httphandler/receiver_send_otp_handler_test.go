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

func Test_ReceiverSendOTPRequest_validateContactInfo(t *testing.T) {
	testCases := []struct {
		name                   string
		receiverSendOTPRequest ReceiverSendOTPRequest
		wantValidationErrors   map[string]interface{}
	}{
		{
			name: "🔴 phone number and email both empty",
			receiverSendOTPRequest: ReceiverSendOTPRequest{
				PhoneNumber: "",
				Email:       "",
			},
			wantValidationErrors: map[string]interface{}{
				"phone_number": "phone_number or email is required",
				"email":        "phone_number or email is required",
			},
		},
		{
			name: "🔴 phone number and email both provided",
			receiverSendOTPRequest: ReceiverSendOTPRequest{
				PhoneNumber: "+141555550000",
				Email:       "foobar@test.com",
			},
			wantValidationErrors: map[string]interface{}{
				"phone_number": "phone_number and email cannot be both provided",
				"email":        "phone_number and email cannot be both provided",
			},
		},
		{
			name: "🔴 phone number is invalid",
			receiverSendOTPRequest: ReceiverSendOTPRequest{
				PhoneNumber: "invalid",
			},
			wantValidationErrors: map[string]interface{}{
				"phone_number": "the provided phone number is not a valid E.164 number",
			},
		},
		{
			name: "🔴 email is invalid",
			receiverSendOTPRequest: ReceiverSendOTPRequest{
				Email: "invalid",
			},
			wantValidationErrors: map[string]interface{}{
				"email": "the provided email is not valid",
			},
		},
		{
			name: "🟢 phone number is valid",
			receiverSendOTPRequest: ReceiverSendOTPRequest{
				PhoneNumber: "+14155550000",
			},
			wantValidationErrors: nil,
		},
		{
			name: "🟢 email is valid",
			receiverSendOTPRequest: ReceiverSendOTPRequest{
				Email: "foobar@test.com",
			},
			wantValidationErrors: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.receiverSendOTPRequest.validateContactInfo()
			if len(tc.wantValidationErrors) == 0 {
				assert.Len(t, v.Errors, 0)
			} else {
				assert.Equal(t, tc.wantValidationErrors, v.Errors)
			}
		})
	}
}

func Test_ReceiverSendOTPHandler_ServeHTTP_validation(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	validClaims := &anchorplatform.SEP24JWTClaims{
		ClientDomainClaim: "no-op-domain.test.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}
	ctxWithValidSEP24Claims := context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, validClaims)
	invalidClaims := &anchorplatform.SEP24JWTClaims{}
	ctxWithInvalidSEP24Claims := context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, invalidClaims)

	const reCAPTCHAToken = "XyZ"

	testCases := []struct {
		name                   string
		context                context.Context
		receiverSendOTPRequest ReceiverSendOTPRequest
		prepareMocksFn         func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, mockMessageDispatcher *message.MockMessageDispatcher)
		wantStatusCode         int
		wantBody               string
	}{
		{
			name:                   "(500 - InternalServerError) if the reCAPTCHA validation returns an error",
			context:                ctx,
			receiverSendOTPRequest: ReceiverSendOTPRequest{ReCAPTCHAToken: "invalid-recaptcha-token"},
			prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, _ *message.MockMessageDispatcher) {
				mockReCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, "invalid-recaptcha-token").
					Return(false, errors.New("invalid recaptcha")).
					Once()
			},
			wantStatusCode: http.StatusInternalServerError,
			wantBody:       `{"error":"Cannot validate reCAPTCHA token"}`,
		},
		{
			name:                   "(400 - BadRequest) if the reCAPTCHA token is invalid",
			context:                ctx,
			receiverSendOTPRequest: ReceiverSendOTPRequest{ReCAPTCHAToken: reCAPTCHAToken},
			prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, _ *message.MockMessageDispatcher) {
				mockReCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, reCAPTCHAToken).
					Return(false, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantBody:       `{"error":"reCAPTCHA token is invalid"}`,
		},
		{
			name:                   "(401 - Unauthorized) if the SEP-24 claims are not in the request context",
			context:                ctx,
			receiverSendOTPRequest: ReceiverSendOTPRequest{ReCAPTCHAToken: reCAPTCHAToken},
			prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, _ *message.MockMessageDispatcher) {
				mockReCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, reCAPTCHAToken).
					Return(true, nil).
					Once()
			},
			wantStatusCode: http.StatusUnauthorized,
			wantBody:       `{"error":"Not authorized."}`,
		},
		{
			name:                   "(401 - Unauthorized) if the SEP-24 claims are invalid",
			context:                ctxWithInvalidSEP24Claims,
			receiverSendOTPRequest: ReceiverSendOTPRequest{ReCAPTCHAToken: reCAPTCHAToken},
			prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, _ *message.MockMessageDispatcher) {
				mockReCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, reCAPTCHAToken).
					Return(true, nil).
					Once()
			},
			wantStatusCode: http.StatusUnauthorized,
			wantBody:       `{"error":"Not authorized."}`,
		},
		{
			name:                   "(400 - BadRequest) if the request body is invalid",
			context:                ctxWithValidSEP24Claims,
			receiverSendOTPRequest: ReceiverSendOTPRequest{ReCAPTCHAToken: reCAPTCHAToken},
			prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, _ *message.MockMessageDispatcher) {
				mockReCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, reCAPTCHAToken).
					Return(true, nil).
					Once()
			},
			wantStatusCode: http.StatusBadRequest,
			wantBody: `{
				"error": "The request was invalid in some way.",
				"extras": {
					"phone_number":"phone_number or email is required",
					"email":"phone_number or email is required"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockReCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
			mockMessageDispatcher := message.NewMockMessageDispatcher(t)

			tc.prepareMocksFn(t, mockReCAPTCHAValidator, mockMessageDispatcher)

			r := chi.NewRouter()
			r.Post("/wallet-registration/otp", ReceiverSendOTPHandler{
				Models:             models,
				MessageDispatcher:  mockMessageDispatcher,
				ReCAPTCHAValidator: mockReCAPTCHAValidator,
			}.ServeHTTP)

			reqBody, err := json.Marshal(tc.receiverSendOTPRequest)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(tc.context, http.MethodPost, "/wallet-registration/otp", strings.NewReader(string(reqBody)))
			require.NoError(t, err)
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.wantStatusCode, resp.StatusCode)
			assert.JSONEq(t, tc.wantBody, string(respBody))
		})
	}
}

func Test_ReceiverSendOTPHandler_ServeHTTP_otpHandlerIsCalled(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	const phoneNumber = "+14155550000"
	const email = "foobar@test.com"
	require.NoError(t, err)
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://correct.test", "correct.test", "wallet123://")

	validClaims := &anchorplatform.SEP24JWTClaims{
		ClientDomainClaim: wallet.SEP10ClientDomain,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}
	ctxWithValidSEP24Claims := context.WithValue(ctx, anchorplatform.SEP24ClaimsContextKey, validClaims)

	const reCAPTCHAToken = "XyZ"

	type testCase struct {
		name                   string
		receiverSendOTPRequest ReceiverSendOTPRequest
		verificationField      data.VerificationType
		contactType            data.ReceiverContactType
		prepareMocksFn         func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, mockMessageDispatcher *message.MockMessageDispatcher)
		shouldCreateObjects    bool
		assertLogsFn           func(t *testing.T, contactType data.ReceiverContactType, r data.Receiver, entries []logrus.Entry)
		wantStatusCode         int
		wantBody               string
	}
	testCases := []testCase{}

	for _, contactType := range data.GetAllReceiverContactTypes() {
		for _, verificationField := range data.GetAllVerificationTypes() {
			receiverSendOTPRequest := ReceiverSendOTPRequest{ReCAPTCHAToken: reCAPTCHAToken}
			var contactInfo string
			var messengerType message.MessengerType
			switch contactType {
			case data.ReceiverContactTypeSMS:
				receiverSendOTPRequest.PhoneNumber = phoneNumber
				contactInfo = phoneNumber
				messengerType = message.MessengerTypeTwilioSMS
			case data.ReceiverContactTypeEmail:
				receiverSendOTPRequest.Email = email
				contactInfo = email
				messengerType = message.MessengerTypeAWSEmail
			}
			truncatedContactInfo := utils.TruncateString(contactInfo, 3)

			testCases = append(testCases, []testCase{
				{
					name:                   fmt.Sprintf("%s/%s/🔴 (500-InternalServerError) when the SMS dispatcher fails", contactType, verificationField),
					receiverSendOTPRequest: receiverSendOTPRequest,
					verificationField:      verificationField,
					contactType:            contactType,
					shouldCreateObjects:    true,
					prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, mockMessageDispatcher *message.MockMessageDispatcher) {
						mockReCAPTCHAValidator.
							On("IsTokenValid", mock.Anything, reCAPTCHAToken).
							Return(true, nil).
							Once()
						mockMessageDispatcher.
							On("SendMessage",
								mock.Anything,
								mock.AnythingOfType("message.Message"),
								[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
							Return(messengerType, errors.New("failed calling message dispatcher")).
							Once().
							Run(func(args mock.Arguments) {
								msg := args.Get(1).(message.Message)
								assert.Contains(t, msg.Message, "is your MyCustomAid verification code.")
								assert.Regexp(t, regexp.MustCompile(`^\d{6}\s.+$`), msg.Message)
							})
					},
					assertLogsFn: func(t *testing.T, contactType data.ReceiverContactType, r data.Receiver, entries []logrus.Entry) {
						contactTypeStr := utils.Humanize(string(contactType))
						wantLog := fmt.Sprintf("sending OTP message to %s %s", contactTypeStr, truncatedContactInfo)
						assert.Contains(t, entries[0].Message, wantLog)
					},
					wantStatusCode: http.StatusInternalServerError,
					wantBody:       fmt.Sprintf(`{"error":"Failed to send OTP message, reason: sending OTP message: cannot send OTP message through %s to %s: failed calling message dispatcher"}`, utils.Humanize(string(contactType)), truncatedContactInfo),
				},
				{
					name:                   fmt.Sprintf("%s/%s/🟡 (200-Ok) with false positive", contactType, verificationField),
					receiverSendOTPRequest: receiverSendOTPRequest,
					verificationField:      verificationField,
					contactType:            contactType,
					shouldCreateObjects:    false,
					prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, mockMessageDispatcher *message.MockMessageDispatcher) {
						mockReCAPTCHAValidator.
							On("IsTokenValid", mock.Anything, reCAPTCHAToken).
							Return(true, nil).
							Once()
					},
					assertLogsFn: func(t *testing.T, contactType data.ReceiverContactType, r data.Receiver, entries []logrus.Entry) {
						contactTypeStr := utils.Humanize(string(contactType))
						wantLog := fmt.Sprintf("Could not find ANY receiver verification for %s %s: %v", contactTypeStr, truncatedContactInfo, data.ErrRecordNotFound)
						assert.Contains(t, entries[0].Message, wantLog)
					},
					wantStatusCode: http.StatusOK,
					wantBody:       fmt.Sprintf(`{"message":"if your %s is registered, you'll receive an OTP","verification_field":"DATE_OF_BIRTH"}`, utils.Humanize(string(contactType))),
				},
				{
					name:                   fmt.Sprintf("%s/%s/🟢 (200-Ok) OTP sent!", contactType, verificationField),
					receiverSendOTPRequest: receiverSendOTPRequest,
					verificationField:      verificationField,
					contactType:            contactType,
					shouldCreateObjects:    true,
					prepareMocksFn: func(t *testing.T, mockReCAPTCHAValidator *validators.ReCAPTCHAValidatorMock, mockMessageDispatcher *message.MockMessageDispatcher) {
						mockReCAPTCHAValidator.
							On("IsTokenValid", mock.Anything, reCAPTCHAToken).
							Return(true, nil).
							Once()
						mockMessageDispatcher.
							On("SendMessage",
								mock.Anything,
								mock.AnythingOfType("message.Message"),
								[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail}).
							Return(messengerType, nil).
							Once().
							Run(func(args mock.Arguments) {
								msg := args.Get(1).(message.Message)
								assert.Contains(t, msg.Message, "is your MyCustomAid verification code.")
								assert.Regexp(t, regexp.MustCompile(`^\d{6}\s.+$`), msg.Message)
							})
					},
					wantStatusCode: http.StatusOK,
					wantBody:       fmt.Sprintf(`{"message":"if your %s is registered, you'll receive an OTP","verification_field":"%s"}`, utils.Humanize(string(contactType)), verificationField),
				},
			}...)
		}
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
			if tc.shouldCreateObjects {
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
					PhoneNumber: tc.receiverSendOTPRequest.PhoneNumber,
					Email:       tc.receiverSendOTPRequest.Email,
				})
				data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
				data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: tc.verificationField,
				})
			}

			mockReCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
			mockMessageDispatcher := message.NewMockMessageDispatcher(t)

			tc.prepareMocksFn(t, mockReCAPTCHAValidator, mockMessageDispatcher)

			r := chi.NewRouter()
			r.Post("/wallet-registration/otp", ReceiverSendOTPHandler{
				Models:             models,
				MessageDispatcher:  mockMessageDispatcher,
				ReCAPTCHAValidator: mockReCAPTCHAValidator,
			}.ServeHTTP)

			reqBody, err := json.Marshal(tc.receiverSendOTPRequest)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(ctxWithValidSEP24Claims, http.MethodPost, "/wallet-registration/otp", strings.NewReader(string(reqBody)))
			require.NoError(t, err)
			rr := httptest.NewRecorder()

			getEntries := log.DefaultLogger.StartTest(logrus.DebugLevel)
			r.ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.wantStatusCode, resp.StatusCode)
			assert.JSONEq(t, tc.wantBody, string(respBody))
			entries := getEntries()
			if tc.assertLogsFn != nil {
				tc.assertLogsFn(t, tc.contactType, data.Receiver{}, entries)
			}
		})
	}
}

func Test_newReceiverSendOTPResponseBody(t *testing.T) {
	for _, otpType := range data.GetAllReceiverContactTypes() {
		for _, verificationType := range data.GetAllVerificationTypes() {
			t.Run(fmt.Sprintf("%s/%s", otpType, verificationType), func(t *testing.T) {
				gotBody := newReceiverSendOTPResponseBody(otpType, verificationType)
				wantBody := ReceiverSendOTPResponseBody{
					Message:           fmt.Sprintf("if your %s is registered, you'll receive an OTP", utils.Humanize(string(otpType))),
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
			wantMessage:            fmt.Sprintf("246810 is your %s verification code. If you did not request this code, please ignore. Do not share your code with anyone.", organization.Name),
		},
		{
			name:                   "🎉 successful with default message",
			overrideOrgOTPTemplate: defaultOTPMessageTemplate,
			wantMessage:            fmt.Sprintf("246810 is your %s verification code. If you did not request this code, please ignore. Do not share your code with anyone.", organization.Name),
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
				var messengerType message.MessengerType
				switch contactType {
				case data.ReceiverContactTypeSMS:
					expectedMsg = message.Message{ToPhoneNumber: phoneNumber, Message: tc.wantMessage}
					contactInfo = phoneNumber
					messengerType = message.MessengerTypeTwilioSMS
				case data.ReceiverContactTypeEmail:
					expectedMsg = message.Message{ToEmail: email, Message: tc.wantMessage, Title: "Your One-Time Password: " + otp}
					contactInfo = email
					messengerType = message.MessengerTypeAWSEmail
				}

				mockMessageDispatcher := message.NewMockMessageDispatcher(t)
				mockCall := mockMessageDispatcher.
					On("SendMessage",
						mock.Anything,
						expectedMsg,
						[]message.MessageChannel{message.MessageChannelSMS, message.MessageChannelEmail})
				if !tc.shouldDispatcherFail {
					mockCall.Return(messengerType, nil).Once()
				} else {
					mockCall.Return(messengerType, errors.New("error sending message")).Once()
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
		wantHttpErr           func(contactType data.ReceiverContactType, r data.Receiver) *httperror.HTTPError
	}{
		{
			name: "🟡 false positive if GetLatestByContactInfo returns no results",
			contactInfo: func(r data.Receiver, contactType data.ReceiverContactType) string {
				return "not_found"
			},
			assertLogsFn: func(t *testing.T, contactType data.ReceiverContactType, r data.Receiver, entries []logrus.Entry) {
				contactTypeStr := utils.Humanize(string(contactType))
				truncatedContactInfo := utils.TruncateString("not_found", 3)
				wantLog := fmt.Sprintf("Could not find ANY receiver verification for %s %s: %v", contactTypeStr, truncatedContactInfo, data.ErrRecordNotFound)
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
				contactTypeStr := utils.Humanize(string(contactType))
				truncatedContactInfo := utils.TruncateString(r.ContactByType(contactType), 3)
				wantLog := fmt.Sprintf("Could not find a match between %s (%s) and client domain (%s)", contactTypeStr, truncatedContactInfo, "incorrect.test")
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
				contactTypeStr := utils.Humanize(string(contactType))
				truncatedContactInfo := utils.TruncateString(receiverWithoutWalletInsert.ContactByType(contactType), 3)
				wantLog := fmt.Sprintf("Could not find a match between %s (%s) and client domain (%s)", contactTypeStr, truncatedContactInfo, "correct.test")
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
					Return(message.MessengerTypeTwilioSMS, errors.New("error sending message")).
					Once()
			},
			wantVerificationField: data.VerificationTypeDateOfBirth,
			wantHttpErr: func(contactType data.ReceiverContactType, r data.Receiver) *httperror.HTTPError {
				contactTypeStr := utils.Humanize(string(contactType))
				truncatedContactInfo := utils.TruncateString(r.ContactByType(contactType), 3)
				err := fmt.Errorf("sending OTP message: %w", fmt.Errorf("cannot send OTP message through %s to %s: %w", contactTypeStr, truncatedContactInfo, errors.New("error sending message")))
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
					Return(message.MessengerTypeTwilioSMS, nil).
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
					wantHTTPErr := tc.wantHttpErr(contactType, *receiverWithWallet)
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
