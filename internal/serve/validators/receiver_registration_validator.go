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
	phone := utils.TrimAndLower(receiverInfo.PhoneNumber)
	email := utils.TrimAndLower(receiverInfo.Email)
	otp := strings.TrimSpace(receiverInfo.OTP)
	verification := strings.TrimSpace(receiverInfo.VerificationValue)
	verificationField := strings.TrimSpace(string(receiverInfo.VerificationField))

	switch {
	case phone == "" && email == "":
		rv.Check(false, "phone_number", "phone_number or email is required")
		rv.Check(false, "email", "phone_number or email is required")
	case phone != "" && email != "":
		rv.Check(false, "phone_number", "phone_number and email cannot be both provided")
		rv.Check(false, "email", "phone_number and email cannot be both provided")
	case phone != "":
		rv.CheckError(utils.ValidatePhoneNumber(phone), "phone_number", "")
	case email != "":
		rv.CheckError(utils.ValidateEmail(email), "email", "")
	}

	// validate otp field
	rv.CheckError(utils.ValidateOTP(otp), "otp", "invalid otp format. Needs to be a 6 digit value")

	// validate verification type field
	rv.Check(verificationField != "", "verification_field", "verification type cannot be empty")
	vf := rv.validateAndGetVerificationType(verificationField)

	// validate verification fields
	switch vf {
	case data.VerificationTypeDateOfBirth:
		rv.CheckError(utils.ValidateDateOfBirthVerification(verification), "verification", "")
	case data.VerificationTypeYearMonth:
		rv.CheckError(utils.ValidateYearMonthVerification(verification), "verification", "")
	case data.VerificationTypePin:
		rv.CheckError(utils.ValidatePinVerification(verification), "verification", "")
	case data.VerificationTypeNationalID:
		rv.CheckError(utils.ValidateNationalIDVerification(verification), "verification", "")
	}

	receiverInfo.PhoneNumber = phone
	receiverInfo.Email = email
	receiverInfo.OTP = otp
	receiverInfo.VerificationValue = verification
	receiverInfo.VerificationField = vf
}

// validateAndGetVerificationType validates if the verification type field is a valid value.
func (rv *ReceiverRegistrationValidator) validateAndGetVerificationType(verificationType string) data.VerificationType {
	vt := data.VerificationType(strings.ToUpper(verificationType))

	if !slices.Contains(data.GetAllVerificationTypes(), vt) {
		rv.Check(false, "verification_field", fmt.Sprintf("invalid parameter. valid values are: %v", data.GetAllVerificationTypes()))
		return ""
	}
	return vt
}
