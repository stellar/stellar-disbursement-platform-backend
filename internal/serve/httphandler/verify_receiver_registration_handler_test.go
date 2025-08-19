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
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
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
		isReCAPTCHADisabled        bool
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
			wantHTTPErr:                httperror.BadRequest("reCAPTCHA token is invalid", nil, nil),
		},
		{
			name:               "returns a 400 response if a body field is invalid",
			contextSep24Claims: sep24JWTClaims,
			requestBody: `{
				"phone_number": "+380445555555",
				"otp": "",
				"verification": "1990-01-01",
				"verification_field": "date_of_birth",
				"reCAPTCHA_token": "token"
			}`,
			isRecaptchaValidFnResponse: []interface{}{true, nil},
			wantHTTPErr:                httperror.BadRequest("", nil, map[string]interface{}{"otp": "invalid otp format. Needs to be a 6 digit value"}),
		},
		{
			name:               "🎉 successfully parses the body into an object if the SEP24 token, recaptcha token and request body are all valid",
			contextSep24Claims: sep24JWTClaims,
			requestBody: `{
				"phone_number": "+380445555555",
				"otp": "123456",
				"verification": "1990-01-01",
				"verification_field": "date_of_birth",
				"reCAPTCHA_token": "token"
			}`,
			isRecaptchaValidFnResponse: []interface{}{true, nil},
			wantSep24Claims:            sep24JWTClaims,
			wantResult: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
				ReCAPTCHAToken:    "token",
			},
		},
		{
			name:               "🎉 successful when recaptcha is disabled",
			contextSep24Claims: sep24JWTClaims,
			requestBody: `{
				"phone_number": "+380445555555",
				"otp": "123456",
				"verification": "1990-01-01",
				"verification_field": "date_of_birth"
			}`,
			// isRecaptchaValidFnResponse: []interface{}{false, nil},
			isReCAPTCHADisabled: true,
			wantSep24Claims:     sep24JWTClaims,
			wantResult: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
		},
	}

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	mockAnchorPlatformService := anchorplatform.AnchorPlatformAPIServiceMock{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &VerifyReceiverRegistrationHandler{
				Models:                   models,
				AnchorPlatformAPIService: &mockAnchorPlatformService,
				ReCAPTCHADisabled:        tc.isReCAPTCHADisabled,
			}

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
				reCAPTCHAValidatorMock := validators.NewReCAPTCHAValidatorMock(t)
				handler.ReCAPTCHAValidator = reCAPTCHAValidatorMock
				reCAPTCHAValidatorMock.
					On("IsTokenValid", mock.Anything, "token").
					Return(tc.isRecaptchaValidFnResponse...).
					Once()
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
		VerificationField: data.VerificationTypeDateOfBirth,
		VerificationValue: "1990-01-01",
	})
	receiverVerificationExceededAttempts.Attempts = data.MaxAttemptsAllowed
	err = models.ReceiverVerification.UpdateReceiverVerification(ctx, data.ReceiverVerificationUpdate{
		ReceiverID:          receiverWithExceededAttempts.ID,
		VerificationField:   data.VerificationTypeDateOfBirth,
		Attempts:            utils.IntPtr(data.MaxAttemptsAllowed),
		VerificationChannel: message.MessageChannelSMS,
	}, dbConnectionPool)
	require.NoError(t, err)

	// receiver with receiver_verification row:
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: "+380445555555"})
	_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
		ReceiverID:        receiver.ID,
		VerificationField: data.VerificationTypeDateOfBirth,
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
				VerificationField: data.VerificationTypeDateOfBirth,
				VerificationValue: "1990-01-01",
			},
			wantErrContains: "DATE_OF_BIRTH not found for receiver id " + receiverMissingReceiverVerification.ID,
		},
		{
			name:     "returns an error if the receiver does not have any receiverVerification row with the given verification type (YEAR_MONTH)",
			receiver: *receiver,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiver.PhoneNumber,
				VerificationField: data.VerificationTypeYearMonth,
				VerificationValue: "1999-12",
			},
			wantErrContains: "YEAR_MONTH not found for receiver id " + receiver.ID,
		},
		{
			name:     "returns an error if the receiver does not have any receiverVerification row with the given verification type (NATIONAL_ID_NUMBER)",
			receiver: *receiver,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiver.PhoneNumber,
				VerificationField: data.VerificationTypeNationalID,
				VerificationValue: "123456",
			},
			wantErrContains: "NATIONAL_ID_NUMBER not found for receiver id " + receiver.ID,
		},
		{
			name:     "returns an error if the receiver has exceeded their max attempts to confirm the verification value",
			receiver: *receiverWithExceededAttempts,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiverWithExceededAttempts.PhoneNumber,
				VerificationField: data.VerificationTypeDateOfBirth,
				VerificationValue: "1990-01-01",
			},
			wantErrContains: "the number of attempts to confirm the verification value exceeded the max attempts",
		},
		{
			name:     "returns an error if the varification value provided in the payload is different from the DB one",
			receiver: *receiver,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiver.PhoneNumber,
				VerificationField: data.VerificationTypeDateOfBirth,
				VerificationValue: "1990-11-11", // <--- different from the DB one (1990-01-01)
			},
			shouldAssertAttemptsCount: true,
			wantErrContains:           "DATE_OF_BIRTH value does not match for receiver with id " + receiver.ID,
		},
		{
			name:     "🎉 successfully process the verification value and updates it accordingly in the DB",
			receiver: *receiver,
			registrationRequest: data.ReceiverRegistrationRequest{
				PhoneNumber:       receiver.PhoneNumber,
				VerificationField: data.VerificationTypeDateOfBirth,
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
				receiverVerifications, err = models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{tc.receiver.ID}, tc.registrationRequest.VerificationField)
				require.NoError(t, err)
				require.Len(t, receiverVerifications, 1)
				receiverVerificationInitial = receiverVerifications[0]
			}

			err = handler.processReceiverVerificationPII(ctx, dbTx, tc.receiver, tc.registrationRequest)

			if tc.wantErrContains == "" {
				receiverVerifications, err = models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{tc.receiver.ID}, tc.registrationRequest.VerificationField)
				require.NoError(t, err)
				require.Len(t, receiverVerifications, 1)
				receiverVerification := receiverVerifications[0]
				assert.NotEmpty(t, receiverVerification.ConfirmedAt)
				assert.Equal(t, receiverVerificationInitial.Attempts, receiverVerification.Attempts, "attempts should not have been incremented")
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
				if tc.shouldAssertAttemptsCount {
					receiverVerifications, err = models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{tc.receiver.ID}, tc.registrationRequest.VerificationField)
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
			wantErrContains:             []string{ErrOTPDoesNotMatch.Error()},
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
			t.Cleanup(func() {
				data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
				data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
			})
			// OTP
			const correctOTP = "123456"
			const wrongOTP = "111111"
			otp := correctOTP
			if !tc.shouldOTPMatch {
				otp = wrongOTP
			}
			receiverEmail := "test@stellar.org"

			// receiver & receiver wallet
			receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: "+380445555555"})
			var receiverWallet *data.ReceiverWallet
			if tc.currentReceiverWalletStatus.State() != "" {
				receiverWallet = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, tc.currentReceiverWalletStatus)
				var stellarAddress string
				var otpConfirmedAt *time.Time
				var otpConfirmedWith string
				if tc.wantWasAlreadyRegistered {
					stellarAddress = "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444"
					now := time.Now()
					otpConfirmedAt = &now
					otpConfirmedWith = receiverEmail
				}

				const q = `
					UPDATE receiver_wallets
					SET otp = $1, otp_created_at = NOW(), stellar_address = $2, otp_confirmed_at = $3, otp_confirmed_with = $4
					WHERE id = $5
				`
				_, err = dbConnectionPool.ExecContext(ctx, q, correctOTP, sql.NullString{String: stellarAddress, Valid: stellarAddress != ""}, otpConfirmedAt, otpConfirmedWith, receiverWallet.ID)
				require.NoError(t, err)
			}

			// assertions
			dbTx := testutils.BeginTxWithRollback(t, ctx, dbConnectionPool)
			rwUpdated, wasAlreadyRegistered, err := handler.processReceiverWalletOTP(ctx, dbTx, *tc.sep24Claims, *receiver, otp, receiverEmail)
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
				assert.Equal(t, rwUpdated.OTPConfirmedWith, receiverEmail)
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
	phoneNumber := "+380445555555"
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

func Test_VerifyReceiverRegistrationHandler_buildPaymentsReadyToPayEventMessage(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tnt := schema.Tenant{ID: "tenant-id"}
	ctx := context.Background()
	ctx = sdpcontext.SetTenantInContext(ctx, &tnt)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	data.DeleteAllFixtures(t, ctx, dbConnectionPool)

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet", "https://home.page", "home.page", "wallet123://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	rw := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	t.Run("doesn't return error when there's no payment", func(t *testing.T) {
		defer data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		handler := VerifyReceiverRegistrationHandler{
			Models: models,
		}

		pausedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Wallet: wallet,
			Asset:  asset,
			Status: data.PausedDisbursementStatus,
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "100",
			Status:         data.PausedPaymentStatus,
			Disbursement:   pausedDisbursement,
			Asset:          *asset,
			ReceiverWallet: rw,
		})

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		msg, err := handler.buildPaymentsReadyToPayEventMessage(ctx, dbConnectionPool, rw)
		assert.NoError(t, err)
		assert.Nil(t, msg)

		entries := getEntries()
		assert.Len(t, entries, 1)
		assert.Equal(t, fmt.Sprintf("no payments ready to pay for receiver wallet ID %s", rw.ID), entries[0].Message)
	})

	t.Run("returns error when tenant isn't in the context", func(t *testing.T) {
		defer data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
		ctxWithoutTenant := context.Background()
		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distAccountResolverMock.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
			Once()
		handler := VerifyReceiverRegistrationHandler{
			Models:                      models,
			DistributionAccountResolver: distAccountResolverMock,
		}

		disbursement := data.CreateDisbursementFixture(t, ctxWithoutTenant, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Wallet: wallet,
			Asset:  asset,
			Status: data.StartedDisbursementStatus,
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw,
		})

		msg, err := handler.buildPaymentsReadyToPayEventMessage(ctxWithoutTenant, dbConnectionPool, rw)
		assert.EqualError(t, err, "creating new message: getting tenant from context: tenant not found in context")
		assert.Nil(t, msg)
	})

	t.Run("🎉 successfully builds the message for stellar payment", func(t *testing.T) {
		defer data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distAccountResolverMock.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
			Once()
		handler := VerifyReceiverRegistrationHandler{
			Models:                      models,
			DistributionAccountResolver: distAccountResolverMock,
		}

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Wallet: wallet,
			Asset:  asset,
			Status: data.StartedDisbursementStatus,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw,
		})

		expectedMessage := events.Message{
			Topic:    events.PaymentReadyToPayTopic,
			Key:      rw.ID,
			TenantID: tnt.ID,
			Type:     events.PaymentReadyToPayReceiverVerificationCompleted,
			Data: schemas.EventPaymentsReadyToPayData{
				TenantID: tnt.ID,
				Payments: []schemas.PaymentReadyToPay{
					{
						ID: payment.ID,
					},
				},
			},
		}

		msg, err := handler.buildPaymentsReadyToPayEventMessage(ctx, dbConnectionPool, rw)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, *msg)
	})

	t.Run("🎉 successfully builds the message for circle payment", func(t *testing.T) {
		defer data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

		distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
		distAccountResolverMock.
			On("DistributionAccountFromContext", mock.Anything).
			Return(schema.TransactionAccount{Type: schema.DistributionAccountCircleDBVault}, nil).
			Once()
		handler := VerifyReceiverRegistrationHandler{
			Models:                      models,
			DistributionAccountResolver: distAccountResolverMock,
		}

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Wallet: wallet,
			Asset:  asset,
			Status: data.StartedDisbursementStatus,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:         "100",
			Status:         data.ReadyPaymentStatus,
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: rw,
		})

		expectedMessage := events.Message{
			Topic:    events.CirclePaymentReadyToPayTopic,
			Key:      rw.ID,
			TenantID: tnt.ID,
			Type:     events.PaymentReadyToPayReceiverVerificationCompleted,
			Data: schemas.EventPaymentsReadyToPayData{
				TenantID: tnt.ID,
				Payments: []schemas.PaymentReadyToPay{
					{
						ID: payment.ID,
					},
				},
			},
		}

		msg, err := handler.buildPaymentsReadyToPayEventMessage(ctx, dbConnectionPool, rw)
		assert.NoError(t, err)
		assert.Equal(t, expectedMessage, *msg)
	})
}

func Test_VerifyReceiverRegistrationHandler_VerifyReceiverRegistration(t *testing.T) {
	ctx := context.Background()
	models := data.SetupModels(t)
	dbConnectionPool := models.DBConnectionPool

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

	phoneNumber := "+380445555555"
	receiverRegistrationRequestWithPhone := data.ReceiverRegistrationRequest{
		PhoneNumber:       phoneNumber,
		OTP:               "123456",
		VerificationValue: "1990-01-01",
		VerificationField: "date_of_birth",
		ReCAPTCHAToken:    "token",
	}
	reqBody, outerErr := json.Marshal(receiverRegistrationRequestWithPhone)
	require.NoError(t, outerErr)

	email := "test@stellar.org"
	receiverRegistrationRequestWithEmail := data.ReceiverRegistrationRequest{
		Email:             email,
		OTP:               "123456",
		VerificationValue: "1990-01-01",
		VerificationField: "date_of_birth",
		ReCAPTCHAToken:    "token",
	}
	reqBodyEmail, outerErr := json.Marshal(receiverRegistrationRequestWithEmail)
	require.NoError(t, outerErr)

	r := chi.NewRouter()

	t.Run("returns an error when validate() fails - testing case where a SEP24 claims are missing from the context", func(t *testing.T) {
		handler := &VerifyReceiverRegistrationHandler{Models: models}

		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, reqErr := http.NewRequest("POST", "/wallet-registration/verification", nil)
		require.NoError(t, reqErr)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, readRespErr := io.ReadAll(resp.Body)
		require.NoError(t, readRespErr)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized.", "error_code": "401_0"}`, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "validating request in VerifyReceiverRegistrationHandler: no SEP-24 claims found in the request context")
	})

	t.Run("returns an error if the receiver cannot be found", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:             models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
		}

		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, reqErr := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, reqErr)
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, readRespErr := io.ReadAll(resp.Body)
		require.NoError(t, readRespErr)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		wantBody := fmt.Sprintf(`{"error": "%s", "error_code": "400_2"}`, InformationNotFoundOnServer)
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), "receiver with contact info +38...555 not found in our server")
	})

	t.Run("returns an error when processReceiverVerificationPII() fails - testing case where no receiverVerification is found", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:             models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
		}

		// update database with the entries needed
		t.Cleanup(func() {
			data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		})
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, reqErr := models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, phoneNumber, wallet.SEP10ClientDomain, receiverRegistrationRequestWithPhone.OTP)
		require.NoError(t, reqErr)

		// set the logger to a buffer so we can check the error message
		buf := new(strings.Builder)
		log.DefaultLogger.SetOutput(buf)

		// setup router and execute request
		r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
		req, reqErr := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBody)))
		require.NoError(t, reqErr)
		req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, validClaims))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// execute and validate response
		resp := rr.Result()
		respBody, readRespErr := io.ReadAll(resp.Body)
		require.NoError(t, readRespErr)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		wantBody := fmt.Sprintf(`{"error": "%s", "error_code": "400_2"}`, InformationNotFoundOnServer)
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		expectedErr := `processing receiver verification entry for receiver with contact info +38...555: verification of type %s not found for receiver id %s`
		require.Contains(t, buf.String(), fmt.Sprintf(expectedErr, data.VerificationTypeDateOfBirth, receiver.ID))
	})

	t.Run("returns an error when processReceiverVerificationPII() fails - testing case where maximum number of verification attempts exceeded", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:             models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
		}

		t.Cleanup(func() {
			data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		})

		// receiver with a receiverVerification row that's exceeded the maximum number of attempts:
		receiverWithExceededAttempts := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverWithExceededAttempts.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, reqErr := models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, phoneNumber, wallet.SEP10ClientDomain, receiverRegistrationRequestWithPhone.OTP)
		require.NoError(t, reqErr)
		receiverVerificationExceededAttempts := data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiverWithExceededAttempts.ID,
			VerificationField: data.VerificationTypeDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		receiverVerificationExceededAttempts.Attempts = data.MaxAttemptsAllowed
		err := models.ReceiverVerification.UpdateReceiverVerification(ctx, data.ReceiverVerificationUpdate{
			ReceiverID:          receiverWithExceededAttempts.ID,
			VerificationField:   data.VerificationTypeDateOfBirth,
			Attempts:            utils.IntPtr(data.MaxAttemptsAllowed),
			VerificationChannel: message.MessageChannelSMS,
		}, dbConnectionPool)
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
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		expectedError := "the number of attempts to confirm the verification value exceeded the max attempts"
		wantBody := fmt.Sprintf(`{"error": "%s", "error_code": "400_3"}`, expectedError)
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		require.Contains(t, buf.String(), expectedError)
	})

	t.Run("returns an error when processReceiverWalletOTP() fails - testing case where no receiverWallet is found", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:             models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
		}

		// update database with the entries needed
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationTypeDateOfBirth,
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
		wantBody := fmt.Sprintf(`{"error": "%s", "error_code": "400_2"}`, InformationNotFoundOnServer)
		assert.JSONEq(t, wantBody, string(respBody))

		// validate logs
		wantErrContains := fmt.Sprintf("processing OTP for receiver with contact info +38...555: receiver wallet not found for receiverID=%s and clientDomain=home.page", receiver.ID)
		require.Contains(t, buf.String(), wantErrContains)
	})

	t.Run("returns an error when processAnchorPlatformID() fails - anchor platform returns an error", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
			ID:     "test-transaction-id",
			Status: "pending_anchor",
			SEP:    "24",
		}
		mockAnchorPlatformService := &anchorplatform.AnchorPlatformAPIServiceMock{}
		defer mockAnchorPlatformService.AssertExpectations(t)
		mockAnchorPlatformService.
			On("PatchAnchorTransactionsPostRegistration", mock.Anything, apTxPatch).
			Return(fmt.Errorf("error updating transaction on anchor platform")).Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:                   models,
			ReCAPTCHAValidator:       reCAPTCHAValidator,
			AnchorPlatformAPIService: mockAnchorPlatformService,
		}

		// update database with the entries needed
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationTypeDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		_ = data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
		_, err := models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
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
		wantBody := `{"error": "An internal error occurred while processing this request.", "error_code": "500_0"}`
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
				reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
				reCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Maybe()

				apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
					ID:     "test-transaction-id",
					Status: "pending_anchor",
					SEP:    "24",
				}
				mockAnchorPlatformService := &anchorplatform.AnchorPlatformAPIServiceMock{}
				defer mockAnchorPlatformService.AssertExpectations(t)
				mockAnchorPlatformService.On("PatchAnchorTransactionsPostRegistration", mock.Anything, apTxPatch).Return(nil).Once()

				// create handler
				handler := &VerifyReceiverRegistrationHandler{
					Models:                   models,
					ReCAPTCHAValidator:       reCAPTCHAValidator,
					AnchorPlatformAPIService: mockAnchorPlatformService,
				}

				// update database with the entries needed
				defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
				_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: data.VerificationTypeDateOfBirth,
					VerificationValue: "1990-01-01",
				})
				receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
				_, err := models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
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
						COALESCE(rw.stellar_memo_type::text, '') as "stellar_memo_type",
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
					assert.Equal(t, schema.MemoTypeID, receiverWalletUpdated.StellarMemoType)
				}
			})
		}
	})

	t.Run("🎉 successfully registers a second wallet in the same address", func(t *testing.T) {
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

		// create second valid wallet
		wallet2 := data.CreateWalletFixture(t, ctx, dbConnectionPool, "testWallet2", "https://wallet2.page", "wallet2.page", "wallet2://")

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				sep24Claims := *validClaims
				sep24Claims.RegisteredClaims.Subject += ":" + tc.inputMemo

				// mocks
				reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
				reCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Maybe()

				apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
					ID:     "test-transaction-id",
					Status: "pending_anchor",
					SEP:    "24",
				}
				mockAnchorPlatformService := &anchorplatform.AnchorPlatformAPIServiceMock{}
				defer mockAnchorPlatformService.AssertExpectations(t)
				mockAnchorPlatformService.On("PatchAnchorTransactionsPostRegistration", mock.Anything, apTxPatch).Return(nil)

				// create handler
				handler := &VerifyReceiverRegistrationHandler{
					Models:                   models,
					ReCAPTCHAValidator:       reCAPTCHAValidator,
					AnchorPlatformAPIService: mockAnchorPlatformService,
				}

				// update database with the entries needed
				defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
				receiver := data.InsertReceiverFixture(t, ctx, dbConnectionPool, &data.ReceiverInsert{Email: &email})
				_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: data.VerificationTypeDateOfBirth,
					VerificationValue: "1990-01-01",
				})
				receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
				_, err := models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, email, wallet.SEP10ClientDomain, "123456")
				require.NoError(t, err)

				// setup router and execute request
				r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
				req, err := http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBodyEmail)))
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
						COALESCE(rw.stellar_memo_type::text, '') as "stellar_memo_type",
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
					assert.Equal(t, schema.MemoTypeID, receiverWalletUpdated.StellarMemoType)
				}

				// registering Second Wallet
				receiverWallet2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet2.ID, data.ReadyReceiversWalletStatus)
				_, err = models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, email, wallet2.SEP10ClientDomain, "123456")
				require.NoError(t, err)

				sep24Claims.ClientDomainClaim = wallet2.SEP10ClientDomain

				req, err = http.NewRequest("POST", "/wallet-registration/verification", strings.NewReader(string(reqBodyEmail)))
				require.NoError(t, err)
				req = req.WithContext(context.WithValue(req.Context(), anchorplatform.SEP24ClaimsContextKey, &sep24Claims))
				rr = httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				// execute and validate response
				resp = rr.Result()
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)

				err = dbConnectionPool.GetContext(ctx, &receiverWalletUpdated, query, receiverWallet2.ID)
				require.NoError(t, err)

				assert.Equal(t, data.RegisteredReceiversWalletStatus, receiverWalletUpdated.Status)
				assert.Equal(t, "GBLTXF46JTCGMWFJASQLVXMMA36IPYTDCN4EN73HRXCGDCGYBZM3A444", receiverWalletUpdated.StellarAddress)
				require.NotEmpty(t, receiverWalletUpdated.OTPConfirmedAt)
				if tc.inputMemo == "" {
					assert.Empty(t, receiverWalletUpdated.StellarMemo)
					assert.Empty(t, receiverWalletUpdated.StellarMemoType)
				} else {
					assert.Equal(t, tc.inputMemo, receiverWalletUpdated.StellarMemo)
					assert.Equal(t, schema.MemoTypeID, receiverWalletUpdated.StellarMemoType)
				}
			})
		}
	})

	t.Run("returns OTP max attempts exceeded error", func(t *testing.T) {
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:             models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
			NetworkPassphrase:  network.TestNetworkPassphrase,
		}

		// update database with the entries needed
		t.Cleanup(func() {
			data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
			data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		})

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationTypeDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		// Set OTP attempts to max
		_, err := dbConnectionPool.ExecContext(ctx,
			"UPDATE receiver_wallets SET otp = $1, otp_created_at = NOW(), otp_attempts = $2 WHERE id = $3",
			"123456", OTPMaxAttempts, receiverWallet.ID)
		require.NoError(t, err)

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
		wantBody := `{"error": "Maximum OTP verification attempts exceeded. Please request a new OTP.", "error_code": "400_4"}`
		assert.JSONEq(t, wantBody, string(respBody))
	})

	t.Run("returns OTP expired error", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:             models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
			NetworkPassphrase:  network.TestNetworkPassphrase,
		}

		// update database with the entries needed
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationTypeDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		// Set OTP with expired created_at time
		expiredTime := time.Now().Add(-OTPExpirationTimeMinutes*time.Minute - time.Minute)
		_, err := dbConnectionPool.ExecContext(ctx,
			"UPDATE receiver_wallets SET otp = $1, otp_created_at = $2, otp_attempts = 0 WHERE id = $3",
			"123456", expiredTime, receiverWallet.ID)
		require.NoError(t, err)

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
		wantBody := `{"error": "OTP has expired. Please request a new one.", "error_code": "400_5"}`
		assert.JSONEq(t, wantBody, string(respBody))
	})

	t.Run("returns OTP does not match error", func(t *testing.T) {
		// mocks
		reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
		reCAPTCHAValidator.
			On("IsTokenValid", mock.Anything, "token").
			Return(true, nil).
			Once()

		// create handler
		handler := &VerifyReceiverRegistrationHandler{
			Models:             models,
			ReCAPTCHAValidator: reCAPTCHAValidator,
			NetworkPassphrase:  network.TestNetworkPassphrase,
		}

		// update database with the entries needed
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
		_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
			ReceiverID:        receiver.ID,
			VerificationField: data.VerificationTypeDateOfBirth,
			VerificationValue: "1990-01-01",
		})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		// Set OTP to different value than in request
		_, err := dbConnectionPool.ExecContext(ctx,
			"UPDATE receiver_wallets SET otp = $1, otp_created_at = NOW(), otp_attempts = 0 WHERE id = $2",
			"622141", receiverWallet.ID) // Different from "123456" in reqBody
		require.NoError(t, err)

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
		wantBody := `{"error": "Invalid OTP. Please check and try again.", "error_code": "400_6"}`
		assert.JSONEq(t, wantBody, string(respBody))

		// Verify OTP attempts were incremented in database
		var otpAttempts int
		err = dbConnectionPool.GetContext(ctx, &otpAttempts, "SELECT otp_attempts FROM receiver_wallets WHERE id = $1", receiverWallet.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, otpAttempts)
	})

	t.Run("🎉 successfully register receiver's stellar address and produce event", func(t *testing.T) {
		testCases := []struct {
			name                       string
			produccesEventSuccessfully bool
		}{
			{
				name:                       "produces event successfully",
				produccesEventSuccessfully: true,
			},
			{
				name:                       "fails to produce event",
				produccesEventSuccessfully: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tnt := schema.Tenant{ID: "tenant-id"}
				ctx = sdpcontext.SetTenantInContext(ctx, &tnt)

				// update database with the entries needed
				defer data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
				defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)

				receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{PhoneNumber: phoneNumber})
				_ = data.CreateReceiverVerificationFixture(t, ctx, dbConnectionPool, data.ReceiverVerificationInsert{
					ReceiverID:        receiver.ID,
					VerificationField: data.VerificationTypeDateOfBirth,
					VerificationValue: "1990-01-01",
				})
				receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
				_, err := models.ReceiverWallet.UpdateOTPByReceiverContactInfoAndWalletDomain(ctx, "+380445555555", wallet.SEP10ClientDomain, "123456")
				require.NoError(t, err)

				// Creating a payment ready to pay
				asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
				disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
					Wallet: wallet,
					Asset:  asset,
					Status: data.StartedDisbursementStatus,
				})
				payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
					Amount:         "100",
					Status:         data.ReadyPaymentStatus,
					Disbursement:   disbursement,
					Asset:          *asset,
					ReceiverWallet: receiverWallet,
				})

				sep24Claims := *validClaims

				// mocks
				reCAPTCHAValidator := validators.NewReCAPTCHAValidatorMock(t)
				reCAPTCHAValidator.
					On("IsTokenValid", mock.Anything, "token").
					Return(true, nil).
					Once()

				apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
					ID:     "test-transaction-id",
					Status: "pending_anchor",
					SEP:    "24",
				}
				mockAnchorPlatformService := &anchorplatform.AnchorPlatformAPIServiceMock{}
				defer mockAnchorPlatformService.AssertExpectations(t)
				mockAnchorPlatformService.On("PatchAnchorTransactionsPostRegistration", mock.Anything, apTxPatch).Return(nil).Once()

				mockCrashTracker := &crashtracker.MockCrashTrackerClient{}
				defer mockCrashTracker.AssertExpectations(t)
				mockEventProducer := events.NewMockProducer(t)

				distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)
				distAccountResolverMock.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{Type: schema.DistributionAccountStellarEnv}, nil).
					Maybe()

				if tc.produccesEventSuccessfully {
					mockEventProducer.
						On("WriteMessages", mock.Anything, []events.Message{
							{
								Topic:    events.PaymentReadyToPayTopic,
								Key:      receiverWallet.ID,
								TenantID: tnt.ID,
								Type:     events.PaymentReadyToPayReceiverVerificationCompleted,
								Data: schemas.EventPaymentsReadyToPayData{
									TenantID: tnt.ID,
									Payments: []schemas.PaymentReadyToPay{{ID: payment.ID}},
								},
							},
						}).
						Return(nil).
						Once()
				} else {
					mockEventProducer.
						On("WriteMessages", mock.Anything, mock.AnythingOfType("[]events.Message")).
						Return(errors.New("FOO BAR")).
						Once()
					mockCrashTracker.
						On("LogAndReportErrors", mock.Anything, mock.Anything, "writing ready-to-pay message (post SEP-24) on the event producer").
						Return(nil).
						Once()
				}

				// create handler
				handler := &VerifyReceiverRegistrationHandler{
					Models:                      models,
					ReCAPTCHAValidator:          reCAPTCHAValidator,
					AnchorPlatformAPIService:    mockAnchorPlatformService,
					EventProducer:               mockEventProducer,
					CrashTrackerClient:          mockCrashTracker,
					DistributionAccountResolver: distAccountResolverMock,
				}

				// setup router and execute request
				r.Post("/wallet-registration/verification", handler.VerifyReceiverRegistration)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/wallet-registration/verification", strings.NewReader(string(reqBody)))
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
					COALESCE(rw.stellar_memo_type::text, '') as "stellar_memo_type",
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
			})
		}
	})
}

func Test_verifyReceiverWalletOTP(t *testing.T) {
	ctx := context.Background()

	expiredOTPCreatedAt := time.Now().Add(-OTPExpirationTimeMinutes * time.Minute).Add(-time.Second) // expired 1 second ago
	validOTPTime := time.Now()

	testCases := []struct {
		name              string
		networkPassphrase string
		attemptedOTP      string
		otp               string
		otpCreatedAt      time.Time
		otpAttempts       int
		wantErr           error
	}{
		// mismatching OTP fails:
		{
			name:              "mismatching OTP fails",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123123",
			otp:               "123456",
			otpCreatedAt:      validOTPTime,
			wantErr:           ErrOTPDoesNotMatch,
		},
		{
			name:              "mismatching OTP fails when passing the TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      validOTPTime,
			wantErr:           ErrOTPDoesNotMatch,
		},
		{
			name:              "mismatching OTP succeeds when passing the TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},

		// matching OTP fails when its created_at date is invalid:
		{
			name:              "matching OTP fails when its created_at date is invalid",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123456",
			otp:               "123456",
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           fmt.Errorf("otp does not have a valid created_at time"),
		},
		{
			name:              "matching OTP fails when its created_at date is invalid and we pass TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               data.TestnetAlwaysValidOTP,
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           fmt.Errorf("otp does not have a valid created_at time"),
		},
		{
			name:              "matching OTP succeeds when its created_at date is invalid but we pass TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      time.Time{}, // invalid created_at
			wantErr:           nil,
		},

		// returns error when otp is expired:
		{
			name:              "matching OTP fails when OTP is expired",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123456",
			otp:               "123456",
			otpCreatedAt:      expiredOTPCreatedAt,
			wantErr:           ErrOTPExpired,
		},
		{
			name:              "matching OTP fails when OTP is expired and we pass TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               data.TestnetAlwaysValidOTP,
			otpCreatedAt:      expiredOTPCreatedAt,
			wantErr:           ErrOTPExpired,
		},
		{
			name:              "matching OTP fails when OTP is expired but we pass TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      expiredOTPCreatedAt,
			wantErr:           nil,
		},
		{
			name:              "matching OTP fails when OTP is expired but we pass TestnetAlwaysValidOTP in Futurenet",
			networkPassphrase: network.FutureNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               "123456",
			otpCreatedAt:      expiredOTPCreatedAt,
			wantErr:           nil,
		},

		// returns error when otp attempts exceeded:
		{
			name:              "matching OTP fails when OTP attempts exceeded",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123456",
			otp:               "123456",
			otpCreatedAt:      validOTPTime,
			otpAttempts:       OTPMaxAttempts,
			wantErr:           ErrOTPMaxAttemptsExceeded,
		},

		// OTP is valid 🎉
		{
			name:              "OTP is valid 🎉",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      "123456",
			otp:               "123456",
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},
		{
			name:              "OTP is valid 🎉 also when we pass TestnetAlwaysValidOTP in Pubnet",
			networkPassphrase: network.PublicNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               data.TestnetAlwaysValidOTP,
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},
		{
			name:              "OTP is valid 🎉 also when we pass TestnetAlwaysValidOTP in Testnet",
			networkPassphrase: network.TestNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               data.TestnetAlwaysValidOTP,
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},
		{
			name:              "OTP is valid 🎉 also when we pass TestnetAlwaysValidOTP in Futurenet",
			networkPassphrase: network.FutureNetworkPassphrase,
			attemptedOTP:      data.TestnetAlwaysValidOTP,
			otp:               data.TestnetAlwaysValidOTP,
			otpCreatedAt:      validOTPTime,
			wantErr:           nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			receiverWallet := data.ReceiverWallet{
				OTP:          tc.otp,
				OTPCreatedAt: &tc.otpCreatedAt,
				OTPAttempts:  tc.otpAttempts,
			}

			err := verifyReceiverWalletOTP(ctx, tc.networkPassphrase, receiverWallet, tc.attemptedOTP)
			if tc.wantErr != nil {
				assert.Equal(t, tc.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
