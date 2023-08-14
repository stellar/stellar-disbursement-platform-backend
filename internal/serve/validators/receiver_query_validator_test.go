package validators

import (
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_ReceiverQueryValidator_ValidateReceiverFilters(t *testing.T) {
	t.Run("Valid filters", func(t *testing.T) {
		validator := NewReceiverQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus:          "draft",
			data.FilterKeyCreatedAtAfter:  "2023-01-01",
			data.FilterKeyCreatedAtBefore: "2023-01-31",
		}

		actual := validator.ValidateAndGetReceiverFilters(filters)

		assert.Equal(t, data.DraftReceiversWalletStatus, actual[data.FilterKeyStatus])
		assert.Equal(t, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), actual[data.FilterKeyCreatedAtAfter])
		assert.Equal(t, time.Date(2023, 1, 31, 0, 0, 0, 0, time.UTC), actual[data.FilterKeyCreatedAtBefore])
	})

	t.Run("Invalid status", func(t *testing.T) {
		validator := NewReceiverQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus: "unknown",
		}

		validator.ValidateAndGetReceiverFilters(filters)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: draft, ready, registered, flagged", validator.Errors["status"])
	})

	t.Run("Invalid date", func(t *testing.T) {
		validator := NewReceiverQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus:          "draft",
			data.FilterKeyCreatedAtAfter:  "00-01-31",
			data.FilterKeyCreatedAtBefore: "00-01-01",
		}

		validator.ValidateAndGetReceiverFilters(filters)

		assert.Equal(t, 2, len(validator.Errors))
		assert.Equal(t, "invalid date format. valid format is 'YYYY-MM-DD'", validator.Errors["created_at_after"])
		assert.Equal(t, "invalid date format. valid format is 'YYYY-MM-DD'", validator.Errors["created_at_before"])
	})

	t.Run("Invalid date range", func(t *testing.T) {
		validator := NewReceiverQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus:          "draft",
			data.FilterKeyCreatedAtAfter:  "2023-01-31",
			data.FilterKeyCreatedAtBefore: "2023-01-01",
		}

		validator.ValidateAndGetReceiverFilters(filters)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "created_at_after must be before created_at_before", validator.Errors["created_at_after"])
	})
}

func Test_ReceiverQueryValidator_ValidateAndGetReceiverStatus(t *testing.T) {
	t.Run("Valid status", func(t *testing.T) {
		validator := NewReceiverQueryValidator()
		validStatus := []data.ReceiversWalletStatus{
			data.DraftReceiversWalletStatus,
			data.ReadyReceiversWalletStatus,
			data.ReadyReceiversWalletStatus,
			data.FlaggedReceiversWalletStatus,
		}
		for _, status := range validStatus {
			assert.Equal(t, status, validator.validateAndGetReceiverWalletStatus(string(status)))
		}
	})

	t.Run("Invalid status", func(t *testing.T) {
		validator := NewReceiverQueryValidator()
		invalidStatus := "unknown"

		actual := validator.validateAndGetReceiverWalletStatus(invalidStatus)
		assert.Empty(t, actual)
		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: draft, ready, registered, flagged", validator.Errors["status"])
	})
}
