package validators

import (
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/dto"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type ReceiverValidator struct {
	*Validator
}

// NewReceiverValidator creates a new ReceiverValidator
func NewReceiverValidator() *ReceiverValidator {
	return &ReceiverValidator{
		Validator: NewValidator(),
	}
}

// ValidateCreateReceiverRequest validates the CreateReceiverRequest
func (rv *ReceiverValidator) ValidateCreateReceiverRequest(req *dto.CreateReceiverRequest) {
	email := strings.TrimSpace(req.Email)
	phoneNumber := strings.TrimSpace(req.PhoneNumber)
	externalID := strings.TrimSpace(req.ExternalID)

	if email == "" && phoneNumber == "" {
		rv.Check(false, "email", "email is required when phone_number is not provided")
		rv.Check(false, "phone_number", "phone_number is required when email is not provided")
	}

	if email != "" {
		rv.CheckError(utils.ValidateEmail(email), "email", "")
	}

	if phoneNumber != "" {
		rv.CheckError(utils.ValidatePhoneNumber(phoneNumber), "phone_number", "")
	}

	rv.Check(externalID != "", "external_id", "external_id is required")

	if len(req.Verifications) == 0 && len(req.Wallets) == 0 {
		rv.Check(false, "verifications", "verifications are required when wallets are not provided")
		rv.Check(false, "wallets", "wallets are required when verifications are not provided")
	}

	if len(req.Wallets) > 1 {
		rv.Check(false, "wallets", "only one wallet is allowed per receiver")
	}

	for i, v := range req.Verifications {
		verificationType := strings.TrimSpace(string(v.Type))
		verificationValue := strings.TrimSpace(v.Value)

		rv.Check(verificationType != "", fmt.Sprintf("verifications[%d].type", i), "verification type is required")
		rv.Check(verificationValue != "", fmt.Sprintf("verifications[%d].value", i), "verification value is required")

		if verificationType != "" && verificationValue != "" {
			rv.validateVerificationType(i, data.VerificationType(verificationType), verificationValue)
		}

		req.Verifications[i].Type = data.VerificationType(verificationType)
		req.Verifications[i].Value = verificationValue
	}

	for i, w := range req.Wallets {
		address := strings.TrimSpace(w.Address)
		memo := strings.TrimSpace(w.Memo)

		rv.Check(address != "", fmt.Sprintf("wallets[%d].address", i), "wallet address is required")

		if address != "" {
			rv.Check(strkey.IsValidEd25519PublicKey(address), fmt.Sprintf("wallets[%d].address", i), "invalid stellar address format")
		}

		if memo != "" {
			_, err := ValidateWalletAddressMemo(address, memo)
			rv.CheckError(err, fmt.Sprintf("wallets[%d].memo", i), "invalid memo format. For more information about memo formats, visit https://docs.stellar.org/learn/encyclopedia/transactions-specialized/memos")
		}

		req.Wallets[i].Address = address
		req.Wallets[i].Memo = memo
	}

	req.Email = email
	req.PhoneNumber = phoneNumber
	req.ExternalID = externalID
}

// validateVerificationType validates specific verification types
func (rv *ReceiverValidator) validateVerificationType(index int, vType data.VerificationType, value string) {
	switch vType {
	case data.VerificationTypeDateOfBirth:
		if _, err := time.Parse("2006-01-02", value); err != nil {
			rv.Check(false, fmt.Sprintf("verifications[%d].value", index), "invalid date of birth format: must be YYYY-MM-DD")
		}
	case data.VerificationTypePin:
		if len(value) < 4 || len(value) > 8 {
			rv.Check(false, fmt.Sprintf("verifications[%d].value", index), "invalid PIN: must be between 4 and 8 characters")
		}
	case data.VerificationTypeNationalID:
		if len(value) > 50 {
			rv.Check(false, fmt.Sprintf("verifications[%d].value", index), "invalid national ID: must be at most 50 characters")
		}
	case data.VerificationTypeYearMonth:
		if _, err := time.Parse("2006-01", value); err != nil {
			rv.Check(false, fmt.Sprintf("verifications[%d].value", index), "invalid year-month format: must be YYYY-MM")
		}
	default:
		rv.Check(false, fmt.Sprintf("verifications[%d].type", index), "invalid verification type")
	}
}
