package validators

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_DisbursementInstructionsValidator_ValidateAndGetInstruction(t *testing.T) {
	tests := []struct {
		name           string
		actual         *data.DisbursementInstruction
		lineNumber     int
		hasErrors      bool
		expectedErrors map[string]interface{}
	}{
		{
			name: "valid record",
			actual: &data.DisbursementInstruction{
				Phone:             "+380445555555",
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber: 1,
			hasErrors:  false,
		},
		{
			name: "empty phone number",
			actual: &data.DisbursementInstruction{
				ID:                "123456789",
				Amount:            "100.5",
				VerificationValue: "1990-01-01",
			},
			lineNumber: 2,
			hasErrors:  true,
			expectedErrors: map[string]interface{}{
				"line 2 - phone": "phone cannot be empty",
			},
		},
		{
			name:       "empty phone, id, amount and birthday",
			actual:     &data.DisbursementInstruction{},
			lineNumber: 2,
			hasErrors:  true,
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
			lineNumber: 2,
			hasErrors:  true,
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
			lineNumber: 3,
			hasErrors:  true,
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
			lineNumber: 3,
			hasErrors:  true,
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
			lineNumber: 3,
			hasErrors:  true,
			expectedErrors: map[string]interface{}{
				"line 3 - birthday": "invalid date of birth format. Correct format: 1990-01-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv := NewDisbursementInstructionsValidator(data.VerificationFieldDateOfBirth)
			iv.ValidateInstruction(tt.actual, tt.lineNumber)

			if tt.hasErrors {
				assert.Equal(t, tt.expectedErrors, iv.Errors)
			} else {
				assert.Empty(t, iv.Errors)
			}
		})
	}
}
