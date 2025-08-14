package validators

import (
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go/strkey"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// CreateReceiverRequest represents the request structure for creating receivers
type CreateReceiverRequest struct {
	Email         string                        `json:"email"`
	PhoneNumber   string                        `json:"phone_number"`
	ExternalID    string                        `json:"external_id"`
	Verifications []ReceiverVerificationRequest `json:"verifications"`
	Wallets       []ReceiverWalletRequest       `json:"wallets"`
}

type ReceiverVerificationRequest struct {
	Type  data.VerificationType `json:"type"`
	Value string                `json:"value"`
}

type ReceiverWalletRequest struct {
	Address string `json:"address"`
	Memo    string `json:"memo,omitempty"`
}

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
func (rv *ReceiverValidator) ValidateCreateReceiverRequest(req *CreateReceiverRequest) {
	email := strings.TrimSpace(req.Email)
	phoneNumber := strings.TrimSpace(req.PhoneNumber)
	externalID := strings.TrimSpace(req.ExternalID)

	if email == "" && phoneNumber == "" {
		rv.Check(false, "email", "either email or phone_number must be provided")
		rv.Check(false, "phone_number", "either email or phone_number must be provided")
	}

	if email != "" {
		rv.CheckError(utils.ValidateEmail(email), "email", "")
	}

	if phoneNumber != "" {
		rv.CheckError(utils.ValidatePhoneNumber(phoneNumber), "phone_number", "")
	}

	rv.Check(externalID != "", "external_id", "external_id is required")

	if len(req.Verifications) == 0 && len(req.Wallets) == 0 {
		rv.Check(false, "verifications", "either verifications or wallets must be provided")
		rv.Check(false, "wallets", "either verifications or wallets must be provided")
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
			rv.Check(len(memo) <= 28, fmt.Sprintf("wallets[%d].memo", i), "memo must be at most 28 characters")
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
