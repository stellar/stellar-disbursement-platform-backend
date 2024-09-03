package validators

import (
	"fmt"
	"slices"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type DisbursementRequestValidator struct {
	verificationField data.VerificationType
	*Validator
}

func NewDisbursementRequestValidator(verificationField data.VerificationType) *DisbursementRequestValidator {
	return &DisbursementRequestValidator{
		verificationField: verificationField,
		Validator:         NewValidator(),
	}
}

// ValidateAndGetVerificationType validates if the verification type field is a valid value.
func (dv *DisbursementRequestValidator) ValidateAndGetVerificationType() data.VerificationType {
	if !slices.Contains(data.GetAllVerificationTypes(), dv.verificationField) {
		dv.Check(false, "verification_field", fmt.Sprintf("invalid parameter. valid values are: %v", data.GetAllVerificationTypes()))
		return ""
	}
	return dv.verificationField
}
