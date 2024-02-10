package validators

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

type DisbursementRequestValidator struct {
	verificationField data.VerificationField
	*Validator
}

func NewDisbursementRequestValidator(verificationField data.VerificationField) *DisbursementRequestValidator {
	return &DisbursementRequestValidator{
		verificationField: verificationField,
		Validator:         NewValidator(),
	}
}

// ValidateAndGetVerificationType validates if the verification type field is a valid value.
func (dv *DisbursementRequestValidator) ValidateAndGetVerificationType() data.VerificationField {
	switch dv.verificationField {
	case data.VerificationFieldDateOfBirth, data.VerificationFieldPin, data.VerificationFieldNationalID:
		return dv.verificationField
	default:
		dv.Check(false, "verification_field", "invalid parameter. valid values are: DATE_OF_BIRTH, PIN, NATIONAL_ID_NUMBER")
		return ""
	}
}
