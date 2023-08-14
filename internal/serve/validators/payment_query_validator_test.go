package validators

import (
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_PaymentQueryValidator_ValidateDisbursementFilters(t *testing.T) {
	t.Run("Valid filters", func(t *testing.T) {
		validator := NewPaymentQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus:          "draft",
			data.FilterKeyReceiverID:      "receiver_id",
			data.FilterKeyCreatedAtAfter:  "2023-01-01",
			data.FilterKeyCreatedAtBefore: "2023-01-31",
		}

		actual := validator.ValidateAndGetPaymentFilters(filters)

		assert.Equal(t, data.DraftPaymentStatus, actual[data.FilterKeyStatus])
		assert.Equal(t, "receiver_id", actual[data.FilterKeyReceiverID])
		assert.Equal(t, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), actual[data.FilterKeyCreatedAtAfter])
		assert.Equal(t, time.Date(2023, 1, 31, 0, 0, 0, 0, time.UTC), actual[data.FilterKeyCreatedAtBefore])
	})

	t.Run("Invalid status", func(t *testing.T) {
		validator := NewPaymentQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus: "unknown",
		}

		validator.ValidateAndGetPaymentFilters(filters)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: draft, ready, pending, paused, success, failed", validator.Errors["status"])
	})

	t.Run("Invalid date", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus:          "draft",
			data.FilterKeyCreatedAtAfter:  "00-01-31",
			data.FilterKeyCreatedAtBefore: "00-01-01",
		}

		validator.ValidateAndGetDisbursementFilters(filters)

		assert.Equal(t, 2, len(validator.Errors))
		assert.Equal(t, "invalid date format. valid format is 'YYYY-MM-DD'", validator.Errors["created_at_after"])
		assert.Equal(t, "invalid date format. valid format is 'YYYY-MM-DD'", validator.Errors["created_at_before"])
	})

	t.Run("Invalid date range", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus:          "draft",
			data.FilterKeyCreatedAtAfter:  "2023-01-31",
			data.FilterKeyCreatedAtBefore: "2023-01-01",
		}

		validator.ValidateAndGetDisbursementFilters(filters)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "created_at_after must be before created_at_before", validator.Errors["created_at_after"])
	})
}

func Test_PaymentQueryValidator_ValidateAndGetPaymentStatus(t *testing.T) {
	t.Run("Valid status", func(t *testing.T) {
		validator := NewPaymentQueryValidator()
		validStatus := []data.PaymentStatus{data.DraftPaymentStatus, data.ReadyPaymentStatus, data.PendingPaymentStatus, data.PausedPaymentStatus, data.SuccessPaymentStatus, data.FailedPaymentStatus}
		for _, status := range validStatus {
			assert.Equal(t, status, validator.validateAndGetPaymentStatus(string(status)))
		}
	})

	t.Run("Invalid status", func(t *testing.T) {
		validator := NewPaymentQueryValidator()
		invalidStatus := "unknown"

		actual := validator.validateAndGetPaymentStatus(invalidStatus)
		assert.Empty(t, actual)
		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid values are: draft, ready, pending, paused, success, failed", validator.Errors["status"])
	})
}
