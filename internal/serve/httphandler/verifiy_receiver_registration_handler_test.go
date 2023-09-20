package httphandler

import (
	"context"
	"database/sql"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
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
	defer data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
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

func Test_VerifyReceiverRegistrationHandler_processReceiverVerificationPII(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}
	reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
	handler := &VerifyReceiverRegistrationHandler{
		Models:                   models,
		AnchorPlatformAPIService: &mockAnchorPlatformService,
		ReCAPTCHAValidator:       reCAPTCHAValidator,
	}

	// receiver without receiver_verification row:
	receiverMissingReceiverVerification := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: "+380443333333"})

	// receiver with a receiverVerification row that's exceeded the maximum number of attempts:
	receiverWithExceededAttempts := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: "+380446666666"})
	receiverVerificationExceededAttempts := data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
		ReceiverID:        receiverWithExceededAttempts.ID,
		VerificationField: data.VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-01",
	})
	receiverVerificationExceededAttempts.Attempts = data.MaxAttemptsAllowed
	err = models.ReceiverVerification.UpdateReceiverVerification(ctx, *receiverVerificationExceededAttempts, dbConnectionPool)
	require.NoError(t, err)

	// receiver with receiver_verification row:
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: "+380445555555"})
	_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
		ReceiverID:        receiver.ID,
		VerificationField: data.VerificationFieldDateOfBirth,
		VerificationValue: "1990-01-01",
	})

	defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)

	testCases := []struct {
		name                      string
		receiver                  data.Receiver
		registrationRequest       data.ReceiverRegistrationRequest
		shouldAssertAttemptsCount bool
		wantErrContains           string
	}{
		{
			name:     "returns an error if the receiver does not have any receiverVerification row",
			receiver: *receiverMissingReceiverVerification,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiverMissingReceiverVerification.PhoneNumber,
				VerificationType:  data.VerificationFieldDateOfBirth,
				VerificationValue: "1990-01-01",
			},
			wantErrContains: "DATE_OF_BIRTH not found for receiver with phone number +38...333",
		},
		{
			name:     "returns an error if the receiver does not have any receiverVerification row with the given verification type",
			receiver: *receiver,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiver.PhoneNumber,
				VerificationType:  data.VerificationFieldNationalID,
				VerificationValue: "123456",
			},
			wantErrContains: "NATIONAL_ID_NUMBER not found for receiver with phone number +38...555",
		},
		{
			name:     "returns an error if the receiver has exceeded their max attempts to confirm the verification value",
			receiver: *receiverWithExceededAttempts,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiverWithExceededAttempts.PhoneNumber,
				VerificationType:  data.VerificationFieldDateOfBirth,
				VerificationValue: "1990-01-01",
			},
			wantErrContains: "the number of attempts to confirm the verification value exceededs the max attempts limit of 6",
		},
		{
			name:     "returns an error if the varification value provided in the payload is different from the DB one",
			receiver: *receiver,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiver.PhoneNumber,
				VerificationType:  data.VerificationFieldDateOfBirth,
				VerificationValue: "1990-11-11", // <--- different from the DB one (1990-01-01)
			},
			shouldAssertAttemptsCount: true,
			wantErrContains:           "DATE_OF_BIRTH value does not match for user with phone number +38...555",
		},
		{
			name:     "🎉 successfully process the verification value and updates it accordingly in the DB",
			receiver: *receiver,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiver.PhoneNumber,
				VerificationType:  data.VerificationFieldDateOfBirth,
				VerificationValue: "1990-01-01",
			},
			shouldAssertAttemptsCount: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			var receiverVerifications []*data.ReceiverVerification
			var receiverVerificationInitial *data.ReceiverVerification
			if tc.shouldAssertAttemptsCount {
				receiverVerifications, err = models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{tc.receiver.ID}, tc.registrationRequest.VerificationType)
				require.NoError(t, err)
				require.Len(t, receiverVerifications, 1)
				receiverVerificationInitial = receiverVerifications[0]
			}

			err = handler.processReceiverVerificationPII(ctx, dbTx, tc.receiver, tc.registrationRequest)

			if tc.wantErrContains == "" {
				receiverVerifications, err = models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{tc.receiver.ID}, tc.registrationRequest.VerificationType)
				require.NoError(t, err)
				require.Len(t, receiverVerifications, 1)
				receiverVerification := receiverVerifications[0]
				assert.NotEmpty(t, receiverVerification.ConfirmedAt)
				assert.Equal(t, receiverVerificationInitial.Attempts, receiverVerification.Attempts, "attempts should not have been incremented")
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
				if tc.shouldAssertAttemptsCount {
					receiverVerifications, err = models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{tc.receiver.ID}, tc.registrationRequest.VerificationType)
					require.NoError(t, err)
					require.Len(t, receiverVerifications, 1)
					receiverVerification := receiverVerifications[0]
					assert.Equal(t, receiverVerificationInitial.Attempts+1, receiverVerification.Attempts, "attempts should have been incremented")
				}
			}
		})
	}
}

func Test_VerifyReceiverRegistrationHandler_processReceiverWalletOTP(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}
	handler := &VerifyReceiverRegistrationHandler{
		Models:                   models,
		AnchorPlatformAPIService: &mockAnchorPlatformService,
	}

	// create valid sep24 token
	defer data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
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
		name                        string
		sep24Claims                 *anchorplatform.SEP24JWTClaims
		currentReceiverWalletStatus data.ReceiversWalletStatus
		// shouldOTPMatch is used to simulate the case where the OTP provided in the request body is equals or different from the one saved in the DB
		shouldOTPMatch           bool
		wantWasAlreadyRegistered bool
		wantErrContains          []string
	}{
		{
			name:                        "returns an error if the receiver wallet cannot be found",
			sep24Claims:                 sep24JWTClaims,
			currentReceiverWalletStatus: "", // <--- no receiver wallet exists
			wantErrContains:             []string{"receiver wallet not found for receiverID=", " and clientDomain=home.page: error querying receiver wallet: sql: no rows in result set"},
		},
		{
			name:                        "🎉 successfully identifies a receiverWallet that has already been marked as REGISTERED",
			sep24Claims:                 sep24JWTClaims,
			currentReceiverWalletStatus: data.RegisteredReceiversWalletStatus,
			wantWasAlreadyRegistered:    true,
		},
		{
			name:                        "returns an error if the receiver wallet is in a status that cannot be transitioned to REGISTERED",
			sep24Claims:                 sep24JWTClaims,
			currentReceiverWalletStatus: data.DraftReceiversWalletStatus,
			wantErrContains:             []string{"transitioning status for receiverWallet[ID=", "cannot transition from DRAFT to REGISTERED"},
		},
		{
			name:                        "returns an error if the OTPs don't match",
			sep24Claims:                 sep24JWTClaims,
			currentReceiverWalletStatus: data.ReadyReceiversWalletStatus,
			wantErrContains:             []string{"receiver wallet OTP is not valid: otp does not match with value saved in the database"},
		},
		{
			name:                        "🎉 successfully updates a receiverWallet to REGISTERED",
			sep24Claims:                 sep24JWTClaims,
			currentReceiverWalletStatus: data.ReadyReceiversWalletStatus,
			shouldOTPMatch:              true,
			wantWasAlreadyRegistered:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// dbTX
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			// OTP
			const correctOTP = "123456"
			const wrongOTP = "111111"
			otp := correctOTP
			if !tc.shouldOTPMatch {
				otp = wrongOTP
			}

			// receiver & receiver wallet
			receiver := data.CreateReceiverFixture(t, ctx, dbTx, &data.Receiver{PhoneNumber: "+380445555555"})
			var receiverWallet *data.ReceiverWallet
			if tc.currentReceiverWalletStatus.State() != "" {
				receiverWallet = data.CreateReceiverWalletFixture(t, ctx, dbTx, receiver.ID, wallet.ID, tc.currentReceiverWalletStatus)
				var stellarAddress string
				var otpConfirmedAt *time.Time
				if tc.wantWasAlreadyRegistered {
					stellarAddress = "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444"
					now := time.Now()
					otpConfirmedAt = &now
				}

				const q = `
					UPDATE receiver_wallets
					SET otp = $1, otp_created_at = NOW(), stellar_address = $2, otp_confirmed_at = $3
					WHERE id = $4
				`
				_, err = dbTx.ExecContext(ctx, q, correctOTP, sql.NullString{String: stellarAddress, Valid: stellarAddress != ""}, otpConfirmedAt, receiverWallet.ID)
				require.NoError(t, err)
			}

			// assertions
			rwUpdated, wasAlreadyRegistered, err := handler.processReceiverWalletOTP(ctx, dbTx, *tc.sep24Claims, *receiver, otp)
			if tc.wantErrContains == nil {
				require.NoError(t, err)
				assert.Equal(t, tc.wantWasAlreadyRegistered, wasAlreadyRegistered)

				// get receiverWallet from DB
				var rw data.ReceiverWallet
				err = dbTx.GetContext(ctx, &rw, "SELECT id, status, stellar_address, otp_confirmed_at FROM receiver_wallets WHERE id = $1", receiverWallet.ID)
				require.NoError(t, err)
				assert.Equal(t, data.RegisteredReceiversWalletStatus, rw.Status)
				assert.Equal(t, rwUpdated.Status, rw.Status)
				assert.NotEmpty(t, rw.StellarAddress)
				assert.Equal(t, rwUpdated.StellarAddress, rw.StellarAddress)
				assert.NotNil(t, rw.OTPConfirmedAt)
				assert.NotNil(t, rwUpdated.OTPConfirmedAt)
				assert.WithinDuration(t, *rwUpdated.OTPConfirmedAt, *rw.OTPConfirmedAt, time.Millisecond)

			} else {
				for _, wantErrContain := range tc.wantErrContains {
					assert.ErrorContains(t, err, wantErrContain)
				}
				require.False(t, wasAlreadyRegistered)
			}
		})
	}
}

func Test_VerifyReceiverRegistrationHandler_processAnchorPlatformID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	handler := &VerifyReceiverRegistrationHandler{Models: models}

	// creeate fixtures
	const phoneNumber = "+380445555555"
	defer data.DeleteAllFixtures(t, ctx, dbConnectionPool)
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://home.page", "home.page", "wallet123://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	// create valid sep24 token
	sep24Claims := &anchorplatform.SEP24JWTClaims{
		ClientDomainClaim: wallet.SEP10ClientDomain,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}

	// mocks
	apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
		ID:     "test-transaction-id",
		Status: "pending_anchor",
		SEP:    "24",
	}

	testCases := []struct {
		name            string
		mockReturnError error
		wantErrContains string
	}{
		{
			name:            "returns an error if the Anchor Platdorm API returns an error",
			mockReturnError: fmt.Errorf("error updating transaction on anchor platform"),
			wantErrContains: "error updating transaction on anchor platform",
		},
		{
			name: "🎉 successfully updates the transaction on the Anchor Platform",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// dbTX
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, dbTx.Rollback())
			}()

			// mocks
			mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}
			defer mockAnchorPlatformService.AssertExpectations(t)
			handler.AnchorPlatformAPIService = &mockAnchorPlatformService
			mockAnchorPlatformService.
				On("PatchAnchorTransactionsPostRegistration", mock.Anything, apTxPatch).
				Return(tc.mockReturnError).Once()

			// assertions
			err = handler.processAnchorPlatformID(ctx, dbTx, *sep24Claims, *receiverWallet)
			if tc.wantErrContains == "" {
				require.NoError(t, err)

				// make sure the receiverWallet was updated in the DB with the anchor platform transaction ID
				var rw data.ReceiverWallet
				err = dbTx.GetContext(ctx, &rw, "SELECT id, anchor_platform_transaction_id FROM receiver_wallets WHERE id = $1", receiverWallet.ID)
				require.NoError(t, err)
				assert.Equal(t, "test-transaction-id", rw.AnchorPlatformTransactionID)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_VerifyReceiverRegistrationHandler_VerifyReceiverRegistration(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	handler := &VerifyReceiverRegistrationHandler{Models: models}

	// create valid sep24 token
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://home.page", "home.page", "wallet123://")
	validClaims := &anchorplatform.SEP24JWTClaims{
		ClientDomainClaim: wallet.SEP10ClientDomain,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-transaction-id",
			Subject:   "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}

	const phoneNumber = "+380445555555"
	receiverRegistrationRequest := data.ReceiverRegistrationRequest{
		PhoneNumber:       phoneNumber,
		OTP:               "123456",
		VerificationValue: "1990-01-01",
		VerificationType:  "date_of_birth",
		ReCAPTCHAToken:    "token",
	}
	reqBody, err := json.Marshal(receiverRegistrationRequest)
	require.NoError(t, err)
	r := chi.NewRouter()

	t.Run("returns an error when validate() fails - testing case where a SEP24 claims are missing from the context", func(t *testing.T) {
		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", nil)
		require.NoError(t, err)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "validating request in VerifyReceiverRegistrationHandler: no SEP-24 claims found in the request context")
	})

	t.Run("returns an error if the receiver cannot be found", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
		defer reCAPTCHAValidator.AssertExpectations(t)
		handler.ReCAPTCHAValidator = reCAPTCHAValidator
		reCAPTCHAValidator.On("IsTokenValid", mock.Anything, "token").Return(true, nil).Once()

		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		wantBody := fmt.Sprintf(`{"error": "%s"}`, InformationNotFoundOnServer)
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "receiver with phone number +38...555 not found in our server")
	})

	t.Run("returns an error when processReceiverVerificationPII() fails - testing case where no receiverVerification is found", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
		defer reCAPTCHAValidator.AssertExpectations(t)
		handler.ReCAPTCHAValidator = reCAPTCHAValidator
		reCAPTCHAValidator.On("IsTokenValid", mock.Anything, "token").Return(true, nil).Once()

		// update database with the entries needed
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		_ = data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})

		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		wantBody := fmt.Sprintf(`{"error": "%s"}`, InformationNotFoundOnServer)
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "processing receiver verification entry for receiver with phone number +38...555: DATE_OF_BIRTH not found for receiver with phone number +38...555")
	})

	t.Run("returns an error when processReceiverWalletOTP() fails - testing case where no receiverWallet is found", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
		defer reCAPTCHAValidator.AssertExpectations(t)
		handler.ReCAPTCHAValidator = reCAPTCHAValidator
		reCAPTCHAValidator.On("IsTokenValid", mock.Anything, "token").Return(true, nil).Once()

		// update database with the entries needed
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})

		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		wantBody := fmt.Sprintf(`{"error": "%s"}`, InformationNotFoundOnServer)
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		wantErrContains := fmt.Sprintf("processing OTP for receiver with phone number +38...555: receiver wallet not found for receiverID=%s and clientDomain=home.page", receiver.ID)
		require.Contains(t, buf.String(), wantErrContains)
	})

	t.Run("returns an error when processAnchorPlatformID() fails - anchor platform returns an error", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
		defer reCAPTCHAValidator.AssertExpectations(t)
		handler.ReCAPTCHAValidator = reCAPTCHAValidator
		reCAPTCHAValidator.On("IsTokenValid", mock.Anything, "token").Return(true, nil).Once()

		apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
			ID:     "test-transaction-id",
			Status: "pending_anchor",
			SEP:    "24",
		}
		mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}
		defer mockAnchorPlatformService.AssertExpectations(t)
		handler.AnchorPlatformAPIService = &mockAnchorPlatformService
		mockAnchorPlatformService.
			On("PatchAnchorTransactionsPostRegistration", mock.Anything, apTxPatch).
			Return(fmt.Errorf("error updating transaction on anchor platform")).Once()

		// update database with the entries needed
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationFieldDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
		require.NoError(t, err)

		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, err)
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		wantBody := `{"error": "An internal error occurred while processing this request."}`
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		wantErrContains := fmt.Sprintf("processing anchor platform transaction ID: updating transaction with ID %s on anchor platform API", validClaims.TransactionID())
		require.Contains(t, buf.String(), wantErrContains)
	})

	t.Run("🎉 successfully registers receiver's stellar address", func(t *testing.T) {
		testCases := []struct {
			name      string
			inputMemo string
		}{
			{
				name:      "without memo",
				inputMemo: "",
			},
			{
				name:      "with memo",
				inputMemo: "123456",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				sep24Claims := *validClaims
				sep24Claims.RegisteredClaims.Subject += ":" + tc.inputMemo

				// mocks
				reCAPTCHAValidator := &validators.ReCAPTCHAValidatorMock{}
				defer reCAPTCHAValidator.AssertExpectations(t)
				handler.ReCAPTCHAValidator = reCAPTCHAValidator
				reCAPTCHAValidator.On("IsTokenValid", mock.Anything, "token").Return(true, nil).Once()

				apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
					ID:     "test-transaction-id",
					Status: "pending_anchor",
					SEP:    "24",
				}
				mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}
				defer mockAnchorPlatformService.AssertExpectations(t)
				handler.AnchorPlatformAPIService = &mockAnchorPlatformService
				mockAnchorPlatformService.On("PatchAnchorTransactionsPostRegistration", mock.Anything, apTxPatch).Return(nil).Once()

				// update database with the entries needed
				defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
				_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: data.VerificationFieldDateOfBirth,
					VerificationValue: "1990-01-01",
				})
				receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
				_, err := models.ReceiverWallet.UpdateOTPByReceiverPhoneNumberAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
				require.NoError(t, err)

				// setup router and execute request
				r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
				req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
				require.NoError(t, err)
				req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, &sep24Claims))
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				// execute and validate response
				resp := rr.Result()
				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				wantBody := `{"message": "ok"}`
				assert.JSONEq(t, wantBody, string(respBody))

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
				require.NotEmpty(t, receiverWalletUpdated.OTPConfirmedAt)
				if tc.inputMemo == "" {
					assert.Empty(t, receiverWalletUpdated.StellarMemo)
					assert.Empty(t, receiverWalletUpdated.StellarMemoType)
				} else {
					assert.Equal(t, tc.inputMemo, receiverWalletUpdated.StellarMemo)
					assert.Equal(t, "id", receiverWalletUpdated.StellarMemoType)
				}
			})
		}
	})
}
