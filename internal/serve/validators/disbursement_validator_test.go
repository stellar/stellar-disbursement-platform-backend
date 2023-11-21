package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_DisbursementUpdateValidator_ValidateDisbursementStatus(t *testing.T) {
	testCases := []struct {
		name              string
		verificationValue string
		verificationType  data.VerificationField
		returnErr         bool
		errStr            string
	}{
		{
			name:              "Invalid date of birth",
			verificationValue: "invalid",
			verificationType:  data.VerificationFieldDateOfBirth,
			returnErr:         true,
			errStr:            "invalid date of birth format. Correct format: 1990-01-01",
		},
		{
			name:              "Invalid date of birth - date in the future",
			verificationValue: "4000-01-01",
			verificationType:  data.VerificationFieldDateOfBirth,
			returnErr:         true,
			errStr:            "date of birth cannot be in the future",
		},
		{
			name:              "Invalid pin - fewer than 4 digits",
			verificationValue: "123",
			verificationType:  data.VerificationFieldPin,
			returnErr:         true,
			errStr:            "invalid pin. Cannot have less than 4 or more than 8 characters in pin",
		},
		{
			name:              "Invalid pin - more than 8 digits",
			verificationValue: "123456789",
			verificationType:  data.VerificationFieldPin,
			returnErr:         true,
			errStr:            "invalid pin. Cannot have less than 4 or more than 8 characters in pin",
		},
		{
			name:              "Invalid national id",
			verificationValue: "6UZMB56FWTKV4U0PJ21TBR6VOQVYSGIMZG2HW2S0L7EK5K83W78",
			verificationType:  data.VerificationFieldNationalID,
			returnErr:         true,
			errStr:            "invalid national id. Cannot have more than 50 characters in national id",
		},
		{
			name:              "Valid date of birth",
			verificationValue: "1999-01-01",
			verificationType:  data.VerificationFieldDateOfBirth,
			returnErr:         false,
		},
		{
			name:              "Valid pin",
			verificationValue: "1234",
			verificationType:  data.VerificationFieldPin,
			returnErr:         false,
		},
		{
			name:              "Valid national id",
			verificationValue: "ABC123",
			verificationType:  data.VerificationFieldNationalID,
			returnErr:         false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewDisbursementValidator()

			disbursementReq := data.PostDisbursementRequest{
				VerificationValue: tc.verificationValue,
				VerificationType:  tc.verificationType,
			}
			validator.ValidateDisbursement(&disbursementReq)

			if tc.returnErr {
				assert.Equal(t, 1, len(validator.Errors))
				assert.Equal(t, tc.errStr, validator.Errors["verification"])
			}
		})
	}
}
