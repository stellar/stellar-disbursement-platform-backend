package validators

import (
	"fmt"
	"slices"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type ReceiverRegistrationValidator struct {
	*Validator
}

// NewReceiverRegistrationValidator creates a new ReceiverRegistrationValidator with the provided configuration.
func NewReceiverRegistrationValidator() *ReceiverRegistrationValidator {
	return &ReceiverRegistrationValidator{
		Validator: NewValidator(),
	}
}

// ValidateReceiver validates if the infos present in the ReceiverRegistrationRequest are valids.
func (rv *ReceiverRegistrationValidator) ValidateReceiver(receiverInfo *data.ReceiverRegistrationRequest) {
	phone := strings.TrimSpace(receiverInfo.PhoneNumber)
	otp := strings.TrimSpace(receiverInfo.OTP)
	verification := strings.TrimSpace(receiverInfo.VerificationValue)
	verificationType := strings.TrimSpace(string(receiverInfo.VerificationType))

	// validate phone field
	rv.CheckError(utils.ValidatePhoneNumber(phone), "phone_number", "invalid phone format. Correct format: +380445555555")
	rv.Check(strings.TrimSpace(phone) != "", "phone_number", "phone cannot be empty")

	// validate otp field
	rv.CheckError(utils.ValidateOTP(otp), "otp", "invalid otp format. Needs to be a 6 digit value")

	// validate verification type field
	rv.Check(verificationType != "", "verification_type", "verification type cannot be empty")
	vt := rv.validateAndGetVerificationType(verificationType)

	// validate verification fields
	switch vt {
	case data.VerificationFieldDateOfBirth:
		rv.CheckError(utils.ValidateDateOfBirthVerification(verification), "verification", "")
	case data.VerificationFieldPin:
		rv.CheckError(utils.ValidatePinVerification(verification), "verification", "")
	case data.VerificationFieldNationalID:
		rv.CheckError(utils.ValidateNationalIDVerification(verification), "verification", "")
	}

	receiverInfo.PhoneNumber = phone
	receiverInfo.OTP = otp
	receiverInfo.VerificationValue = verification
	receiverInfo.VerificationType = vt
}

// validateAndGetVerificationType validates if the verification type field is a valid value.
func (rv *ReceiverRegistrationValidator) validateAndGetVerificationType(verificationType string) data.VerificationField {
	vt := data.VerificationField(strings.ToUpper(verificationType))

	if !slices.Contains(data.GetAllVerificationFields(), vt) {
		rv.Check(false, "verification_type", fmt.Sprintf("invalid parameter. valid values are: %v", data.GetAllVerificationFields()))
		return ""
	}
	return vt
}
