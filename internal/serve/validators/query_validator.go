package validators

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"golang.org/x/exp/slices"
)

type QueryValidator struct {
	*Validator
	DefaultSortField  data.SortField
	DefaultSortOrder  data.SortOrder
	AllowedSortFields []data.SortField
	AllowedFilters    []data.FilterKey
}

// ParseParametersFromRequest parses query parameters from the request and returns a QueryParams struct.
func (qv *QueryValidator) ParseParametersFromRequest(r *http.Request) *data.QueryParams {
	page := qv.validateAndGetIntParams(r, "page", 1)
	pageLimit := qv.validateAndGetIntParams(r, "page_limit", 20)

	query := r.URL.Query()
	sortBy := data.SortField(query.Get("sort"))
	if sortBy == "" {
		sortBy = qv.DefaultSortField
	} else if !slices.Contains(qv.AllowedSortFields, sortBy) {
		qv.addError("sort", "invalid sort field name")
	}

	sortOrder := data.SortOrder(strings.ToUpper(query.Get("direction")))
	if sortOrder == "" {
		sortOrder = qv.DefaultSortOrder
	} else if sortOrder != data.SortOrderASC && sortOrder != data.SortOrderDESC {
		qv.addError("direction", "invalid sort order. valid values are 'asc' and 'desc'")
	}

	filters := make(map[data.FilterKey]interface{})
	for _, fk := range qv.AllowedFilters {
		value := strings.TrimSpace(query.Get(string(fk)))
		if value != "" {
			filters[fk] = value
		}
	}

	if qv.HasErrors() {
		return &data.QueryParams{}
	}

	return &data.QueryParams{
		Query:     strings.TrimSpace(query.Get("q")),
		Page:      page,
		PageLimit: pageLimit,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Filters:   filters,
	}
}

// validateAndGetIntParams validates the query parameter and returns the value as an integer.
func (qv *QueryValidator) validateAndGetIntParams(r *http.Request, param string, defaultValue int) int {
	value := r.URL.Query().Get(param)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		qv.CheckError(err, param, "parameter must be an integer")
		return defaultValue
	}

	return intValue
}

// ValidateAndGetTimeParams validates the query parameter and returns the value as a time.Time.
func (qv *QueryValidator) ValidateAndGetTimeParams(param string, value interface{}) time.Time {
	if value == nil {
		return time.Time{}
	}

	dateStr, ok := value.(string)
	if !ok {
		qv.Check(false, param, "invalid date format. valid format is 'YYYY-MM-DD'")
		return time.Time{}
	}

	dateParam, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		qv.Check(false, param, "invalid date format. valid format is 'YYYY-MM-DD'")
		return time.Time{}
	}

	return dateParam
}
