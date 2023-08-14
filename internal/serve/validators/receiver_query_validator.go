package validators

import (
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type ReceiverQueryValidator struct {
	QueryValidator
}

// NewReceiverQueryValidator creates a new ReceiverQueryValidator with the provided configuration.
func NewReceiverQueryValidator() *ReceiverQueryValidator {
	return &ReceiverQueryValidator{
		QueryValidator: QueryValidator{
			DefaultSortField:  data.DefaultReceiverSortField,
			DefaultSortOrder:  data.DefaultReceiverSortOrder,
			AllowedSortFields: data.AllowedReceiverSorts,
			AllowedFilters:    data.AllowedReceiverFilters,
			Validator:         NewValidator(),
		},
	}
}

// ValidateAndGetReceiverFilters validates the filters and returns a map of valid filters.
func (qv *ReceiverQueryValidator) ValidateAndGetReceiverFilters(filters map[data.FilterKey]interface{}) map[data.FilterKey]interface{} {
	validFilters := make(map[data.FilterKey]interface{})
	if filters[data.FilterKeyStatus] != nil {
		validFilters[data.FilterKeyStatus] = qv.validateAndGetReceiverWalletStatus(filters[data.FilterKeyStatus].(string))
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

// validateAndGetReceiverWalletStatus validates the status parameter and returns the corresponding ReceiverWalletStatus.
func (qv *ReceiverQueryValidator) validateAndGetReceiverWalletStatus(status string) data.ReceiversWalletStatus {
	s := data.ReceiversWalletStatus(strings.ToUpper(status))
	switch s {
	case data.DraftReceiversWalletStatus, data.ReadyReceiversWalletStatus, data.RegisteredReceiversWalletStatus, data.FlaggedReceiversWalletStatus:
		return s
	default:
		qv.Check(false, string(data.FilterKeyStatus), "invalid parameter. valid values are: draft, ready, registered, flagged")
		return ""
	}
}
