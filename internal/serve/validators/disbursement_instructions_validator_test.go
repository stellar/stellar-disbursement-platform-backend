package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_DisbursementInstructionsValidator_ValidateAndGetInstruction(t *testing.T) {
	tests := []struct {
		name              string
		instruction       *data.DisbursementInstruction
		lineNumber        int
		verificationField data.VerificationField
		hasErrors         bool
		expectedErrors    map[string]interface{}
	}{
		{
			name: "error if phone number and email are empty",
			instruction: &data.DisbursementInstruction{
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber:        2,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 2 - contact": "phone or email must be provided",
			},
		},
		{
			name:              "error with all fields empty (phone, id, amount, date of birth)",
			instruction:       &data.DisbursementInstruction{},
			lineNumber:        2,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 2 - amount":        "invalid amount. Amount must be a positive number",
				"line 2 - date of birth": "date of birth cannot be empty",
				"line 2 - id":            "id cannot be empty",
				"line 2 - contact":       "phone or email must be provided",
			},
		},
		{
			name: "error if phone number format is invalid",
			instruction: &data.DisbursementInstruction{
				Phone:             "+123-12-345-678",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber:        2,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 2 - phone": "invalid phone format. Correct format: +380445555555",
			},
		},
		{
			name: "error if amount format is invalid",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5USDC",
				VerificationValue: "1990-01-01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - amount": "invalid amount. Amount must be a positive number",
			},
		},
		{
			name: "error if email is not valid",
			instruction: &data.DisbursementInstruction{
				Email:             "invalidemail",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - email": "invalid email format",
			},
		},
		{
			name: "error if amount is not positive",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "-100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - amount": "invalid amount. Amount must be a positive number",
			},
		},
		{
			name: "error if DoB format is invalid",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990/01/01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - date of birth": "invalid date of birth format. Correct format: 1990-01-30",
			},
		},
		{
			name: "error if DoB in the future",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "2090-01-01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - date of birth": "date of birth cannot be in the future",
			},
		},
		{
			name: "error if year month format is invalid",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990/01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldYearMonth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - year/month": "invalid year/month format. Correct format: 1990-12",
			},
		},
		{
			name: "error if year month in the future",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "2090-01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldYearMonth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - year/month": "year/month cannot be in the future",
			},
		},
		{
			name: "error if PIN is invalid - less than 4 characters",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "123",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldPin,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - pin": "invalid pin length. Cannot have less than 4 or more than 8 characters in pin",
			},
		},
		{
			name: "error if PIN is invalid - more than 8 characters",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "123456789",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldPin,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - pin": "invalid pin length. Cannot have less than 4 or more than 8 characters in pin",
			},
		},
		{
			name: "error if NATIONAL_ID_NUMBER is invalid - more than 50 characters",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "6UZMB56FWTKV4U0PJ21TBR6VOQVYSGIMZG2HW2S0L7EK5K83W78",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldNationalID,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - national id": "invalid national id. Cannot have more than 50 characters in national id",
			},
		},
		// VALID CASES
		{
			name: "🎉 successfully validates instructions (DATE_OF_BIRTH)",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber:        1,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         false,
		},
		{
			name: "🎉 successfully validates instructions (YEAR_MONTH)",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01",
			},
			lineNumber:        1,
			verificationField: data.VerificationFieldYearMonth,
			hasErrors:         false,
		},
		{
			name: "🎉 successfully validates instructions (NATIONAL_ID_NUMBER)",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "ABCD123",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldNationalID,
			hasErrors:         false,
		},
		{
			name: "🎉 successfully validates instructions (PIN)",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1234",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldPin,
			hasErrors:         false,
		},
		{
			name: "🎉 successfully validates instructions (Email)",
			instruction: &data.DisbursementInstruction{
				Email:             "myemail@stellar.org",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1234",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldPin,
			hasErrors:         false,
		},
		{
			name: "🎉 successfully validates instructions (Phone)",
			instruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1234",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldPin,
			hasErrors:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv := NewDisbursementInstructionsValidator(tt.verificationField)
			iv.ValidateInstruction(tt.instruction, tt.lineNumber)

			if tt.hasErrors {
				assert.Equal(t, tt.expectedErrors, iv.Errors)
			} else {
				assert.Empty(t, iv.Errors)
			}
		})
	}
}

func Test_DisbursementInstructionsValidator_SanitizeInstruction(t *testing.T) {
	externalPaymentID := "123456789"
	externalPaymentIDWithSpaces := "  123456789  "
	tests := []struct {
		name                string
		actual              *data.DisbursementInstruction
		expectedInstruction *data.DisbursementInstruction
	}{
		{
			name: "Sanitized instruction",
			actual: &data.DisbursementInstruction{
				Phone:             "  +380445555555  ",
				ID:                "  123456789  ",
				Amount:            "  100.5  ",
				VerificationValue: "  1990-01-01  ",
			},
			expectedInstruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
				ExternalPaymentId: "",
			},
		},
		{
			name: "Sanitized instruction with external payment id",
			actual: &data.DisbursementInstruction{
				Phone:             "  +380445555555  ",
				ID:                "  123456789  ",
				Amount:            "  100.5  ",
				VerificationValue: "  1990-01-01  ",
				ExternalPaymentId: externalPaymentIDWithSpaces,
			},
			expectedInstruction: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
				ExternalPaymentId: externalPaymentID,
			},
		},
		{
			name: "Sanitized instruction with email",
			actual: &data.DisbursementInstruction{
				Email:             "   MyEmail@stellar.org  ",
				ID:                "  123456789  ",
				Amount:            "  100.5  ",
				VerificationValue: "  1990-01-01  ",
			},
			expectedInstruction: &data.DisbursementInstruction{
				Email:             "myemail@stellar.org",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv := NewDisbursementInstructionsValidator(data.VerificationFieldDateOfBirth)
			sanitizedInstruction := iv.SanitizeInstruction(tt.actual)

			assert.Equal(t, tt.expectedInstruction, sanitizedInstruction)
		})
	}
}
