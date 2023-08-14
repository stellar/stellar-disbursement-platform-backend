package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_UpdateReceiverValidator_ValidateReceiver(t *testing.T) {
	t.Run("Empty request", func(t *testing.T) {
		validator := NewUpdateReceiverValidator()

		receiverInfo := UpdateReceiverRequest{}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "request body is empty", validator.Errors["body"])
	})

	t.Run("Invalid date of birth", func(t *testing.T) {
		validator := NewUpdateReceiverValidator()

		receiverInfo := UpdateReceiverRequest{
			DateOfBirth: "invalid",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid date of birth format. Correct format: 1990-01-30", validator.Errors["date_of_birth"])
	})

	t.Run("Invalid pin", func(t *testing.T) {
		validator := NewUpdateReceiverValidator()

		receiverInfo := UpdateReceiverRequest{
			Pin: "   ",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid pin format", validator.Errors["pin"])
	})

	t.Run("Invalid national ID", func(t *testing.T) {
		validator := NewUpdateReceiverValidator()

		receiverInfo := UpdateReceiverRequest{
			NationalID: "   ",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid national ID format", validator.Errors["national_id"])
	})

	t.Run("invalid email", func(t *testing.T) {
		validator := NewUpdateReceiverValidator()

		receiverInfo := UpdateReceiverRequest{
			Email: "invalid",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid email format", validator.Errors["email"])

		receiverInfo = UpdateReceiverRequest{
			Email: "     ",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid email format", validator.Errors["email"])
	})

	t.Run("invalid external ID", func(t *testing.T) {
		validator := NewUpdateReceiverValidator()

		receiverInfo := UpdateReceiverRequest{
			ExternalID: "    ",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid external_id format", validator.Errors["external_id"])
	})

	t.Run("Valid receiver values", func(t *testing.T) {
		validator := NewUpdateReceiverValidator()

		receiverInfo := UpdateReceiverRequest{
			DateOfBirth: "1999-01-01",
			Pin:         "123   ",
			NationalID:  "   12345CODE",
			Email:       "receiver@email.com",
			ExternalID:  "externalID",
		}
		validator.ValidateReceiver(&receiverInfo)

		assert.Equal(t, 0, len(validator.Errors))
		assert.Equal(t, "1999-01-01", receiverInfo.DateOfBirth)
		assert.Equal(t, "123", receiverInfo.Pin)
		assert.Equal(t, "12345CODE", receiverInfo.NationalID)
	})
}
