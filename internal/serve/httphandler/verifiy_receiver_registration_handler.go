package httphandler

import (
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
		log.Ctx(ctx).Error("request body is empty")
		return reqObj, nil, httperror.BadRequest("", nil, nil)
	}
	receiverRegistrationRequest := data.ReceiverRegistrationRequest{}
	err := json.NewDecoder(r.Body).Decode(&receiverRegistrationRequest)
	if err != nil {
		err = fmt.Errorf("invalid request body: %w", err)
		log.Ctx(ctx).Error(err)
		return reqObj, nil, httperror.BadRequest("", err, nil)
	}

	// STEP 3: Validate reCAPTCHA Token
	isValid, err := v.ReCAPTCHAValidator.IsTokenValid(ctx, receiverRegistrationRequest.ReCAPTCHAToken)
	if err != nil {
		return reqObj, nil, httperror.InternalError(ctx, "Cannot validate reCAPTCHA token", err, nil)
	}
	if !isValid {
		log.Ctx(ctx).Errorf("reCAPTCHA token is invalid for request with OTP %s and Phone Number %s",
			utils.TruncateString(receiverRegistrationRequest.OTP, 2), utils.TruncateString(receiverRegistrationRequest.PhoneNumber, 4))
		return reqObj, nil, httperror.BadRequest("", nil, nil)
	}

	// STEP 4: Validate request body
	validator := validators.NewReceiverRegistrationValidator()
	validator.ValidateReceiver(&receiverRegistrationRequest)
	if validator.HasErrors() {
		log.Ctx(ctx).Errorf("request invalid: %s", validator.Errors)
		return reqObj, nil, httperror.BadRequest("", nil, validator.Errors)
	}

	return receiverRegistrationRequest, sep24Claims, nil
}

// VerifyReceiverRegistration implements the http.Handler interface.
func (v VerifyReceiverRegistrationHandler) VerifyReceiverRegistration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// STEP 1: Validate request
	receiverRegistrationRequest, sep24Claims, httpErr := v.validate(r)
	if httpErr != nil {
		httpErr.Render(w)
		return
	}

	atomicFnErr := db.RunInTransaction(ctx, v.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) error {
		// STEP 2: find the receivers with the given phone number
		receivers, err := v.Models.Receiver.GetByPhoneNumbers(ctx, dbTx, []string{receiverRegistrationRequest.PhoneNumber})
		if err != nil {
			log.Ctx(ctx).Errorf("error retrieving receiver with phone number %s: %s", utils.TruncateString(receiverRegistrationRequest.PhoneNumber, 3), err.Error())
			return err
		}
		if len(receivers) == 0 {
			err = fmt.Errorf("receiver with phone number %s not found in our server", receiverRegistrationRequest.PhoneNumber)
			return &ErrorInformationNotFound{cause: err}
		}

		// STEP 3: find the receiverVerification entry that matches the pair [receiverID, verificationType]
		receiverVerifications, err := v.Models.ReceiverVerification.GetByReceiverIDsAndVerificationField(ctx, dbTx, []string{receivers[0].ID}, receiverRegistrationRequest.VerificationType)
		if err != nil {
			log.Ctx(ctx).Errorf("error retrieving receiver verification for verification type %s", receiverRegistrationRequest.VerificationType)
			return err
		}
		if len(receiverVerifications) == 0 {
			err = fmt.Errorf("%s not found for receiver with phone number %s", receiverRegistrationRequest.VerificationType, receiverRegistrationRequest.PhoneNumber)
			return &ErrorInformationNotFound{cause: err}
		}
		if len(receiverVerifications) > 1 {
			log.Ctx(ctx).Warnf("receiver with id %s has more than one verification saved in the database for type %s", receivers[0].ID, receiverRegistrationRequest.VerificationType)
		}
		receiverVerification := receiverVerifications[0]

		// STEP 4: check if the number of attempts to confirm the verification value exceeded the max value
		// TODO: the application currently can't recover from a max attempts exceeded error.
		if v.Models.ReceiverVerification.ExceededAttempts(receiverVerification.Attempts) {
			err = fmt.Errorf("number of attempts to confirm the verification value exceeded max attempts value %d", data.MaxAttemptsAllowed)
			return &ErrorInformationNotFound{cause: err}
		}

		now := time.Now()
		// check if verification value match with value saved in the database
		if !data.CompareVerificationValue(receiverVerification.HashedValue, receiverRegistrationRequest.VerificationValue) {
			baseErrMsg := fmt.Sprintf("%s value does not match for user with phone number %s", receiverRegistrationRequest.VerificationType, receiverRegistrationRequest.PhoneNumber)
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

		// update the receiver verification with the confirmation that the value was checked
		if receiverVerification.ConfirmedAt == nil {
			receiverVerification.ConfirmedAt = &now
			err = v.Models.ReceiverVerification.UpdateReceiverVerification(ctx, *receiverVerification, dbTx)
			if err != nil {
				return fmt.Errorf("updating successfully verified user: %w", err)
			}
		}

		receiverWallet, err := v.Models.ReceiverWallet.GetByReceiverIDAndWalletDomain(ctx, receivers[0].ID, sep24Claims.ClientDomain(), dbTx)
		if err != nil {
			err = fmt.Errorf("receiver wallet not found for receiver with id %s and client domain %s: %w", receivers[0].ID, sep24Claims.ClientDomain(), err)
			return &ErrorInformationNotFound{cause: err}
		}

		// check if receiver is already registered
		if receiverWallet.Status == data.RegisteredReceiversWalletStatus {
			log.Ctx(ctx).Info("receiver already registered in the SDP")
			return nil
		}

		// check if receiver wallet status can transition to RegisteredReceiversWalletStatus
		err = receiverWallet.Status.TransitionTo(data.RegisteredReceiversWalletStatus)
		if err != nil {
			err = fmt.Errorf("transitioning status for receiver[ID=%s], receiverWallet[ID=%s]: %w", receivers[0].ID, receiverWallet.ID, err)
			return &ErrorInformationNotFound{cause: err}
		}

		// check if receiver_wallet OTP is valid and not expired
		err = v.Models.ReceiverWallet.VerifyReceiverWalletOTP(ctx, v.NetworkPassphrase, *receiverWallet, receiverRegistrationRequest.OTP)
		if err != nil {
			err = fmt.Errorf("receiver wallet otp is not valid: %w", err)
			return &ErrorInformationNotFound{cause: err}
		}

		// update receiver wallet
		receiverWallet.StellarAddress = sep24Claims.SEP10StellarAccount()
		if sep24Claims.SEP10StellarMemo() != "" {
			receiverWallet.StellarMemo = sep24Claims.SEP10StellarMemo()
			receiverWallet.StellarMemoType = "id"
		}
		receiverWallet.Status = data.RegisteredReceiversWalletStatus

		err = v.Models.ReceiverWallet.UpdateReceiverWallet(ctx, *receiverWallet, dbTx)
		if err != nil {
			err = fmt.Errorf("completing receiver wallet registration for phone number %s: %w", utils.TruncateString(receiverRegistrationRequest.PhoneNumber, 3), err)
			return err
		}

		// TODO: find the oldest payment that touches that receiver wallet, then save the AP txID there.
		// PATCH transaction on the AnchorPlatform
		transaction := &anchorplatform.Transaction{
			TransactionValues: anchorplatform.TransactionValues{
				ID:                 sep24Claims.TransactionID(),
				Status:             "pending_anchor",
				Sep:                "24",
				Kind:               "deposit",
				DestinationAccount: sep24Claims.SEP10StellarAccount(),
				Memo:               sep24Claims.SEP10StellarMemo(),
				KYCVerified:        true,
			},
		}
		err = v.AnchorPlatformAPIService.UpdateAnchorTransactions(ctx, []anchorplatform.Transaction{*transaction})
		if err != nil {
			err = fmt.Errorf("updating transaction with ID %s on anchor platform API: %w", sep24Claims.TransactionID(), err)
			return err
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
