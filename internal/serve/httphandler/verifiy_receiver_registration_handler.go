package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// ErrorInformationNotFound implements the error interface.
type ErrorInformationNotFound struct {
	cause error
}

func (e *ErrorInformationNotFound) Error() string {
	return e.cause.Error()
}

const (
	InformationNotFoundOnServer = "the information you provided could not be found in our server"
)

type VerifyReceiverRegistrationHandler struct {
	AnchorPlatformAPIService anchorplatform.AnchorPlatformAPIServiceInterface
	Models                   *data.Models
	ReCAPTCHAValidator       validators.ReCAPTCHAValidator
	NetworkPassphrase        string
}

// validate validates the request [header, body, body.reCAPTCHA_token], and returns the decoded payload, or an http error.
func (v VerifyReceiverRegistrationHandler) validate(r *http.Request) (reqObj data.ReceiverRegistrationRequest, sep24Claims *anchorplatform.SEP24JWTClaims, httpErr *httperror.HTTPError) {
	ctx := r.Context()

	// STEP 1: Validate SEP-24 JWT token
	sep24Claims = anchorplatform.GetSEP24Claims(ctx)
	if sep24Claims == nil {
		err := fmt.Errorf("no SEP-24 claims found in the request context")
		log.Ctx(ctx).Error(err)
		return reqObj, nil, httperror.Unauthorized("", err, nil)
	}

	// STEP 2: Decode request body
	if r.Body == nil {
		err := fmt.Errorf("request body is empty")
		return reqObj, nil, httperror.BadRequest("", err, nil)
	}
	receiverRegistrationRequest := data.ReceiverRegistrationRequest{}
	err := json.NewDecoder(r.Body).Decode(&receiverRegistrationRequest)
	if err != nil {
		err = fmt.Errorf("invalid request body: %w", err)
		return reqObj, nil, httperror.BadRequest("", err, nil)
	}

	// STEP 3: Validate reCAPTCHA Token
	isValid, err := v.ReCAPTCHAValidator.IsTokenValid(ctx, receiverRegistrationRequest.ReCAPTCHAToken)
	if err != nil {
		err = fmt.Errorf("validating reCAPTCHA token: %w", err)
		return reqObj, nil, httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", err, nil)
	}
	if !isValid {
		truncatedPhoneNumber := utils.TruncateString(receiverRegistrationRequest.PhoneNumber, 3)
		truncatedOTP := utils.TruncateString(receiverRegistrationRequest.OTP, 2)
		err = fmt.Errorf("reCAPTCHA token is invalid for request with OTP %s and Phone Number %s", truncatedOTP, truncatedPhoneNumber)
		return reqObj, nil, httperror.BadRequest("", err, nil)
	}

	// STEP 4: Validate request body
	validator := validators.NewReceiverRegistrationValidator()
	validator.ValidateReceiver(&receiverRegistrationRequest)
	if validator.HasErrors() {
		err = fmt.Errorf("request invalid: %s", validator.Errors)
		return reqObj, nil, httperror.BadRequest("", err, validator.Errors)
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
	truncatedPhoneNumber := utils.TruncateString(receiver.PhoneNumber, 3)

	// STEP 1: find the receiverVerification entry that matches the pair [receiverID, verificationType]
	receiverVerifications, err := v.Models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{receiver.ID}, receiverRegistrationRequest.VerificationType)
	if err != nil {
		return fmt.Errorf("error retrieving receiver verification for verification type %s: %w", receiverRegistrationRequest.VerificationType, err)
	}
	if len(receiverVerifications) == 0 {
		err = fmt.Errorf("%s not found for receiver with phone number %s", receiverRegistrationRequest.VerificationType, truncatedPhoneNumber)
		return &ErrorInformationNotFound{cause: err}
	}
	if len(receiverVerifications) > 1 {
		log.Ctx(ctx).Warnf("receiver with id %s has more than one verification saved in the database for type %s", receiver.ID, receiverRegistrationRequest.VerificationType)
	}
	receiverVerification := receiverVerifications[0]

	// STEP 2: check if the number of attempts to confirm the verification value has already exceeded the max value
	if v.Models.ReceiverVerification.ExceededAttempts(receiverVerification.Attempts) {
		// TODO: the application currently can't recover from a max attempts exceeded error.
		err = fmt.Errorf("the number of attempts to confirm the verification value exceededs the max attempts limit of %d", data.MaxAttemptsAllowed)
		return &ErrorInformationNotFound{cause: err}
	}

	// STEP 3: check if the payload verification value matches the one saved in the database
	if !data.CompareVerificationValue(receiverVerification.HashedValue, receiverRegistrationRequest.VerificationValue) {
		baseErrMsg := fmt.Sprintf("%s value does not match for user with phone number %s", receiverRegistrationRequest.VerificationType, truncatedPhoneNumber)
		// update the receiver verification with the confirmation that the value was checked
		receiverVerification.Attempts = receiverVerification.Attempts + 1
		receiverVerification.FailedAt = &now
		receiverVerification.ConfirmedAt = nil

		// this update is done using the DBConnectionPool and not dbTx because we don't want to roolback these changes after returning the error
		updateErr := v.Models.ReceiverVerification.UpdateReceiverVerification(ctx, *receiverVerification, v.Models.DBConnectionPool)
		if updateErr != nil {
			err = fmt.Errorf("%s: %w", baseErrMsg, updateErr)
		} else {
			err = fmt.Errorf("%s", baseErrMsg)
		}

		return &ErrorInformationNotFound{cause: err}
	}

	// STEP 4: update the receiver verification row with the confirmation that the value was successfully validated
	if receiverVerification.ConfirmedAt == nil {
		receiverVerification.ConfirmedAt = &now
		err = v.Models.ReceiverVerification.UpdateReceiverVerification(ctx, *receiverVerification, dbTx)
		if err != nil {
			return fmt.Errorf("updating successfully verified user: %w", err)
		}
	}

	return nil
}

// processReceiverWalletOTP processes the OTP provided by the user and updates the receiver wallet status to "REGISTERED" if the OTP is valid.
func (v VerifyReceiverRegistrationHandler) processReceiverWalletOTP(
	ctx context.Context,
	dbTx db.DBTransaction,
	sep24Claims anchorplatform.SEP24JWTClaims,
	receiver data.Receiver, otp string,
) (receiverWallet data.ReceiverWallet, wasAlreadyRegistered bool, err error) {
	// STEP 1: find the receiver wallet for the given [receiverID, clientDomain]
	rw, err := v.Models.ReceiverWallet.GetByReceiverIDAndWalletDomain(ctx, receiver.ID, sep24Claims.ClientDomain(), dbTx)
	if err != nil {
		err = fmt.Errorf("receiver wallet not found for receiverID=%s and clientDomain=%s: %w", receiver.ID, sep24Claims.ClientDomain(), err)
		return receiverWallet, false, &ErrorInformationNotFound{cause: err}
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
		return receiverWallet, false, &ErrorInformationNotFound{cause: err}
	}

	// STEP 4: verify receiver wallet OTP
	err = v.Models.ReceiverWallet.VerifyReceiverWalletOTP(ctx, v.NetworkPassphrase, *rw, otp)
	if err != nil {
		err = fmt.Errorf("receiver wallet OTP is not valid: %w", err)
		return receiverWallet, false, &ErrorInformationNotFound{cause: err}
	}

	// STEP 5: update receiver wallet status to "REGISTERED"
	now := time.Now()
	rw.OTPConfirmedAt = &now
	rw.Status = data.RegisteredReceiversWalletStatus
	rw.StellarAddress = sep24Claims.SEP10StellarAccount()
	rw.StellarMemo = sep24Claims.SEP10StellarMemo()
	rw.StellarMemoType = ""
	if sep24Claims.SEP10StellarMemo() != "" {
		rw.StellarMemoType = "id"
	}
	err = v.Models.ReceiverWallet.UpdateReceiverWallet(ctx, *rw, dbTx)
	if err != nil {
		err = fmt.Errorf("completing receiver wallet registration: %w", err)
		return receiverWallet, false, err
	}

	return *rw, false, nil
}

// processAnchorPlatformID PATCHes the transaction on the AnchorPlatform with the "pending_anchor" status, and updates
// the receiver wallet with the anchor platform transaction ID.
func (v VerifyReceiverRegistrationHandler) processAnchorPlatformID(ctx context.Context, dbTx db.DBTransaction, sep24Claims anchorplatform.SEP24JWTClaims, receiverWallet data.ReceiverWallet) error {
	// STEP 1: update receiver wallet with the anchor platform transaction ID.
	receiverWallet.AnchorPlatformTransactionID = sep24Claims.TransactionID()
	err := v.Models.ReceiverWallet.UpdateReceiverWallet(ctx, receiverWallet, dbTx)
	if err != nil {
		return fmt.Errorf("updating receiver wallet with anchor platform transaction ID: %w", err)
	}

	// STEP 2: PATCH transaction on the AnchorPlatform, signaling that it is pending anchor
	apTxPatch := anchorplatform.APSep24TransactionPatchPostRegistration{
		ID:     sep24Claims.TransactionID(),
		SEP:    "24",
		Status: anchorplatform.APTransactionStatusPendingAnchor,
	}
	err = v.AnchorPlatformAPIService.PatchAnchorTransactionsPostRegistration(ctx, apTxPatch)
	if err != nil {
		return fmt.Errorf("updating transaction with ID %s on anchor platform API: %w", sep24Claims.TransactionID(), err)
	}

	return nil
}

// VerifyReceiverRegistration is the handler for the SEP-24 `POST /wallet-registration/verification` endpoint. It is
// where the SDP verifies the receiver's PII & OTP, update the receiver wallet with the Stellar account and memo, found
// in the JWT token, and PATCH the transaction on the AnchorPlatform.
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

	truncatedPhoneNumber := utils.TruncateString(receiverRegistrationRequest.PhoneNumber, 3)

	atomicFnErr := db.RunInTransaction(ctx, v.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// STEP 2: find the receivers with the given phone number
		receivers, err := v.Models.Receiver.GetByPhoneNumbers(ctx, dbTx, []string{receiverRegistrationRequest.PhoneNumber})
		if err != nil {
			err = fmt.Errorf("error retrieving receiver with phone number %s: %w", truncatedPhoneNumber, err)
			return err
		}
		if len(receivers) == 0 {
			err = fmt.Errorf("receiver with phone number %s not found in our server", truncatedPhoneNumber)
			return &ErrorInformationNotFound{cause: err}
		}

		// STEP 3: process receiverVerification PII info that matches the pair [receiverID, verificationType]
		receiver := receivers[0]
		err = v.processReceiverVerificationPII(ctx, dbTx, *receiver, receiverRegistrationRequest)
		if err != nil {
			return fmt.Errorf("processing receiver verification entry for receiver with phone number %s: %w", truncatedPhoneNumber, err)
		}

		// STEP 4: process OTP
		receiverWallet, wasAlreadyRegistered, err := v.processReceiverWalletOTP(ctx, dbTx, *sep24Claims, *receiver, receiverRegistrationRequest.OTP)
		if err != nil {
			return fmt.Errorf("processing OTP for receiver with phone number %s: %w", truncatedPhoneNumber, err)
		}
		if wasAlreadyRegistered {
			return nil
		}

		// STEP 5: PATCH transaction on the AnchorPlatform and update the receiver wallet with the anchor platform tx ID
		err = v.processAnchorPlatformID(ctx, dbTx, *sep24Claims, receiverWallet)
		if err != nil {
			return fmt.Errorf("processing anchor platform transaction ID: %w", err)
		}

		return nil
	})
	if atomicFnErr != nil {
		var errorInformationNotFound *ErrorInformationNotFound
		if errors.As(atomicFnErr, &errorInformationNotFound) {
			log.Ctx(ctx).Error(errorInformationNotFound.cause)
			httperror.BadRequest(InformationNotFoundOnServer, atomicFnErr, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "", atomicFnErr, nil).Render(w)
		return
	}

	httpjson.Render(w, map[string]string{"message": "ok"}, httpjson.JSON)
}
