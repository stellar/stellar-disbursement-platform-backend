package validators

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stretchr/testify/assert"
)

func Test_QueryValidator_ParseQueryParameters(t *testing.T) {
	tests := []struct {
		name              string
		url               string
		defaultSortBy     data.SortField
		defaultSortOrder  data.SortOrder
		allowedSortFields []data.SortField
		filterKeys        []data.FilterKey
		expectedParams    *data.QueryParams
		hasErrors         bool
		expectedErrors    map[string]interface{}
	}{
		{
			name:             "no query parameters - return default values",
			url:              "http://example.com/test",
			defaultSortBy:    data.SortFieldName,
			defaultSortOrder: data.SortOrderASC,
			expectedParams: &data.QueryParams{
				Query:     "",
				Page:      1,
				PageLimit: 20,
				SortBy:    data.SortFieldName,
				SortOrder: data.SortOrderASC,
				Filters:   map[data.FilterKey]interface{}{},
			},
			hasErrors:      false,
			expectedErrors: map[string]interface{}{},
		},
		{
			name:             "valid query parameters",
			url:              "http://example.com/test?q=hello&page=2&page_limit=10&sort=created_at&direction=desc&status=completed&created_at_after=2020-01-01&created_at_before=2020-01-02",
			defaultSortBy:    data.SortFieldName,
			defaultSortOrder: data.SortOrderASC,
			allowedSortFields: []data.SortField{
				data.SortFieldName,
				data.SortFieldCreatedAt,
			},
			filterKeys: []data.FilterKey{
				data.FilterKeyStatus,
				data.FilterKeyCreatedAtAfter,
				data.FilterKeyCreatedAtBefore,
			},
			expectedParams: &data.QueryParams{
				Query:     "hello",
				Page:      2,
				PageLimit: 10,
				SortBy:    data.SortFieldCreatedAt,
				SortOrder: data.SortOrderDESC,
				Filters: map[data.FilterKey]interface{}{
					data.FilterKeyStatus:          "completed",
					data.FilterKeyCreatedAtAfter:  "2020-01-01",
					data.FilterKeyCreatedAtBefore: "2020-01-02",
				},
			},
			hasErrors:      false,
			expectedErrors: map[string]interface{}{},
		},
		{
			name:           "invalid page value",
			url:            "http://example.com/test?page=abc",
			expectedParams: &data.QueryParams{},
			hasErrors:      true,
			expectedErrors: map[string]interface{}{
				"page": "parameter must be an integer",
			},
		},
		{
			name:           "invalid page_limit value",
			url:            "http://example.com/test?page_limit=abc",
			expectedParams: &data.QueryParams{},
			hasErrors:      true,
			expectedErrors: map[string]interface{}{
				"page_limit": "parameter must be an integer",
			},
		},
		{
			name:           "invalid sort field",
			url:            "http://example.com/test?sort=abc",
			expectedParams: &data.QueryParams{},
			hasErrors:      true,
			expectedErrors: map[string]interface{}{
				"sort": "invalid sort field name",
			},
		},
		{
			name:           "invalid sort order",
			url:            "http://example.com/test?direction=abc",
			expectedParams: &data.QueryParams{},
			hasErrors:      true,
			expectedErrors: map[string]interface{}{
				"direction": "invalid sort order. valid values are 'asc' and 'desc'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			v := &DisbursementQueryValidator{
				QueryValidator: QueryValidator{
					DefaultSortField:  tt.defaultSortBy,
					DefaultSortOrder:  tt.defaultSortOrder,
					AllowedSortFields: tt.allowedSortFields,
					AllowedFilters:    tt.filterKeys,
					Validator:         NewValidator(),
				},
			}
			params := v.ParseParametersFromRequest(req)

			assert.Equal(t, tt.expectedParams, params)
			assert.Equal(t, tt.hasErrors, v.HasErrors())
			assert.Equal(t, tt.expectedErrors, v.Errors)
		})
	}
}

func Test_QueryValidator_ValidateAndGetIntParams(t *testing.T) {
	tests := []struct {
		name         string
		param        string
		url          string
		defaultValue int
		expected     int
		hasError     bool
	}{
		{
			name:         "no parameter",
			param:        "limit",
			url:          "http://example.com/test",
			defaultValue: 10,
			expected:     10,
			hasError:     false,
		},
		{
			name:         "valid parameter",
			param:        "limit",
			url:          "http://example.com/test?limit=5",
			defaultValue: 10,
			expected:     5,
			hasError:     false,
		},
		{
			name:         "invalid parameter",
			param:        "limit",
			url:          "http://example.com/test?limit=abc",
			defaultValue: 10,
			expected:     10,
			hasError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			qv := QueryValidator{
				Validator: NewValidator(),
			}

			actual := qv.validateAndGetIntParams(req, tt.param, tt.defaultValue)

			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.hasError, qv.HasErrors())
		})
	}
}
