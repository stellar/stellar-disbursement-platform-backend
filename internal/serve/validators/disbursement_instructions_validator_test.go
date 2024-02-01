package validators

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_DisbursementInstructionsValidator_ValidateAndGetInstruction(t *testing.T) {
	tests := []struct {
		name              string
		actual            *data.DisbursementInstruction
		lineNumber        int
		verificationField data.VerificationField
		hasErrors         bool
		expectedErrors    map[string]interface{}
	}{
		{
			name: "valid record",
			actual: &data.DisbursementInstruction{
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
			name: "empty phone number",
			actual: &data.DisbursementInstruction{
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber:        2,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 2 - phone": "phone cannot be empty",
			},
		},
		{
			name:              "empty phone, id, amount and birthday",
			actual:            &data.DisbursementInstruction{},
			lineNumber:        2,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 2 - amount":   "invalid amount. Amount must be a positive number",
				"line 2 - birthday": "invalid date of birth format. Correct format: 1990-01-01",
				"line 2 - id":       "id cannot be empty",
				"line 2 - phone":    "phone cannot be empty",
			},
		},
		{
			name: "invalid phone number",
			actual: &data.DisbursementInstruction{
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
			name: "invalid amount format",
			actual: &data.DisbursementInstruction{
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
			name: "amount must be positive",
			actual: &data.DisbursementInstruction{
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
			name: "invalid birthday format",
			actual: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990/01/01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - birthday": "invalid date of birth format. Correct format: 1990-01-01",
			},
		},
		{
			name: "date of birth in the future",
			actual: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "2090-01-01",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldDateOfBirth,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - birthday": "date of birth cannot be in the future",
			},
		},
		{
			name: "valid pin",
			actual: &data.DisbursementInstruction{
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
			name: "invalid pin - less than 4 characters",
			actual: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "123",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldPin,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - pin": "invalid pin. Cannot have less than 4 or more than 8 characters in pin",
			},
		},
		{
			name: "invalid pin - more than 8 characters",
			actual: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "123456789",
			},
			lineNumber:        3,
			verificationField: data.VerificationFieldPin,
			hasErrors:         true,
			expectedErrors: map[string]interface{}{
				"line 3 - pin": "invalid pin. Cannot have less than 4 or more than 8 characters in pin",
			},
		},
		{
			name: "valid national id",
			actual: &data.DisbursementInstruction{
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
			name: "invalid national - more than 50 characters",
			actual: &data.DisbursementInstruction{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv := NewDisbursementInstructionsValidator(tt.verificationField)
			iv.ValidateInstruction(tt.actual, tt.lineNumber)

			if tt.hasErrors {
				assert.Equal(t, tt.expectedErrors, iv.Errors)
			} else {
				assert.Empty(t, iv.Errors)
			}
		})
	}
}
