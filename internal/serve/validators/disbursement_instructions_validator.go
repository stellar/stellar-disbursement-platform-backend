package validators

import (
	"fmt"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const (
	VERIFICATION_FIELD_PIN_MIN_LENGTH = 4
	VERIFICATION_FIELD_PIN_MAX_LENGTH = 8

	VERIFICATION_FIELD_MAX_ID_LENGTH = 50
)

type DisbursementInstructionsValidator struct {
	verificationField data.VerificationField
	*Validator
}

func NewDisbursementInstructionsValidator(verificationField data.VerificationField) *DisbursementInstructionsValidator {
	return &DisbursementInstructionsValidator{
		verificationField: verificationField,
		Validator:         NewValidator(),
	}
}

func (iv *DisbursementInstructionsValidator) ValidateInstruction(instruction *data.DisbursementInstruction, lineNumber int) {
	phone := instruction.Phone
	id := instruction.ID
	amount := instruction.Amount
	verification := instruction.VerificationValue

	// validate phone field
	iv.CheckError(utils.ValidatePhoneNumber(phone), fmt.Sprintf("line %d - phone", lineNumber), "invalid phone format. Correct format: +380445555555")
	iv.Check(strings.TrimSpace(phone) != "", fmt.Sprintf("line %d - phone", lineNumber), "phone cannot be empty")

	// validate id field
	iv.Check(strings.TrimSpace(id) != "", fmt.Sprintf("line %d - id", lineNumber), "id cannot be empty")

	// validate amount field
	iv.CheckError(utils.ValidateAmount(amount), fmt.Sprintf("line %d - amount", lineNumber), "invalid amount. Amount must be a positive number")

	// validate verification field
	switch iv.verificationField {
	case data.VerificationFieldDateOfBirth:
		// date of birth with format 2006-01-02
		dob, err := time.Parse("2006-01-02", verification)
		iv.CheckError(err, fmt.Sprintf("line %d - birthday", lineNumber), "invalid date of birth format. Correct format: 1990-01-01")

		// check if date of birth is in the past
		iv.Check(dob.Before(time.Now()), fmt.Sprintf("line %d - birthday", lineNumber), "date of birth cannot be in the future")
	case data.VerificationFieldPin:
		if len(verification) < VERIFICATION_FIELD_PIN_MIN_LENGTH || len(verification) > VERIFICATION_FIELD_PIN_MAX_LENGTH {
			iv.addError(fmt.Sprintf("line %d - pin", lineNumber), "invalid pin. Cannot have less than 4 or more than 8 characters in pin")
		}
	case data.VerificationFieldNationalID:
		if len(verification) > VERIFICATION_FIELD_MAX_ID_LENGTH {
			iv.addError(fmt.Sprintf("line %d - national id", lineNumber), "invalid national id. Cannot have more than 50 characters in national id")
		}
	}
}

func (iv *DisbursementInstructionsValidator) SanitizeInstruction(instruction *data.DisbursementInstruction) *data.DisbursementInstruction {
	var sanitizedInstruction data.DisbursementInstruction
	sanitizedInstruction.Phone = strings.TrimSpace(instruction.Phone)
	sanitizedInstruction.ID = strings.TrimSpace(instruction.ID)
	sanitizedInstruction.Amount = strings.TrimSpace(instruction.Amount)
	sanitizedInstruction.VerificationValue = strings.TrimSpace(instruction.VerificationValue)

	if instruction.ExternalPaymentId != nil {
		externalPaymentId := strings.TrimSpace(*instruction.ExternalPaymentId)
		sanitizedInstruction.ExternalPaymentId = &externalPaymentId
	}
	return &sanitizedInstruction
}
