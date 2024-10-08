package validators

import (
	"fmt"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type DisbursementInstructionsValidator struct {
	verificationField data.VerificationType
	*Validator
}

func NewDisbursementInstructionsValidator(verificationField data.VerificationType) *DisbursementInstructionsValidator {
	return &DisbursementInstructionsValidator{
		verificationField: verificationField,
		Validator:         NewValidator(),
	}
}

func (iv *DisbursementInstructionsValidator) ValidateInstruction(instruction *data.DisbursementInstruction, lineNumber int) {
	var phone, email string
	if instruction.Phone != "" {
		phone = strings.TrimSpace(instruction.Phone)
	}
	if instruction.Email != "" {
		email = strings.TrimSpace(instruction.Email)
	}

	id := strings.TrimSpace(instruction.ID)
	amount := strings.TrimSpace(instruction.Amount)
	verification := strings.TrimSpace(instruction.VerificationValue)

	// validate contact field provided
	iv.Check(phone != "" || email != "", fmt.Sprintf("line %d - contact", lineNumber), "phone or email must be provided")

	// validate phone field
	if phone != "" {
		iv.CheckError(utils.ValidatePhoneNumber(phone), fmt.Sprintf("line %d - phone", lineNumber), "invalid phone format. Correct format: +380445555555")
	}

	// validate email field
	if email != "" {
		iv.CheckError(utils.ValidateEmail(email), fmt.Sprintf("line %d - email", lineNumber), "invalid email format")
	}

	// validate id field
	iv.Check(id != "", fmt.Sprintf("line %d - id", lineNumber), "id cannot be empty")

	// validate amount field
	iv.CheckError(utils.ValidateAmount(amount), fmt.Sprintf("line %d - amount", lineNumber), "invalid amount. Amount must be a positive number")

	// validate verification field
	switch iv.verificationField {
	case data.VerificationTypeDateOfBirth:
		iv.CheckError(utils.ValidateDateOfBirthVerification(verification), fmt.Sprintf("line %d - date of birth", lineNumber), "")
	case data.VerificationTypeYearMonth:
		iv.CheckError(utils.ValidateYearMonthVerification(verification), fmt.Sprintf("line %d - year/month", lineNumber), "")
	case data.VerificationTypePin:
		iv.CheckError(utils.ValidatePinVerification(verification), fmt.Sprintf("line %d - pin", lineNumber), "")
	case data.VerificationTypeNationalID:
		iv.CheckError(utils.ValidateNationalIDVerification(verification), fmt.Sprintf("line %d - national id", lineNumber), "")
	}
}

func (iv *DisbursementInstructionsValidator) SanitizeInstruction(instruction *data.DisbursementInstruction) *data.DisbursementInstruction {
	var sanitizedInstruction data.DisbursementInstruction
	if instruction.Phone != "" {
		sanitizedInstruction.Phone = strings.ToLower(strings.TrimSpace(instruction.Phone))
	}

	if instruction.Email != "" {
		sanitizedInstruction.Email = strings.ToLower(strings.TrimSpace(instruction.Email))
	}

	if instruction.ExternalPaymentId != "" {
		sanitizedInstruction.ExternalPaymentId = strings.TrimSpace(instruction.ExternalPaymentId)
	}

	sanitizedInstruction.ID = strings.TrimSpace(instruction.ID)
	sanitizedInstruction.Amount = strings.TrimSpace(instruction.Amount)
	sanitizedInstruction.VerificationValue = strings.TrimSpace(instruction.VerificationValue)

	return &sanitizedInstruction
}
