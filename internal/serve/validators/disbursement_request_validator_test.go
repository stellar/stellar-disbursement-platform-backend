package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_DisbursementRequestValidator_ValidateAndGetVerificationType(t *testing.T) {
	t.Run("Valid verification type", func(t *testing.T) {
		validField := []data.VerificationType{
			data.VerificationTypeDateOfBirth,
			data.VerificationTypeYearMonth,
			data.VerificationTypePin,
			data.VerificationTypeNationalID,
		}
		for _, field := range validField {
			validator := NewDisbursementRequestValidator(field)
			assert.Equal(t, field, validator.ValidateAndGetVerificationType())
		}
	})

	t.Run("Invalid verification type", func(t *testing.T) {
		field := data.VerificationType("field")
		validator := NewDisbursementRequestValidator(field)

		actual := validator.ValidateAndGetVerificationType()
		assert.Empty(t, actual)
		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: [DATE_OF_BIRTH YEAR_MONTH PIN NATIONAL_ID_NUMBER]", validator.Errors["verification_field"])
	})
}
