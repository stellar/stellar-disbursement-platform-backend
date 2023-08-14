package validators

import (
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_DisbursementQueryValidator_ValidateDisbursementFilters(t *testing.T) {
	t.Run("Valid filters", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus:          "draft",
			data.FilterKeyCreatedAtAfter:  "2023-01-01",
			data.FilterKeyCreatedAtBefore: "2023-01-31",
		}

		actual := validator.ValidateAndGetDisbursementFilters(filters)

		assert.Equal(t, []data.DisbursementStatus{data.DraftDisbursementStatus}, actual[data.FilterKeyStatus])
		assert.Equal(t, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), actual[data.FilterKeyCreatedAtAfter])
		assert.Equal(t, time.Date(2023, 1, 31, 0, 0, 0, 0, time.UTC), actual[data.FilterKeyCreatedAtBefore])
	})

	t.Run("Invalid status", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		filters := map[data.FilterKey]interface{}{
			data.FilterKeyStatus: "unknown",
		}

		validator.ValidateAndGetDisbursementFilters(filters)

		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid value is a comma separate list of statuses: draft, ready, started, paused, completed", validator.Errors["status"])
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

func Test_DisbursementQueryValidator_ValidateAndGetDisbursementStatuses(t *testing.T) {
	t.Run("Valid status", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		validStatus := []data.DisbursementStatus{data.DraftDisbursementStatus, data.ReadyDisbursementStatus, data.StartedDisbursementStatus, data.PausedDisbursementStatus, data.CompletedDisbursementStatus}
		for _, status := range validStatus {
			assert.Equal(t, []data.DisbursementStatus{status}, validator.validateAndGetDisbursementStatuses(string(status)))
		}
	})

	t.Run("Invalid status", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		invalidStatus := "unknown"

		actual := validator.validateAndGetDisbursementStatuses(invalidStatus)
		assert.Empty(t, actual)
		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid value is a comma separate list of statuses: draft, ready, started, paused, completed", validator.Errors["status"])
	})

	t.Run("mix of valid and invalid statuses", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		statuses := "unknown1,unknown2,draft"

		actual := validator.validateAndGetDisbursementStatuses(statuses)
		assert.Equal(t, 1, len(actual))
		assert.Equal(t, []data.DisbursementStatus{data.DraftDisbursementStatus}, actual)
		assert.Equal(t, 1, len(validator.Errors))
		assert.Equal(t, "invalid parameter. valid value is a comma separate list of statuses: draft, ready, started, paused, completed", validator.Errors["status"])
	})

	t.Run("valid comma separated list of statuses", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		statuses := "draft,ready,completed"

		actual := validator.validateAndGetDisbursementStatuses(statuses)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, []data.DisbursementStatus{data.DraftDisbursementStatus, data.ReadyDisbursementStatus, data.CompletedDisbursementStatus}, actual)
		assert.Equal(t, 0, len(validator.Errors))
	})

	t.Run("valid comma separated list of statuses with spaces", func(t *testing.T) {
		validator := NewDisbursementQueryValidator()
		statuses := "   draft ,  ready , completed "

		actual := validator.validateAndGetDisbursementStatuses(statuses)
		assert.Equal(t, 3, len(actual))
		assert.Equal(t, []data.DisbursementStatus{data.DraftDisbursementStatus, data.ReadyDisbursementStatus, data.CompletedDisbursementStatus}, actual)
		assert.Equal(t, 0, len(validator.Errors))
	})
}
