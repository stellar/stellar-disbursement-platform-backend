package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_VerifyReceiverRegistrationHandler_validate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// create valid sep24 token
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://home.page", "home.page", "wallet123://")
	sep24JWTClaims := &anchorplatform.SEP24JWTClaims{
		ClientDomainClaim: wallet.SEP10ClientDomain,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}

	testCases := []struct {
		name                       string
		contextSep24Claims         *anchorplatform.SEP24JWTClaims
		requestBody                string
		isRecaptchaValidFnResponse []interface{}
		wantHTTPErr                *httperror.HTTPError
		wantSep24Claims            *anchorplatform.SEP24JWTClaims
		wantResult                 data.ReceiverRegistrationRequest
	}{
		{
			name:        "returns a 401 response if SEP24 token is missing",
			wantHTTPErr: httperror.Unauthorized("", fmt.Errorf("no SEP-24 claims found in the request context"), nil),
		},
		{
			name:               "returns a 400 response if the request body is empty",
			contextSep24Claims: sep24JWTClaims,
			wantHTTPErr:        httperror.BadRequest("", nil, nil),
		},
		{
			name:               "returns a 400 response if the request body is invalid",
			contextSep24Claims: sep24JWTClaims,
			requestBody:        "invalid",
			wantHTTPErr:        httperror.BadRequest("", nil, nil),
		},
		{
			name:                       "returns a 500 response if the reCAPTCHA validation returns an error",
			contextSep24Claims:         sep24JWTClaims,
			requestBody:                `{"reCAPTCHA_token": "token"}`,
			isRecaptchaValidFnResponse: []interface{}{false, errors.New("unexpected error")},
			wantHTTPErr:                httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", errors.New("unexpected error"), nil),
		},
		{
			name:                       "returns a 400 response if the reCAPTCHA token is invalid",
			contextSep24Claims:         sep24JWTClaims,
			requestBody:                `{"reCAPTCHA_token": "token"}`,
			isRecaptchaValidFnResponse: []interface{}{false, nil},
			wantHTTPErr:                httperror.BadRequest("", nil, nil),
		},
		{
			name:               "returns a 400 response if a body field is invalid",
			contextSep24Claims: sep24JWTClaims,
			requestBody: `{
				"phone_number": "+380445555555",
				"otp": "",
				"verification": "1990-01-01",
				"verification_type": "date_of_birth",
				"reCAPTCHA_token": "token"
			}`,
			isRecaptchaValidFnResponse: []interface{}{true, nil},
			wantHTTPErr:                httperror.BadRequest("", nil, map[string]interface{}{"otp": "invalid otp format. Needs to be a 6 digit value"}),
		},
		{
			name:               "🎉 successfully parses the body into an object if the SEP24 token, recaptcha token and reqquest body are all valid",
			contextSep24Claims: sep24JWTClaims,
			requestBody: `{
				"phone_number": "+380445555555",
				"otp": "123456",
				"verification": "1990-01-01",
				"verification_type": "date_of_birth",
				"reCAPTCHA_token": "token"
			}`,
			isRecaptchaValidFnResponse: []interface{}{true, nil},
			wantSep24Claims:            sep24JWTClaims,
			wantResult: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationType:  data.VerificationFieldDateOfBirth,
				ReCAPTCHAToken:    "token",
			},
		},
	}

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}
	handler := &VerifyReceiverRegistrationHandler{
		Models:                   models,
		AnchorPlatformAPIService: &mockAnchorPlatformService,
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var requestBody io.Reader
			if tc.requestBody != "" {
				requestBody = strings.NewReader(tc.requestBody)
			}

			req, err := http.NewRequest("POST", "/wallet-registration/verification", requestBody)
			require.NoError(t, err)

			if tc.contextSep24Claims != nil {
				req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, tc.contextSep24Claims))
			}

			if tc.isRecaptchaValidFnResponse != nil {
				reCAPTCHAValidatorMock := &validators.ReCAPTCHAValidatorMock{}
				handler.ReCAPTCHAValidator = reCAPTCHAValidatorMock
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(tc.isRecaptchaValidFnResponse...).
					Once()
				defer reCAPTCHAValidatorMock.AssertExpectations(t)
			}

			gotReceiverRegistrationRequest, gotSep24Claims, httpErr := handler.validate(req)
			if tc.wantHTTPErr == nil {
				require.Nil(t, httpErr)
				assert.Equal(t, tc.wantSep24Claims, gotSep24Claims)
				assert.Equal(t, tc.wantResult, gotReceiverRegistrationRequest)
			} else {
				require.NotNil(t, httpErr)
				assert.Equal(t, tc.wantHTTPErr.StatusCode, httpErr.StatusCode)
				assert.Equal(t, tc.wantHTTPErr.Message, httpErr.Message)
				assert.Equal(t, tc.wantHTTPErr.Extras, httpErr.Extras)
				assert.Nil(t, gotSep24Claims)
				assert.Empty(t, gotReceiverRegistrationRequest)
			}
		})
	}
}

func Test_VerifyReceiverRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://home.page", "home.page", "wallet123://")

	mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}
	reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
	handler := &VerifyReceiverRegistrationHandler{
		Models:                   models,
		AnchorPlatformAPIService: &mockAnchorPlatformService,
		ReCAPTCHAValidator:       reCAPTCHAValidator,
	}

	// setup router
	r := chi.NewRouter()
	r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)

	t.Run("error receiver not found in our server", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := fmt.Sprintf(`{
			"error": "%s"
		  }`, InformationNotFoundOnServer)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "receiver with phone number +380445555555 not found in our server")
	})

	t.Run("error receiver verification not found in our server", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		_ = data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := fmt.Sprintf(`{
			"error": "%s"
		  }`, InformationNotFoundOnServer)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "DATE_OF_BIRTH not found for receiver with phone number +380445555555")
	})

	t.Run("error comparing verification values exceeded attempts", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		receiverVerification := data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "2000-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)

		attempts := 0

		const totalAttempts = data.MaxAttemptsAllowed + 1
		for range [totalAttempts]interface{}{} {
			buf.Reset()

			req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

			reCAPTCHAValidator.
				On("IsTokenValid", mock.Anything, "token").
				Return(true, nil).
				Once()

			r.ServeHTTP(rr, req)

			attempts += 1

			resp := rr.Result()
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			want := fmt.Sprintf(`{
				"error": "%s"
			  }`, InformationNotFoundOnServer)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.JSONEq(t, want, string(respBody))

			// validate the number of attempts
			query := `
				SELECT
					rv.attempts
				FROM
					receiver_verifications rv
				WHERE
					rv.receiver_id = $1 AND rv.verification_field = $2
			`
			receiverVerificationUpdated := data.ReceiverVerification{}
			err = dbConnectionPool.GetContext(ctx, &receiverVerificationUpdated, query, receiverVerification.ReceiverID, receiverVerification.VerificationField)
			require.NoError(t, err)

			expectedLog := ""
			if attempts == totalAttempts {
				expectedLog = "number of attempts to confirm the verification value exceeded max attempts value 6"
				assert.Equal(t, data.MaxAttemptsAllowed, receiverVerificationUpdated.Attempts)
			} else {
				expectedLog = "DATE_OF_BIRTH value does not match for user with phone number +380445555555"
				assert.Equal(t, attempts, receiverVerificationUpdated.Attempts)
			}

			// validate logs
			require.Contains(t, buf.String(), expectedLog)
		}
	})

	t.Run("error comparing verification values", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		receiverVerification := data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "2000-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := fmt.Sprintf(`{
			"error": "%s"
		  }`, InformationNotFoundOnServer)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate if the receiver verification has been updated
		query := `
			SELECT
				rv.attempts,
				rv.confirmed_at,
				rv.failed_at
			FROM
				receiver_verifications rv
			WHERE
				rv.receiver_id = $1 AND rv.verification_field = $2
		`
		receiverVerificationUpdated := data.ReceiverVerification{}
		err = dbConnectionPool.GetContext(ctx, &receiverVerificationUpdated, query, receiverVerification.ReceiverID, receiverVerification.VerificationField)
		require.NoError(t, err)

		assert.Empty(t, receiverVerificationUpdated.ConfirmedAt)
		assert.NotEmpty(t, receiverVerificationUpdated.FailedAt)
		assert.Equal(t, 1, receiverVerificationUpdated.Attempts)

		// validate logs
		require.Contains(t, buf.String(), "DATE_OF_BIRTH value does not match for user with phone number +380445555555")
	})

	t.Run("error getting receiver wallet", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := fmt.Sprintf(`{
			"error": "%s"
		  }`, InformationNotFoundOnServer)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate logs
		msg := fmt.Sprintf("receiver wallet not found for receiver with id %s and client domain home.page", receiver.ID)
		require.Contains(t, buf.String(), msg)
	})

	t.Run("error receiver wallet otp does not match the value saved in the database", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "111111",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := fmt.Sprintf(`{
			"error": "%s"
		  }`, InformationNotFoundOnServer)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "receiver wallet otp is not valid: otp does not match with value saved in the database")
	})

	t.Run("error receiver wallet otp is expired", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		query := `
			UPDATE
				receiver_wallets rw
			SET
				otp_created_at = $1
			WHERE
				rw.stellar_address = $2
		`
		expiredOTPCreatedAt := time.Now().Add(-data.OTPExpirationTimeMinutes * time.Minute).Add(-time.Second) // expired 1 second ago
		_, err = dbConnectionPool.ExecContext(ctx, query, expiredOTPCreatedAt, receiverWallet.StellarAddress)
		require.NoError(t, err)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}
		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := fmt.Sprintf(`{"error": "%s"}`, InformationNotFoundOnServer)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "receiver wallet otp is not valid: otp is expired")
	})

	t.Run("error anchor platform service API", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		// set stellar values to empty
		query := `
			UPDATE
				receiver_wallets rw
			SET
				stellar_address = '',
				stellar_memo = '',
				stellar_memo_type = ''
			WHERE
				rw.id = $1
		`
		_, err = dbConnectionPool.ExecContext(ctx, query, receiverWallet.ID)
		require.NoError(t, err)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		transaction := &anchorplatform.Transaction{
			TransactionValues: anchorplatform.TransactionValues{
				ID:                 "test-transaction-id",
				Status:             "pending_anchor",
				Sep:                "24",
				Kind:               "deposit",
				DestinationAccount: validClaims.SEP10StellarAccount(),
				Memo:               validClaims.SEP10StellarMemo(),
				KYCVerified:        true,
			},
		}
		mockAnchorPlatformService.
			On("UpdateAnchorTransactions", mock.Anything, []anchorplatform.Transaction{*transaction}).
			Return(fmt.Errorf("error updating transaction on anchor platform")).Once()

		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := `{
			"error": "An internal error occurred while processing this request."
		  }
		`

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate if the receiver wallet has been updated
		query = `
			SELECT
				rw.status,
				rw.stellar_address,
				rw.stellar_memo,
				rw.stellar_memo_type,
				otp_confirmed_at
			FROM
				receiver_wallets rw
			WHERE
				rw.id = $1
		`
		receiverWalletUpdated := data.ReceiverWallet{}
		err = dbConnectionPool.GetContext(ctx, &receiverWalletUpdated, query, receiverWallet.ID)
		require.NoError(t, err)

		assert.Equal(t, data.ReadyReceiversWalletStatus, receiverWalletUpdated.Status)
		assert.Empty(t, receiverWalletUpdated.StellarAddress)
		assert.Empty(t, receiverWalletUpdated.StellarMemo)
		assert.Empty(t, receiverWalletUpdated.StellarMemoType)
		require.Empty(t, receiverWalletUpdated.OTPConfirmedAt)

		// validate logs
		require.Contains(t, buf.String(), "updating transaction with ID test-transaction-id on anchor platform API")
		mockAnchorPlatformService.AssertExpectations(t)
	})

	t.Run("receiver already registered", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)
		log.DefaultLogger.SetLevel(log.InfoLevel)

		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := `{
			"message": "ok"
		  }
		`

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "receiver already registered in the SDP")
	})

	t.Run("invalid receiver wallet status", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)
		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := fmt.Sprintf(`{
			"error": "%s"
		  }`, InformationNotFoundOnServer)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate logs
		msg := fmt.Sprintf("transitioning status for receiver[ID=%s], receiverWallet[ID=%s]", receiver.ID, receiverWallet.ID)
		require.Contains(t, buf.String(), msg)
	})

	t.Run("successfully verifying receiver registration without stellar memo", func(t *testing.T) {
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		transaction := &anchorplatform.Transaction{
			TransactionValues: anchorplatform.TransactionValues{
				ID:                 "test-transaction-id",
				Status:             "pending_anchor",
				Sep:                "24",
				Kind:               "deposit",
				DestinationAccount: validClaims.SEP10StellarAccount(),
				Memo:               validClaims.SEP10StellarMemo(),
				KYCVerified:        true,
			},
		}
		mockAnchorPlatformService.
			On("UpdateAnchorTransactions", mock.Anything, []anchorplatform.Transaction{*transaction}).
			Return(nil).Once()

		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := `{
			"message": "ok"
		  }
		`
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate if the receiver wallet has been updated
		query := `
			SELECT
				rw.status,
				rw.stellar_address,
				COALESCE(rw.stellar_memo, '') as "stellar_memo",
				COALESCE(rw.stellar_memo_type, '') as "stellar_memo_type",
				otp_confirmed_at
			FROM
				receiver_wallets rw
			WHERE
				rw.id = $1
		`
		receiverWalletUpdated := data.ReceiverWallet{}
		err = dbConnectionPool.GetContext(ctx, &receiverWalletUpdated, query, receiverWallet.ID)
		require.NoError(t, err)

		assert.Equal(t, data.RegisteredReceiversWalletStatus, receiverWalletUpdated.Status)
		assert.Equal(t, "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", receiverWalletUpdated.StellarAddress)
		assert.Empty(t, receiverWalletUpdated.StellarMemo)
		assert.Empty(t, receiverWalletUpdated.StellarMemoType)
		require.NotEmpty(t, receiverWalletUpdated.OTPConfirmedAt)

		mockAnchorPlatformService.AssertExpectations(t)
	})

	t.Run("successfully verifying receiver registration with stellar memo", func(t *testing.T) {
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+380445555555",
		})

		receiverVerification := data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		request := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "date_of_birth",
			ReCAPTCHAToken:    "token",
		}

		reqBody, err := json.Marshal(request)
		require.NoError(t, err)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)

		rr := httptest.NewRecorder()

		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create valid sep24 token
		validClaims := &anchorplatform.SEP24JWTClaims{
			ClientDomainClaim: wallet.SEP10ClientDomain,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-transaction-id",
				Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444:123456",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			},
		}
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))

		transaction := &anchorplatform.Transaction{
			TransactionValues: anchorplatform.TransactionValues{
				ID:                 "test-transaction-id",
				Status:             "pending_anchor",
				Sep:                "24",
				Kind:               "deposit",
				DestinationAccount: validClaims.SEP10StellarAccount(),
				Memo:               validClaims.SEP10StellarMemo(),
				KYCVerified:        true,
			},
		}
		mockAnchorPlatformService.
			On("UpdateAnchorTransactions", mock.Anything, []anchorplatform.Transaction{*transaction}).
			Return(nil).Once()

		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		want := `{
			"message": "ok"
		  }
		`
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, want, string(respBody))

		// validate if the receiver wallet has been updated
		query := `
			SELECT
				rw.status,
				rw.stellar_address,
				rw.stellar_memo,
				rw.stellar_memo_type,
				otp_confirmed_at
			FROM
				receiver_wallets rw
			WHERE
				rw.id = $1
		`
		receiverWalletUpdated := data.ReceiverWallet{}
		err = dbConnectionPool.GetContext(ctx, &receiverWalletUpdated, query, receiverWallet.ID)
		require.NoError(t, err)

		assert.Equal(t, data.RegisteredReceiversWalletStatus, receiverWalletUpdated.Status)
		assert.Equal(t, "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", receiverWalletUpdated.StellarAddress)
		assert.Equal(t, "123456", receiverWalletUpdated.StellarMemo)
		assert.Equal(t, "id", receiverWalletUpdated.StellarMemoType)
		require.NotEmpty(t, receiverWalletUpdated.OTPConfirmedAt)

		// validate if the receiver verification field confirmed_at has been updated
		query = `
			SELECT
				rv.confirmed_at
			FROM
				receiver_verifications rv
			WHERE
				rv.receiver_id = $1 AND rv.verification_field = $2
		`
		receiverVerificationUpdated := data.ReceiverVerification{}
		err = dbConnectionPool.GetContext(ctx, &receiverVerificationUpdated, query, receiverVerification.ReceiverID, receiverVerification.VerificationField)
		require.NoError(t, err)

		assert.NotEmpty(t, receiverVerificationUpdated.ConfirmedAt)

		mockAnchorPlatformService.AssertExpectations(t)
	})

	reCAPTCHAValidator.AssertExpectations(t)
}
