package validators

import (
	"fmt"
	"slices"
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
func (qv *PaymentQueryValidator) ValidateAndGetPaymentFilters(filters map[data.FilterKey]any) map[data.FilterKey]any {
	validFilters := make(map[data.FilterKey]any)
	if filters[data.FilterKeyStatus] != nil {
		validFilters[data.FilterKeyStatus] = qv.validateAndGetPaymentStatus(filters[data.FilterKeyStatus].(string))
	}
	if filters[data.FilterKeyReceiverID] != nil {
		validFilters[data.FilterKeyReceiverID] = filters[data.FilterKeyReceiverID]
	}
	if filters[data.FilterKeyPaymentType] != nil {
		validFilters[data.FilterKeyPaymentType] = qv.validateAndGetPaymentType(filters[data.FilterKeyPaymentType].(string))
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
	if slices.Contains(data.PaymentStatuses(), s) {
		return s
	}

	qv.Check(false, string(data.FilterKeyStatus), fmt.Sprintf("invalid parameter. valid values are: %v", data.PaymentStatuses()))
	return ""
}

func (qv *PaymentQueryValidator) validateAndGetPaymentType(typeParam string) data.PaymentType {
	switch strings.ToUpper(strings.TrimSpace(typeParam)) {
	case "DIRECT":
		return data.PaymentTypeDirect
	case "DISBURSEMENT":
		return data.PaymentTypeDisbursement
	default:
		qv.Check(false, string(data.FilterKeyPaymentType), fmt.Sprintf("invalid payment type '%s'. Must be 'direct' or 'disbursement'", typeParam))
		return ""
	}
}
