package validators

import (
	"fmt"
	"strings"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type DisbursementInstructionsValidator struct {
	contactType       data.RegistrationContactType
	verificationField data.VerificationType
	*Validator
}

func NewDisbursementInstructionsValidator(contactType data.RegistrationContactType, verificationField data.VerificationType) *DisbursementInstructionsValidator {
	return &DisbursementInstructionsValidator{
		contactType:       contactType,
		verificationField: verificationField,
		Validator:         NewValidator(),
	}
}

func (iv *DisbursementInstructionsValidator) ValidateInstruction(instruction *data.DisbursementInstruction, lineNumber int) {
	// 1. Validate required fields
	iv.Check(instruction.ID != "", fmt.Sprintf("line %d - id", lineNumber), "id cannot be empty")
	iv.CheckError(utils.ValidateAmount(instruction.Amount), fmt.Sprintf("line %d - amount", lineNumber), "invalid amount. Amount must be a positive number")

	// 2. Validate Contact fields
	switch iv.contactType.ReceiverContactType {
	case data.ReceiverContactTypeEmail:
		iv.Check(instruction.Email != "", fmt.Sprintf("line %d - email", lineNumber), "email cannot be empty")
		if instruction.Email != "" {
			iv.CheckError(utils.ValidateEmail(instruction.Email), fmt.Sprintf("line %d - email", lineNumber), "invalid email format")
		}
	case data.ReceiverContactTypeSMS:
		iv.Check(instruction.Phone != "", fmt.Sprintf("line %d - phone", lineNumber), "phone cannot be empty")
		if instruction.Phone != "" {
			iv.CheckError(utils.ValidatePhoneNumber(instruction.Phone), fmt.Sprintf("line %d - phone", lineNumber), "invalid phone format. Correct format: +380445555555")
		}
	}

	// 3. Validate WalletAddress field
	if iv.contactType.IncludesWalletAddress {
		iv.Check(instruction.WalletAddress != "", fmt.Sprintf("line %d - wallet address", lineNumber), "wallet address cannot be empty")
		if instruction.WalletAddress != "" {
			iv.Check(strkey.IsValidEd25519PublicKey(instruction.WalletAddress) || strkey.IsValidContractAddress(instruction.WalletAddress), fmt.Sprintf("line %d - wallet address", lineNumber), "invalid wallet address. Must be a valid Stellar public key or contract address")
		}
		if instruction.WalletAddressMemo != "" && strkey.IsValidEd25519PublicKey(instruction.WalletAddress) {
			_, _, err := schema.ParseMemo(instruction.WalletAddressMemo)
			iv.CheckError(err, fmt.Sprintf("line %d - wallet address memo", lineNumber), "invalid wallet address memo. For more information, visit https://docs.stellar.org/learn/encyclopedia/transactions-specialized/memos")
		}
		if instruction.WalletAddressMemo != "" && strkey.IsValidContractAddress(instruction.WalletAddress) {
			iv.AddError(fmt.Sprintf("line %d - wallet address memo", lineNumber), "wallet address memo is not supported for contract addresses")
		}
	} else {
		// 4. Validate verification field
		verification := instruction.VerificationValue
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
}

func (iv *DisbursementInstructionsValidator) SanitizeInstruction(instruction *data.DisbursementInstruction) *data.DisbursementInstruction {
	var sanitizedInstruction data.DisbursementInstruction
	if instruction.Phone != "" {
		sanitizedInstruction.Phone = strings.ToLower(strings.TrimSpace(instruction.Phone))
	}

	if instruction.Email != "" {
		sanitizedInstruction.Email = strings.ToLower(strings.TrimSpace(instruction.Email))
	}

	if instruction.WalletAddress != "" {
		sanitizedInstruction.WalletAddress = strings.ToUpper(strings.TrimSpace(instruction.WalletAddress))
	}
	sanitizedInstruction.WalletAddressMemo = strings.TrimSpace(instruction.WalletAddressMemo)

	if instruction.ExternalPaymentId != "" {
		sanitizedInstruction.ExternalPaymentId = strings.TrimSpace(instruction.ExternalPaymentId)
	}

	sanitizedInstruction.ID = strings.TrimSpace(instruction.ID)
	sanitizedInstruction.Amount = strings.TrimSpace(instruction.Amount)
	sanitizedInstruction.VerificationValue = strings.TrimSpace(instruction.VerificationValue)

	return &sanitizedInstruction
}
