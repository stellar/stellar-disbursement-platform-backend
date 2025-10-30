package validators

import (
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type UpdateReceiverRequest struct {
	// receiver_verifications fields:
	DateOfBirth string `json:"date_of_birth"`
	YearMonth   string `json:"year_month"`
	Pin         string `json:"pin"`
	NationalID  string `json:"national_id"`
	// receivers fields:
	Email       string `json:"email"`
	PhoneNumber string `json:"phone_number"`
	ExternalID  string `json:"external_id"`
}
type UpdateReceiverValidator struct {
	*Validator
}

// NewUpdateReceiverValidator creates a new UpdateReceiverValidator.
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
		_, validationErr := utils.ValidateDateOfBirthVerification(dateOfBirth)
		ur.CheckError(validationErr, "date_of_birth", "")
	}

	if yearMonth != "" {
		_, validationErr := utils.ValidateYearMonthVerification(yearMonth)
		ur.CheckError(validationErr, "year_month", "")
	}

	if updateReceiverRequest.Pin != "" {
		_, validationErr := utils.ValidatePinVerification(pin)
		ur.CheckError(validationErr, "pin", "")
	}

	if updateReceiverRequest.NationalID != "" {
		_, validationErr := utils.ValidateNationalIDVerification(nationalID)
		ur.CheckError(validationErr, "national_id", "")
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
