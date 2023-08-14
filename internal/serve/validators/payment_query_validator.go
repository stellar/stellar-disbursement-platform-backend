package validators

import (
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type PaymentQueryValidator struct {
	QueryValidator
}

// NewPaymentQueryValidator creates a new PaymentQueryValidator with the provided configuration.
func NewPaymentQueryValidator() *PaymentQueryValidator {
	return &PaymentQueryValidator{
		QueryValidator: QueryValidator{
			DefaultSortField:  data.DefaultPaymentSortField,
			DefaultSortOrder:  data.DefaultPaymentSortOrder,
			AllowedSortFields: data.AllowedPaymentSorts,
			AllowedFilters:    data.AllowedPaymentFilters,
			Validator:         NewValidator(),
		},
	}
}

// ValidateAndGetPaymentFilters validates the filters and returns a map of valid filters.
func (qv *PaymentQueryValidator) ValidateAndGetPaymentFilters(filters map[data.FilterKey]interface{}) map[data.FilterKey]interface{} {
	validFilters := make(map[data.FilterKey]interface{})
	if filters[data.FilterKeyStatus] != nil {
		validFilters[data.FilterKeyStatus] = qv.validateAndGetPaymentStatus(filters[data.FilterKeyStatus].(string))
	}
	if filters[data.FilterKeyReceiverID] != nil {
		validFilters[data.FilterKeyReceiverID] = filters[data.FilterKeyReceiverID]
	}

	createdAtAfter := qv.ValidateAndGetTimeParams(string(data.FilterKeyCreatedAtAfter), filters[data.FilterKeyCreatedAtAfter])
	createdAtBefore := qv.ValidateAndGetTimeParams(string(data.FilterKeyCreatedAtBefore), filters[data.FilterKeyCreatedAtBefore])

	if qv.HasErrors() {
		return validFilters
	}

	if !createdAtAfter.IsZero() && !createdAtBefore.IsZero() {
		qv.Check(createdAtAfter.Before(createdAtBefore), string(data.FilterKeyCreatedAtAfter), "created_at_after must be before created_at_before")
	}

	if !createdAtAfter.IsZero() {
		validFilters[data.FilterKeyCreatedAtAfter] = createdAtAfter
	}
	if !createdAtBefore.IsZero() {
		validFilters[data.FilterKeyCreatedAtBefore] = createdAtBefore
	}
	return validFilters
}

// validateAndGetPaymentStatus validates the status parameter and returns the corresponding PaymentStatus.
func (qv *PaymentQueryValidator) validateAndGetPaymentStatus(status string) data.PaymentStatus {
	s := data.PaymentStatus(strings.ToUpper(status))
	switch s {
	case data.DraftPaymentStatus, data.ReadyPaymentStatus, data.PendingPaymentStatus, data.PausedPaymentStatus, data.SuccessPaymentStatus, data.FailedPaymentStatus:
		return s
	default:
		qv.Check(false, string(data.FilterKeyStatus), "invalid parameter. valid values are: draft, ready, pending, paused, success, failed")
		return ""
	}
}
