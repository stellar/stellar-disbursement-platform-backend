package httphandler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sepauth"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

// InformationNotFoundError implements the error interface.
type InformationNotFoundError struct {
	cause error
}

func (e *InformationNotFoundError) Error() string {
	return e.cause.Error()
}

type VerificationAttemptsExceededError struct {
	cause error
}

func (e *VerificationAttemptsExceededError) Error() string {
	return e.cause.Error()
}

const (
	OTPExpirationTimeMinutes    = 5
	OTPMaxAttempts              = 5
	InformationNotFoundOnServer = "The information you provided could not be found"
)

var (
	ErrOTPMaxAttemptsExceeded = errors.New("the number of attempts to confirm the OTP exceeded the max attempts")
	ErrOTPExpired             = errors.New("the OTP is expired, please request a new one")
	ErrOTPDoesNotMatch        = errors.New("the OTP does not match the one saved in the database")
)

type VerifyReceiverRegistrationHandler struct {
	Models                      *data.Models
	ReCAPTCHAValidator          validators.ReCAPTCHAValidator
	ReCAPTCHADisabled           bool
	NetworkPassphrase           string
	CrashTrackerClient          crashtracker.CrashTrackerClient
	DistributionAccountResolver signing.DistributionAccountResolver
}

// validate validates the request [header, body, body.reCAPTCHA_token], and returns the decoded payload, or an http error.
func (v VerifyReceiverRegistrationHandler) validate(r *http.Request) (reqObj data.ReceiverRegistrationRequest, sep24Claims *sepauth.SEP24JWTClaims, httpErr *httperror.HTTPError) {
	ctx := r.Context()

	// STEP 1: Validate SEP-24 JWT token
	sep24Claims = sepauth.GetSEP24Claims(ctx)
	if sep24Claims == nil {
		err := fmt.Errorf("no SEP-24 claims found in the request context")
		log.Ctx(ctx).Error(err)
		return reqObj, nil, httperror.Unauthorized("", err, nil).WithErrorCode(httperror.Code401_0)
	}

	// STEP 2: Decode request body
	if r.Body == nil {
		err := fmt.Errorf("request body is empty")
		return reqObj, nil, httperror.BadRequest("", err, nil).WithErrorCode(httperror.Code400_0)
	}
	receiverRegistrationRequest := data.ReceiverRegistrationRequest{}
	err := json.NewDecoder(r.Body).Decode(&receiverRegistrationRequest)
	if err != nil {
		err = fmt.Errorf("invalid request body: %w", err)
		return reqObj, nil, httperror.BadRequest("", err, nil).WithErrorCode(httperror.Code400_0)
	}

	// STEP 3: Validate reCAPTCHA Token
	if !IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            v.Models,
		ReCAPTCHADisabled: v.ReCAPTCHADisabled,
	}) {
		isValid, tokenErr := v.ReCAPTCHAValidator.IsTokenValid(ctx, receiverRegistrationRequest.ReCAPTCHAToken)
		if tokenErr != nil {
			tokenErr = fmt.Errorf("validating reCAPTCHA token: %w", tokenErr)
			return reqObj, nil, httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", tokenErr, nil).WithErrorCode(httperror.Code500_5)
		}
		if !isValid {
			truncatedPhoneNumber := utils.TruncateString(receiverRegistrationRequest.PhoneNumber, 3)
			truncatedOTP := utils.TruncateString(receiverRegistrationRequest.OTP, 2)
			err = fmt.Errorf("reCAPTCHA token is invalid for request with OTP %s and Phone Number %s", truncatedOTP, truncatedPhoneNumber)
			return reqObj, nil, httperror.BadRequest("reCAPTCHA token is invalid", err, nil).WithErrorCode(httperror.Code400_1)
		}
	}

	// STEP 4: Validate request body
	validator := validators.NewReceiverRegistrationValidator()
	validator.ValidateReceiver(&receiverRegistrationRequest)
	if validator.HasErrors() {
		err = fmt.Errorf("request invalid: %s", validator.Errors)
		return reqObj, nil, httperror.BadRequest("", err, validator.Errors).
			WithErrorCode(httperror.Code400_0).
			WithExtrasCodes(validator.ErrorCodes)
	}

	return receiverRegistrationRequest, sep24Claims, nil
}

// processReceiverVerificationPII processes the receiver verification entry to make sure the verification value
// provided matches the one saved in the database for the given user (phone number). It returns an error if:
// - there is no receiver verification entry for the given receiverID and verificationType
// - the number of attempts to confirm the verification value has already exceeded the max value
// - the payload verification value does not match the one saved in the database
func (v VerifyReceiverRegistrationHandler) processReceiverVerificationPII(
	ctx context.Context,
	dbTx db.DBTransaction,
	receiver data.Receiver,
	receiverRegistrationRequest data.ReceiverRegistrationRequest,
) error {
	now := time.Now()

	// STEP 1: find the receiverVerification entry that matches the pair [receiverID, verificationType]
	receiverVerifications, err := v.Models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{receiver.ID}, receiverRegistrationRequest.VerificationField)
	if err != nil {
		return fmt.Errorf("retrieving receiver verification for verification type %s: %w", receiverRegistrationRequest.VerificationField, err)
	}
	if len(receiverVerifications) == 0 {
		err = fmt.Errorf("verification of type %s not found for receiver id %s", receiverRegistrationRequest.VerificationField, receiver.ID)
		return &InformationNotFoundError{cause: err}
	}
	if len(receiverVerifications) > 1 {
		log.Ctx(ctx).Warnf("receiver with id %s has more than one verification saved in the database for type %s", receiver.ID, receiverRegistrationRequest.VerificationField)
	}
	receiverVerification := receiverVerifications[0]

	// STEP 2: check if the number of attempts to confirm the verification value has already exceeded the max value
	if v.Models.ReceiverVerification.ExceededAttempts(receiverVerification.Attempts) {
		// TODO: the application currently can't recover from a max attempts exceeded error.
		err = fmt.Errorf("the number of attempts to confirm the verification value exceeded the max attempts")
		return &VerificationAttemptsExceededError{cause: err}
	}

	// STEP 3: check if the payload verification value matches the one saved in the database
	rvu := data.ReceiverVerificationUpdate{
		ReceiverID:        receiverVerification.ReceiverID,
		VerificationField: receiverVerification.VerificationField,
	}

	if strings.TrimSpace(receiverRegistrationRequest.PhoneNumber) != "" {
		rvu.VerificationChannel = message.MessageChannelSMS
	} else if strings.TrimSpace(receiverRegistrationRequest.Email) != "" {
		rvu.VerificationChannel = message.MessageChannelEmail
	} else {
		err = fmt.Errorf("no valid verification channel found resolved for receiver")
		return &InformationNotFoundError{cause: err}
	}

	if !data.CompareVerificationValue(receiverVerification.HashedValue, receiverRegistrationRequest.VerificationValue) {
		baseErrMsg := fmt.Sprintf("%s value does not match for receiver with id %s", receiverRegistrationRequest.VerificationField, receiver.ID)
		// update the receiver verification with the confirmation that the value was checked
		rvu.Attempts = utils.IntPtr(receiverVerification.Attempts + 1)
		rvu.FailedAt = &now

		// this update is done using the DBConnectionPool and not dbTx because we don't want to rollback these changes after returning the error
		updateErr := v.Models.ReceiverVerification.UpdateReceiverVerification(ctx, rvu, v.Models.DBConnectionPool)
		if updateErr != nil {
			err = fmt.Errorf("%s: %w", baseErrMsg, updateErr)
		} else {
			err = fmt.Errorf("%s", baseErrMsg)
		}

		return &InformationNotFoundError{cause: err}
	}

	// STEP 4: update the receiver verification row with the confirmation that the value was successfully validated
	rvu.ConfirmedAt = &now
	rvu.ConfirmedByID = receiver.ID
	rvu.ConfirmedByType = data.ConfirmedByTypeReceiver

	err = v.Models.ReceiverVerification.UpdateReceiverVerification(ctx, rvu, dbTx)
	if err != nil {
		return fmt.Errorf("updating successfully verified user: %w", err)
	}

	return nil
}

// processReceiverWalletOTP processes the OTP provided by the user and updates the receiver wallet status to "REGISTERED" if the OTP is valid.
func (v VerifyReceiverRegistrationHandler) processReceiverWalletOTP(
	ctx context.Context,
	dbTx db.DBTransaction,
	sep24Claims sepauth.SEP24JWTClaims,
	receiver data.Receiver, otp string,
	contactInfo string,
) (receiverWallet data.ReceiverWallet, wasAlreadyRegistered bool, err error) {
	// STEP 1: find the receiver wallet for the given [receiverID, clientDomain]
	rw, err := v.Models.ReceiverWallet.GetByReceiverIDAndWalletDomain(ctx, receiver.ID, sep24Claims.ClientDomain(), dbTx)
	if err != nil {
		err = fmt.Errorf("receiver wallet not found for receiverID=%s and clientDomain=%s: %w", receiver.ID, sep24Claims.ClientDomain(), err)
		return receiverWallet, false, &InformationNotFoundError{cause: err}
	}

	// STEP 2: check if receiver wallet status is already "REGISTERED"
	if rw.Status == data.RegisteredReceiversWalletStatus {
		log.Ctx(ctx).Info("receiver already registered in the SDP")
		return *rw, true, nil
	}

	// STEP 3: check if receiver wallet status can be transitioned to "REGISTERED"
	err = rw.Status.TransitionTo(data.RegisteredReceiversWalletStatus)
	if err != nil {
		err = fmt.Errorf("transitioning status for receiverWallet[ID=%s]: %w", rw.ID, err)
		return receiverWallet, false, &InformationNotFoundError{cause: err}
	}

	// STEP 4: verify receiver wallet OTP
	if err = verifyReceiverWalletOTP(ctx, v.NetworkPassphrase, *rw, otp); err != nil {
		if errors.Is(err, ErrOTPDoesNotMatch) {
			log.Ctx(ctx).Errorf("receiver wallet OTP does not match for receiver wallet ID %s", rw.ID)
			return *rw, false, v.incrementOTPAttempts(ctx, rw)
		} else {
			err = fmt.Errorf("unable to verify receiver wallet OTP for receiver wallet ID %s: %w", rw.ID, err)
			return receiverWallet, false, fmt.Errorf("verifying receiver wallet OTP: %w", err)
		}
	}

	// STEP 5: update receiver wallet status to "REGISTERED"
	now := time.Now()
	rw.OTPConfirmedAt = &now
	rw.OTPConfirmedWith = contactInfo
	rw.Status = data.RegisteredReceiversWalletStatus
	rw.StellarAddress = sep24Claims.Account()
	rw.StellarMemo = sep24Claims.Memo()
	rw.StellarMemoType = ""
	if sep24Claims.Memo() != "" {
		rw.StellarMemoType = schema.MemoTypeID
	}
	err = v.Models.ReceiverWallet.Update(ctx, rw.ID, data.ReceiverWalletUpdate{
		Status:           rw.Status,
		StellarAddress:   rw.StellarAddress,
		StellarMemo:      &rw.StellarMemo,
		StellarMemoType:  &rw.StellarMemoType,
		OTPConfirmedAt:   now,
		OTPConfirmedWith: rw.OTPConfirmedWith,
	}, dbTx)
	if err != nil {
		err = fmt.Errorf("completing receiver wallet registration: %w", err)
		return receiverWallet, false, err
	}

	return *rw, false, nil
}

// processTransactionID patches the receiver wallet with the SEP-24 transaction ID.
func (v VerifyReceiverRegistrationHandler) processTransactionID(ctx context.Context, dbTx db.DBTransaction, sep24Claims sepauth.SEP24JWTClaims, receiverWallet data.ReceiverWallet) error {
	err := v.Models.ReceiverWallet.Update(ctx, receiverWallet.ID, data.ReceiverWalletUpdate{
		SEP24TransactionID: sep24Claims.TransactionID(),
	}, dbTx)
	if err != nil {
		return fmt.Errorf("updating receiver wallet with transaction ID: %w", err)
	}

	log.Ctx(ctx).Infof("Updated receiver wallet %s with SEP-24 transaction ID %s",
		receiverWallet.ID, sep24Claims.TransactionID())

	return nil
}

// VerifyReceiverRegistration is the handler for the SEP-24 `POST /wallet-registration/verification` endpoint. It is
// where the SDP verifies the receiver's PII & OTP, update the receiver wallet with the Stellar account and memo, found
// in the JWT token.
func (v VerifyReceiverRegistrationHandler) VerifyReceiverRegistration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// STEP 1: Validate request
	receiverRegistrationRequest, sep24Claims, httpErr := v.validate(r)
	if httpErr != nil {
		if httpErr.Err != nil {
			log.Ctx(ctx).Errorf("validating request in VerifyReceiverRegistrationHandler: %v", httpErr.Err)
		}
		httpErr.Render(w)
		return
	}

	var contactInfo string
	if receiverRegistrationRequest.PhoneNumber != "" {
		contactInfo = receiverRegistrationRequest.PhoneNumber
	} else if receiverRegistrationRequest.Email != "" {
		contactInfo = receiverRegistrationRequest.Email
	} else {
		httperror.InternalError(ctx, "Unexpected contact info", nil, nil).WithErrorCode(httperror.Code500_6).Render(w)
		return
	}

	truncatedContactInfo := utils.TruncateString(contactInfo, 3)

	err := db.RunInTransaction(ctx, v.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// STEP 2: find the receivers with the given phone number
		receivers, err := v.Models.Receiver.GetByContacts(ctx, dbTx, contactInfo)
		if err != nil {
			return fmt.Errorf("retrieving receiver with contact info %s: %w", truncatedContactInfo, err)
		}
		if len(receivers) == 0 {
			err = fmt.Errorf("receiver with contact info %s not found in our server", truncatedContactInfo)
			return &InformationNotFoundError{cause: err}
		}
		receiver := receivers[0]

		// STEP 3: process OTP
		receiverWallet, wasAlreadyRegistered, err := v.processReceiverWalletOTP(ctx, dbTx, *sep24Claims, *receiver, receiverRegistrationRequest.OTP, contactInfo)
		if err != nil {
			return fmt.Errorf("processing OTP for receiver with contact info %s: %w", truncatedContactInfo, err)
		}

		// STEP 4: process receiverVerification PII info that matches the pair [receiverID, verificationType]
		err = v.processReceiverVerificationPII(ctx, dbTx, *receiver, receiverRegistrationRequest)
		if err != nil {
			return fmt.Errorf("processing receiver verification entry for receiver with contact info %s: %w", truncatedContactInfo, err)
		}

		// STEP 5: PATCH transaction on the AnchorPlatform and update the receiver wallet with the anchor platform tx ID
		if !wasAlreadyRegistered {
			err = v.processTransactionID(ctx, dbTx, *sep24Claims, receiverWallet)
			if err != nil {
				return fmt.Errorf("processing transaction ID: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		var infoNotFoundErr *InformationNotFoundError
		// if error is due to verification attempts being exceeded, we want to display the message with what that limit is clearly
		// to the user
		var verficationAttemptsExceededErr *VerificationAttemptsExceededError

		switch {
		case errors.Is(err, ErrOTPMaxAttemptsExceeded):
			httperror.BadRequest("Maximum OTP verification attempts exceeded. Please request a new OTP.", err, nil).
				WithErrorCode(httperror.Code400_4).
				Render(w)
			return
		case errors.Is(err, ErrOTPExpired):
			httperror.BadRequest("OTP has expired. Please request a new one.", err, nil).
				WithErrorCode(httperror.Code400_5).
				Render(w)
			return
		case errors.Is(err, ErrOTPDoesNotMatch):
			httperror.BadRequest("Invalid OTP. Please check and try again.", err, nil).
				WithErrorCode(httperror.Code400_6).
				Render(w)
			return
		case errors.As(err, &infoNotFoundErr):
			log.Ctx(ctx).Error(infoNotFoundErr.cause)
			httperror.BadRequest(InformationNotFoundOnServer, err, nil).WithErrorCode(httperror.Code400_2).Render(w)
			return
		case errors.As(err, &verficationAttemptsExceededErr):
			log.Ctx(ctx).Error(verficationAttemptsExceededErr.cause)
			httperror.BadRequest(verficationAttemptsExceededErr.Error(), err, nil).WithErrorCode(httperror.Code400_3).Render(w)
			return
		default:
			httperror.InternalError(ctx, "", err, nil).WithErrorCode(httperror.Code500_0).Render(w)
			return
		}
	}

	httpjson.Render(w, map[string]string{"message": "ok"}, httpjson.JSON)
}

// incrementOTPAttempts increments the OTP attempts counter and returns the appropriate error.
func (v VerifyReceiverRegistrationHandler) incrementOTPAttempts(ctx context.Context, receiverWallet *data.ReceiverWallet) error {
	receiverWallet.OTPAttempts++

	err := v.Models.ReceiverWallet.Update(ctx, receiverWallet.ID, data.ReceiverWalletUpdate{
		OTPAttempts: &receiverWallet.OTPAttempts,
	}, v.Models.DBConnectionPool)
	if err != nil {
		return fmt.Errorf("updating receiver wallet OTP attempts: %w", err)
	}

	// Check if max attempts reached after increment
	if receiverWallet.OTPAttempts >= OTPMaxAttempts {
		return ErrOTPMaxAttemptsExceeded
	}

	return ErrOTPDoesNotMatch
}

// verifyReceiverWalletOTP validates the receiver wallet OTP.
func verifyReceiverWalletOTP(ctx context.Context, networkPassphrase string, receiverWallet data.ReceiverWallet, providedOTP string) error {
	// Validation
	if providedOTP == "" {
		return fmt.Errorf("otp cannot be empty")
	}

	// 1. Check if OTP max attempts exceeded
	if receiverWallet.OTPAttempts >= OTPMaxAttempts {
		return ErrOTPMaxAttemptsExceeded
	}

	// 2. Verify special testnet OTPs
	if utils.IsTestNetwork(networkPassphrase) {
		switch providedOTP {
		case data.TestnetAlwaysValidOTP:
			log.Ctx(ctx).Warnf("OTP is being approved because TestnetAlwaysValidOTP (%s) was used", data.TestnetAlwaysValidOTP)
			return nil
		case data.TestnetAlwaysInvalidOTP:
			log.Ctx(ctx).Errorf("OTP is being denied because TestnetAlwaysInvalidOTP (%s) was used", data.TestnetAlwaysInvalidOTP)
			return ErrOTPDoesNotMatch
		}
	}

	// 3. Check if OTP is expired
	if receiverWallet.OTPCreatedAt == nil || receiverWallet.OTPCreatedAt.IsZero() {
		return fmt.Errorf("otp does not have a valid created_at time")
	}
	otpExpirationTime := receiverWallet.OTPCreatedAt.UTC().Add(time.Minute * OTPExpirationTimeMinutes)
	if time.Now().UTC().After(otpExpirationTime) {
		return ErrOTPExpired
	}

	// 4. Verify the OTP against the one saved in the database
	if !isOTPValid(receiverWallet.OTP, providedOTP) {
		return ErrOTPDoesNotMatch
	}

	return nil
}

// isOTPValid performs a constant-time comparison of OTPs to prevent timing attacks.
func isOTPValid(storedOTP, providedOTP string) bool {
	if len(storedOTP) != len(providedOTP) {
		return false
	}
	// Use subtle.ConstantTimeCompare for timing-attack resistant comparison
	return subtle.ConstantTimeCompare([]byte(storedOTP), []byte(providedOTP)) == 1
}
