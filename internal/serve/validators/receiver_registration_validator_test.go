package validators

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_ReceiverRegistrationValidator_ValidateReceiver(t *testing.T) {
	t.Run("Invalid phone number", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "invalid",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "DATE_OF_BIRTH",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid phone format. Correct format: +380445555555", validator.Errors["phone_number"])
	})

	t.Run("Empty phone number", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "DATE_OF_BIRTH",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "phone cannot be empty", validator.Errors["phone_number"])
	})

	t.Run("Invalid otp", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "12mock",
			VerificationValue: "1990-01-01",
			VerificationType:  "DATE_OF_BIRTH",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid otp format. Needs to be a 6 digit value", validator.Errors["otp"])
	})

	t.Run("Invalid verification type", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "1990-01-01",
			VerificationType:  "mock_type",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: DATE_OF_BIRTH, PIN, NATIONAL_ID_NUMBER", validator.Errors["verification_type"])
	})

	t.Run("Invalid date of birth", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "90/01/01",
			VerificationType:  "DATE_OF_BIRTH",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid date of birth format. Correct format: 1990-01-01", validator.Errors["verification"])
	})

	t.Run("Invalid pin", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "ABCDE1234",
			VerificationType:  "PIN",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid pin. Cannot have less than 4 or more than 8 characters in pin", validator.Errors["verification"])
	})

	t.Run("Invalid national ID number", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555",
			OTP:               "123456",
			VerificationValue: "6UZMB56FWTKV4U0PJ21TBR6VOQVYSGIMZG2HW2S0L7EK5K83W78XXXXX",
			VerificationType:  "NATIONAL_ID_NUMBER",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid national id. Cannot have more than 50 characters in national id", validator.Errors["verification"])
	})

	t.Run("Valid receiver values", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()

		receiverInfo := data.ReceiverRegistrationRequest{
			PhoneNumber:       "+380445555555  ",
			OTP:               "  123456  ",
			VerificationValue: "1990-01-01  ",
			VerificationType:  "date_of_birth",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 0, len(validator.Errors))
		assert.Equal(t, "+380445555555", receiverInfo.PhoneNumber)
		assert.Equal(t, "123456", receiverInfo.OTP)
		assert.Equal(t, "1990-01-01", receiverInfo.VerificationValue)
		assert.Equal(t, data.VerificationField("DATE_OF_BIRTH"), receiverInfo.VerificationType)

		receiverInfo.VerificationValue = "1234"
		receiverInfo.VerificationType = "pin"
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 0, len(validator.Errors))
		assert.Equal(t, "1234", receiverInfo.VerificationValue)
		assert.Equal(t, data.VerificationField("PIN"), receiverInfo.VerificationType)

		receiverInfo.VerificationValue = "NATIONALIDNUMBER123"
		receiverInfo.VerificationType = "national_id_number"
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 0, len(validator.Errors))
		assert.Equal(t, "NATIONALIDNUMBER123", receiverInfo.VerificationValue)
		assert.Equal(t, data.VerificationField("NATIONAL_ID_NUMBER"), receiverInfo.VerificationType)
	})
}

func Test_ReceiverRegistrationValidator_ValidateAndGetVerificationType(t *testing.T) {
	t.Run("Valid verification type", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()
		validField := []data.VerificationField{
			data.VerificationFieldDateOfBirth,
			data.VerificationFieldPin,
			data.VerificationFieldNationalID,
		}
		for _, field := range validField {
			assert.Equal(t, field, validator.validateAndGetVerificationType(string(field)))
		}
	})

	t.Run("Invalid verification type", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()
		invalidStatus := "unknown"

		actual := validator.validateAndGetVerificationType(invalidStatus)
		assert.Empty(t, actual)
		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: DATE_OF_BIRTH, PIN, NATIONAL_ID_NUMBER", validator.Errors["verification_type"])
	})
}
