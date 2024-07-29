package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_ReceiverRegistrationValidator_ValidateReceiver(t *testing.T) {
	type testCase struct {
		name             string
		receiverInfo     data.ReceiverRegistrationRequest
		expectedErrorLen int
		expectedErrorMsg string
		expectedErrorKey string
		expectedReceiver data.ReceiverRegistrationRequest
	}

	testCases := []testCase{
		{
			name: "error if phone number is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "invalid",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationType:  data.VerificationFieldDateOfBirth,
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "invalid phone format. Correct format: +380445555555",
			expectedErrorKey: "phone_number",
		},
		{
			name: "error if phone number is empty",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationType:  data.VerificationFieldDateOfBirth,
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "phone cannot be empty",
			expectedErrorKey: "phone_number",
		},
		{
			name: "error if OTP is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "12mock",
				VerificationValue: "1990-01-01",
				VerificationType:  data.VerificationFieldDateOfBirth,
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "invalid otp format. Needs to be a 6 digit value",
			expectedErrorKey: "otp",
		},
		{
			name: "error if verification type is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationType:  "mock_type",
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "invalid parameter. valid values are: [DATE_OF_BIRTH YEAR_MONTH PIN NATIONAL_ID_NUMBER]",
			expectedErrorKey: "verification_type",
		},
		{
			name: "error if verification[DATE_OF_BIRTH] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "90/01/01",
				VerificationType:  data.VerificationFieldDateOfBirth,
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "invalid date of birth format. Correct format: 1990-01-30",
			expectedErrorKey: "verification",
		},
		{
			name: "error if verification[YEAR_MONTH] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "90/12",
				VerificationType:  data.VerificationFieldYearMonth,
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "invalid year/month format. Correct format: 1990-12",
			expectedErrorKey: "verification",
		},
		{
			name: "error if verification[PIN] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "ABCDE1234",
				VerificationType:  data.VerificationFieldPin,
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "invalid pin length. Cannot have less than 4 or more than 8 characters in pin",
			expectedErrorKey: "verification",
		},
		{
			name: "error if verification[NATIONAL_ID_NUMBER] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "6UZMB56FWTKV4U0PJ21TBR6VOQVYSGIMZG2HW2S0L7EK5K83W78XXXXX",
				VerificationType:  data.VerificationFieldNationalID,
			},
			expectedErrorLen: 1,
			expectedErrorMsg: "invalid national id. Cannot have more than 50 characters in national id",
			expectedErrorKey: "verification",
		},
		{
			name: "[DATE_OF_BIRTH] valid receiver values",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "1990-01-01  ",
				VerificationType:  "date_of_birth",
			},
			expectedErrorLen: 0,
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationType:  data.VerificationFieldDateOfBirth,
			},
		},
		{
			name: "[YEAR_MONTH] valid receiver values",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "1990-12  ",
				VerificationType:  "year_month",
			},
			expectedErrorLen: 0,
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-12",
				VerificationType:  data.VerificationFieldYearMonth,
			},
		},
		{
			name: "[PIN] valid receiver values",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "1234  ",
				VerificationType:  "pin",
			},
			expectedErrorLen: 0,
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1234",
				VerificationType:  data.VerificationFieldPin,
			},
		},
		{
			name: "[NATIONAL_ID_NUMBER] valid receiver values",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "  NATIONALIDNUMBER123",
				VerificationType:  "national_id_number",
			},
			expectedErrorLen: 0,
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "NATIONALIDNUMBER123",
				VerificationType:  data.VerificationFieldNationalID,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewReceiverRegistrationValidator()
			validator.ValidateReceiver(&tc.receiverInfo)

			assert.Equal(t, tc.expectedErrorLen, len(validator.Errors))

			if tc.expectedErrorLen > 0 {
				assert.Equal(t, tc.expectedErrorMsg, validator.Errors[tc.expectedErrorKey])
			} else {
				assert.Equal(t, tc.expectedReceiver.PhoneNumber, tc.receiverInfo.PhoneNumber)
				assert.Equal(t, tc.expectedReceiver.OTP, tc.receiverInfo.OTP)
				assert.Equal(t, tc.expectedReceiver.VerificationValue, tc.receiverInfo.VerificationValue)
				assert.Equal(t, tc.expectedReceiver.VerificationType, tc.receiverInfo.VerificationType)
			}
		})
	}
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
		assert.Equal(t, "invalid parameter. valid values are: [DATE_OF_BIRTH YEAR_MONTH PIN NATIONAL_ID_NUMBER]", validator.Errors["verification_type"])
	})
}
