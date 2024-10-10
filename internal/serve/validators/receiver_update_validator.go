package validators

import (
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type UpdateReceiverRequest struct {
	// Verification fields:
	DateOfBirth string `json:"date_of_birth"`
	YearMonth   string `json:"year_month"`
	Pin         string `json:"pin"`
	NationalID  string `json:"national_id"`
	// Receiver fields:
	Email       string `json:"email"`
	PhoneNumber string `json:"phone_number"`
	ExternalID  string `json:"external_id"`
}
type UpdateReceiverValidator struct {
	*Validator
}

// NewReceiverRegistrationValidator creates a new ReceiverRegistrationValidator with the provided configuration.
func NewUpdateReceiverValidator() *UpdateReceiverValidator {
	return &UpdateReceiverValidator{
		Validator: NewValidator(),
	}
}

// ValidateReceiver validates if the infos present in the ReceiverRegistrationRequest are valids.
func (ur *UpdateReceiverValidator) ValidateReceiver(updateReceiverRequest *UpdateReceiverRequest) {
	ur.Check(*updateReceiverRequest != UpdateReceiverRequest{}, "body", "request body is empty")

	if ur.HasErrors() {
		return
	}

	dateOfBirth := strings.TrimSpace(updateReceiverRequest.DateOfBirth)
	yearMonth := strings.TrimSpace(updateReceiverRequest.YearMonth)
	pin := strings.TrimSpace(updateReceiverRequest.Pin)
	nationalID := strings.TrimSpace(updateReceiverRequest.NationalID)
	email := strings.TrimSpace(updateReceiverRequest.Email)
	externalID := strings.TrimSpace(updateReceiverRequest.ExternalID)

	if dateOfBirth != "" {
		ur.CheckError(utils.ValidateDateOfBirthVerification(dateOfBirth), "date_of_birth", "")
	}

	if yearMonth != "" {
		ur.CheckError(utils.ValidateYearMonthVerification(yearMonth), "year_month", "")
	}

	if updateReceiverRequest.Pin != "" {
		ur.CheckError(utils.ValidatePinVerification(pin), "pin", "")
	}

	if updateReceiverRequest.NationalID != "" {
		ur.CheckError(utils.ValidateNationalIDVerification(nationalID), "national_id", "")
	}

	if updateReceiverRequest.Email != "" {
		ur.Check(utils.ValidateEmail(email) == nil, "email", "invalid email format")
	}

	if updateReceiverRequest.PhoneNumber != "" {
		ur.Check(utils.ValidatePhoneNumber(updateReceiverRequest.PhoneNumber) == nil, "phone_number", "invalid phone number format")
	}

	if updateReceiverRequest.ExternalID != "" {
		ur.Check(externalID != "", "external_id", "external_id cannot be set to empty")
	}

	updateReceiverRequest.DateOfBirth = dateOfBirth
	updateReceiverRequest.YearMonth = yearMonth
	updateReceiverRequest.Pin = pin
	updateReceiverRequest.NationalID = nationalID
	updateReceiverRequest.Email = email
	updateReceiverRequest.ExternalID = externalID
}
