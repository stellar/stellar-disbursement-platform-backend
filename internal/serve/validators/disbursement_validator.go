package validators

import (
	"strings"
	"time"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

const (
	VERIFICATION_FIELD_PIN_MIN_LENGTH = 4
	VERIFICATION_FIELD_PIN_MAX_LENGTH = 8

	VERIFICATION_FIELD_MAX_ID_LENGTH = 50
)

type DisbursementStatusValidator struct {
	*Validator
}

func NewDisbursementStatusValidator() *DisbursementStatusValidator {
	return &DisbursementStatusValidator{
		Validator: NewValidator(),
	}
}

func (dsv *DisbursementStatusValidator) ValidateDisbursementStatus(disbursement *data.PostDisbursementRequest) {
	verificationType := strings.TrimSpace(string(disbursement.VerificationType))
	value := strings.TrimSpace(string(disbursement.VerificationValue))

	// validate verification type field
	dsv.Check(verificationType != "", "verification_type", "verification type cannot be empty")
	vt := dsv.validateAndGetVerificationType(verificationType)

	// validate verification field
	// date of birth with format 2006-01-02
	if vt == data.VerificationFieldDateOfBirth {
		dob, err := time.Parse("2006-01-02", verificationType)
		dsv.CheckError(err, "verification", "invalid date of birth format. Correct format: 1990-01-01")

		dsv.Check(dob.Before(time.Now()), "verification", "date of birth cannot be in the future")
	} else if vt == data.VerificationFieldPin {
		if len(verificationType) < VERIFICATION_FIELD_PIN_MIN_LENGTH || len(verificationType) > VERIFICATION_FIELD_PIN_MAX_LENGTH {
			dsv.addError("verification", "invalid pin. Cannot have less than 4 or more than 8 characters in pin")
		}
	} else if vt == data.VerificationFieldNationalID {
		if len(verificationType) > VERIFICATION_FIELD_MAX_ID_LENGTH {
			dsv.addError("verification", "invalid national id. Cannot have more than 50 characters in national id")
		}
	} else {
		// TODO: validate other VerificationField types.
		log.Warnf("Verification type %v is not being validated for ValidateReceiver", vt)
	}

	disbursement.VerificationType = vt
	disbursement.VerificationValue = value
}

// validateAndGetVerificationType validates if the verification type field is a valid value.
func (dsv *DisbursementStatusValidator) validateAndGetVerificationType(verificationType string) data.VerificationField {
	vt := data.VerificationField(strings.ToUpper(verificationType))

	switch vt {
	case data.VerificationFieldDateOfBirth, data.VerificationFieldPin, data.VerificationFieldNationalID:
		return vt
	default:
		dsv.Check(false, "verification_type", "invalid parameter. valid values are: DATE_OF_BIRTH, PIN, NATIONAL_ID_NUMBER")
		return ""
	}
}
