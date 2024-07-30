package validators

import (
	"fmt"
	"slices"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

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
	if !slices.Contains(data.GetAllVerificationFields(), dv.verificationField) {
		dv.Check(false, "verification_field", fmt.Sprintf("invalid parameter. valid values are: %v", data.GetAllVerificationFields()))
		return ""
	}
	return dv.verificationField
}
