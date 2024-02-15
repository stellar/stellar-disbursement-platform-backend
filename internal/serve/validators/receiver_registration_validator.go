package validators

import (
	"strings"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type ReceiverRegistrationValidator struct {
	*Validator
}

// NewReceiverRegistrationValidator creates a new ReceiverRegistrationValidator with the provided configuration.
func NewReceiverRegistrationValidator() *ReceiverRegistrationValidator {
	return &ReceiverRegistrationValidator{
		Validator: NewValidator(),
	}
}

// ValidateReceiver validates if the infos present in the ReceiverRegistrationRequest are valids.
func (rv *ReceiverRegistrationValidator) ValidateReceiver(receiverInfo *data.ReceiverRegistrationRequest) {
	phone := strings.TrimSpace(receiverInfo.PhoneNumber)
	otp := strings.TrimSpace(receiverInfo.OTP)
	verification := strings.TrimSpace(receiverInfo.VerificationValue)
	verificationType := strings.TrimSpace(string(receiverInfo.VerificationType))

	// validate phone field
	rv.CheckError(utils.ValidatePhoneNumber(phone), "phone_number", "invalid phone format. Correct format: +380445555555")
	rv.Check(strings.TrimSpace(phone) != "", "phone_number", "phone cannot be empty")

	// validate otp field
	rv.CheckError(utils.ValidateOTP(otp), "otp", "invalid otp format. Needs to be a 6 digit value")

	// validate verification type field
	rv.Check(verificationType != "", "verification_type", "verification type cannot be empty")
	vt := rv.validateAndGetVerificationType(verificationType)

	// validate verification field
	// date of birth with format 2006-01-02
	if vt == data.VerificationFieldDateOfBirth {
		dob, err := time.Parse("2006-01-02", verification)
		rv.CheckError(err, "verification", "invalid date of birth format. Correct format: 1990-01-01")

		// check if date of birth is in the past
		rv.Check(dob.Before(time.Now()), "verification", "date of birth cannot be in the future")
	} else if vt == data.VerificationFieldPin {
		if len(verification) < VERIFICATION_FIELD_PIN_MIN_LENGTH || len(verification) > VERIFICATION_FIELD_PIN_MAX_LENGTH {
			rv.addError("verification", "invalid pin. Cannot have less than 4 or more than 8 characters in pin")
		}
	} else if vt == data.VerificationFieldNationalID {
		if len(verification) > VERIFICATION_FIELD_MAX_ID_LENGTH {
			rv.addError("verification", "invalid national id. Cannot have more than 50 characters in national id")
		}
	} else {
		// TODO: validate other VerificationField types.
		log.Warnf("Verification type %v is not being validated for ValidateReceiver", vt)
	}

	receiverInfo.PhoneNumber = phone
	receiverInfo.OTP = otp
	receiverInfo.VerificationValue = verification
	receiverInfo.VerificationType = vt
}

// validateAndGetVerificationType validates if the verification type field is a valid value.
func (rv *ReceiverRegistrationValidator) validateAndGetVerificationType(verificationType string) data.VerificationField {
	vt := data.VerificationField(strings.ToUpper(verificationType))

	switch vt {
	case data.VerificationFieldDateOfBirth, data.VerificationFieldPin, data.VerificationFieldNationalID:
		return vt
	default:
		rv.Check(false, "verification_type", "invalid parameter. valid values are: DATE_OF_BIRTH, PIN, NATIONAL_ID_NUMBER")
		return ""
	}
}
