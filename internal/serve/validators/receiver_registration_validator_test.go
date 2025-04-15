package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_ReceiverRegistrationValidator_ValidateReceiver(t *testing.T) {
	type testCase struct {
		name                     string
		receiverInfo             data.ReceiverRegistrationRequest
		expectedReceiver         data.ReceiverRegistrationRequest
		expectedValidationErrors map[string]interface{}
	}

	testCases := []testCase{
		{
			name: "error if phone number is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "invalid",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
			expectedValidationErrors: map[string]interface{}{
				"phone_number": "the provided phone number is not a valid E.164 number",
			},
		},
		{
			name: "error if email is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				Email:             "invalid",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
			expectedValidationErrors: map[string]interface{}{
				"email": "the email address provided is not valid",
			},
		},
		{
			name: "error if phone number and email are empty",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
			expectedValidationErrors: map[string]interface{}{
				"phone_number": "phone_number or email is required",
				"email":        "phone_number or email is required",
			},
		},
		{
			name: "error if phone number and email are provided",
			receiverInfo: data.ReceiverRegistrationRequest{
				Email:             "test@stellar.com",
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
			expectedValidationErrors: map[string]interface{}{
				"phone_number": "phone_number and email cannot be both provided",
				"email":        "phone_number and email cannot be both provided",
			},
		},
		{
			name: "error if OTP is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "12mock",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
			expectedValidationErrors: map[string]interface{}{
				"otp": "invalid otp format. Needs to be a 6 digit value",
			},
		},
		{
			name: "error if verification type is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: "mock_type",
			},
			expectedValidationErrors: map[string]interface{}{
				"verification_field": "invalid parameter. valid values are: [DATE_OF_BIRTH YEAR_MONTH PIN NATIONAL_ID_NUMBER]",
			},
		},
		{
			name: "error if verification[DATE_OF_BIRTH] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "90/01/01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
			expectedValidationErrors: map[string]interface{}{
				"verification": "invalid date of birth format. Correct format: 1990-01-30",
			},
		},
		{
			name: "error if verification[YEAR_MONTH] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "90/12",
				VerificationField: data.VerificationTypeYearMonth,
			},
			expectedValidationErrors: map[string]interface{}{
				"verification": "invalid year/month format. Correct format: 1990-12",
			},
		},
		{
			name: "error if verification[PIN] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "ABCDE1234",
				VerificationField: data.VerificationTypePin,
			},
			expectedValidationErrors: map[string]interface{}{
				"verification": "invalid pin length. Cannot have less than 4 or more than 8 characters in pin",
			},
		},
		{
			name: "error if verification[NATIONAL_ID_NUMBER] is invalid",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "6UZMB56FWTKV4U0PJ21TBR6VOQVYSGIMZG2HW2S0L7EK5K83W78XXXXX",
				VerificationField: data.VerificationTypeNationalID,
			},
			expectedValidationErrors: map[string]interface{}{
				"verification": "invalid national id. Cannot have more than 50 characters in national id",
			},
		},
		{
			name: "🎉 successfully validates receiver values [DATE_OF_BIRTH]",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "1990-01-01  ",
				VerificationField: "date_of_birth",
			},
			expectedValidationErrors: map[string]interface{}{},
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-01-01",
				VerificationField: data.VerificationTypeDateOfBirth,
			},
		},
		{
			name: "🎉 successfully validates receiver values [YEAR_MONTH]",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "1990-12  ",
				VerificationField: "year_month",
			},
			expectedValidationErrors: map[string]interface{}{},
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1990-12",
				VerificationField: data.VerificationTypeYearMonth,
			},
		},
		{
			name: "🎉 successfully validates receiver values [PIN]",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "1234  ",
				VerificationField: "pin",
			},
			expectedValidationErrors: map[string]interface{}{},
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "1234",
				VerificationField: data.VerificationTypePin,
			},
		},
		{
			name: "🎉 successfully validates receiver values [NATIONAL_ID_NUMBER]",
			receiverInfo: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555  ",
				OTP:               "  123456  ",
				VerificationValue: "  NATIONALIDNUMBER123",
				VerificationField: "national_id_number",
			},
			expectedValidationErrors: map[string]interface{}{},
			expectedReceiver: data.ReceiverRegistrationRequest{
				PhoneNumber:       "+380445555555",
				OTP:               "123456",
				VerificationValue: "NATIONALIDNUMBER123",
				VerificationField: data.VerificationTypeNationalID,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewReceiverRegistrationValidator()
			validator.ValidateReceiver(&tc.receiverInfo)

			if len(tc.expectedValidationErrors) > 0 {
				assert.Equal(t, tc.expectedValidationErrors, validator.Errors)
			} else {
				assert.Equal(t, tc.expectedReceiver.Email, tc.receiverInfo.Email)
				assert.Equal(t, tc.expectedReceiver.PhoneNumber, tc.receiverInfo.PhoneNumber)
				assert.Equal(t, tc.expectedReceiver.OTP, tc.receiverInfo.OTP)
				assert.Equal(t, tc.expectedReceiver.VerificationValue, tc.receiverInfo.VerificationValue)
				assert.Equal(t, tc.expectedReceiver.VerificationField, tc.receiverInfo.VerificationField)
			}
		})
	}
}

func Test_ReceiverRegistrationValidator_ValidateAndGetVerificationType(t *testing.T) {
	t.Run("Valid verification type", func(t *testing.T) {
		validator := NewReceiverRegistrationValidator()
		validField := []data.VerificationType{
			data.VerificationTypeDateOfBirth,
			data.VerificationTypeYearMonth,
			data.VerificationTypePin,
			data.VerificationTypeNationalID,
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
		assert.Equal(t, "invalid parameter. valid values are: [DATE_OF_BIRTH YEAR_MONTH PIN NATIONAL_ID_NUMBER]", validator.Errors["verification_field"])
	})
}
