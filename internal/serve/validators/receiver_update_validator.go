package validators

import (
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type UpdateReceiverRequest struct {
	DateOfBirth string `json:"date_of_birth"`
	Pin         string `json:"pin"`
	NationalID  string `json:"national_id"`
	Email       string `json:"email"`
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
	pin := strings.TrimSpace(updateReceiverRequest.Pin)
	nationalID := strings.TrimSpace(updateReceiverRequest.NationalID)
	email := strings.TrimSpace(updateReceiverRequest.Email)
	externalID := strings.TrimSpace(updateReceiverRequest.ExternalID)

	if dateOfBirth != "" {
		ur.CheckError(utils.ValidateDateOfBirthVerification(dateOfBirth), "date_of_birth", "")
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

	if updateReceiverRequest.ExternalID != "" {
		ur.Check(externalID != "", "external_id", "invalid external_id format")
	}

	updateReceiverRequest.DateOfBirth = dateOfBirth
	updateReceiverRequest.Pin = pin
	updateReceiverRequest.NationalID = nationalID
	updateReceiverRequest.Email = email
	updateReceiverRequest.ExternalID = externalID
}
