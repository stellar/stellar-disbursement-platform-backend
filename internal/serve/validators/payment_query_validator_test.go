package validators

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
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
		assert.Equal(t, fmt.Sprintf("invalid parameter. valid values are: %v", data.PaymentStatuses()), validator.Errors["status"])
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
		assert.Equal(t, fmt.Sprintf("invalid parameter. valid values are: %v", data.PaymentStatuses()), validator.Errors["status"])
	})
}

func TestPaymentQueryValidator_PaymentTypeFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		paymentType   string
		expectedType  data.PaymentType
		expectedError bool
		errorField    string
	}{
		{
			name:         "valid direct type",
			paymentType:  "direct",
			expectedType: data.PaymentTypeDirect,
		},
		{
			name:         "valid disbursement type",
			paymentType:  "disbursement",
			expectedType: data.PaymentTypeDisbursement,
		},
		{
			name:         "case insensitive - DIRECT",
			paymentType:  "DIRECT",
			expectedType: data.PaymentTypeDirect,
		},
		{
			name:         "case insensitive - DISBURSEMENT",
			paymentType:  "DISBURSEMENT",
			expectedType: data.PaymentTypeDisbursement,
		},
		{
			name:          "invalid type - chaos",
			paymentType:   "chaos",
			expectedError: true,
			errorField:    "type",
		},
		{
			name:          "invalid type - empty",
			paymentType:   "",
			expectedError: true,
			errorField:    "type",
		},
		{
			name:          "invalid type - numbers",
			paymentType:   "123",
			expectedError: true,
			errorField:    "type",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			validator := NewPaymentQueryValidator()

			filters := map[data.FilterKey]any{
				data.FilterKeyPaymentType: tc.paymentType,
			}

			validatedFilters := validator.ValidateAndGetPaymentFilters(filters)

			if tc.expectedError {
				assert.True(t, validator.HasErrors())
				assert.Contains(t, validator.Errors, tc.errorField)
			} else {
				assert.False(t, validator.HasErrors())
				assert.Equal(t, tc.expectedType, validatedFilters[data.FilterKeyPaymentType])
			}
		})
	}
}
