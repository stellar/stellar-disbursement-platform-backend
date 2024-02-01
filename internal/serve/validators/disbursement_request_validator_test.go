package validators

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_DisbursementRequestValidator_ValidateAndGetVerificationType(t *testing.T) {
	t.Run("Valid verification type", func(t *testing.T) {
		validField := []data.VerificationField{
			data.VerificationFieldDateOfBirth,
			data.VerificationFieldPin,
			data.VerificationFieldNationalID,
		}
		for _, field := range validField {
			validator := NewDisbursementRequestValidator(field)
			assert.Equal(t, field, validator.ValidateAndGetVerificationType())
		}
	})

	t.Run("Invalid verification type", func(t *testing.T) {
		field := data.VerificationField("field")
		validator := NewDisbursementRequestValidator(field)

		actual := validator.ValidateAndGetVerificationType()
		assert.Empty(t, actual)
		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: DATE_OF_BIRTH, PIN, NATIONAL_ID_NUMBER", validator.Errors["verification_field"])
	})
}
