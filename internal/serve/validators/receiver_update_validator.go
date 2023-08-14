package validators

import (
	"strings"
	"time"

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
		_, err := time.Parse("2006-01-02", updateReceiverRequest.DateOfBirth)
		ur.CheckError(err, "date_of_birth", "invalid date of birth format. Correct format: 1990-01-30")
	}

	if updateReceiverRequest.Pin != "" {
		// TODO: add new validation to PIN type.
		ur.Check(pin != "", "pin", "invalid pin format")
	}

	if updateReceiverRequest.NationalID != "" {
		// TODO: add new validation to NationalID type.
		ur.Check(nationalID != "", "national_id", "invalid national ID format")
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
