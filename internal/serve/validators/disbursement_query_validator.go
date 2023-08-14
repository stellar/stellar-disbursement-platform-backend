package validators

import (
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type DisbursementQueryValidator struct {
	QueryValidator
}

// NewDisbursementQueryValidator creates a new DisbursementQueryValidator with the provided configuration.
func NewDisbursementQueryValidator() *DisbursementQueryValidator {
	return &DisbursementQueryValidator{
		QueryValidator: QueryValidator{
			DefaultSortField:  data.DefaultDisbursementSortField,
			DefaultSortOrder:  data.DefaultDisbursementSortOrder,
			AllowedSortFields: data.AllowedDisbursementSorts,
			AllowedFilters:    data.AllowedDisbursementFilters,
			Validator:         NewValidator(),
		},
	}
}

// ValidateAndGetDisbursementFilters validates the filters and returns a map of valid filters.
func (qv *DisbursementQueryValidator) ValidateAndGetDisbursementFilters(filters map[data.FilterKey]interface{}) map[data.FilterKey]interface{} {
	validFilters := make(map[data.FilterKey]interface{})
	if filters[data.FilterKeyStatus] != nil {
		validFilters[data.FilterKeyStatus] = qv.validateAndGetDisbursementStatuses(filters[data.FilterKeyStatus].(string))
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

// validateAndGetDisbursementStatuses takes a comma-separated string of disbursement statuses
// and returns a slice of valid DisbursementStatus values.
func (qv *DisbursementQueryValidator) validateAndGetDisbursementStatuses(statuses string) []data.DisbursementStatus {
	statusList := strings.Split(statuses, ",")
	validStatuses := []data.DisbursementStatus{}

	for _, status := range statusList {
		s := data.DisbursementStatus(strings.ToUpper(strings.TrimSpace(status)))
		switch s {
		case data.DraftDisbursementStatus, data.ReadyDisbursementStatus, data.StartedDisbursementStatus, data.PausedDisbursementStatus, data.CompletedDisbursementStatus:
			validStatuses = append(validStatuses, s)
		default:
			qv.Check(false, string(data.FilterKeyStatus), "invalid parameter. valid value is a comma separate list of statuses: draft, ready, started, paused, completed")
		}
	}
	return validStatuses
}
