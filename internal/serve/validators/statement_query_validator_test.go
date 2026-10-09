package validators

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_StatementQueryValidator_ValidateAndGetStatementParams(t *testing.T) {
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jan31 := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	testCases := []struct {
		name          string
		query         string
		expectedError string // field name that must carry an error; empty = valid
		expected      StatementQueryParams
	}{
		{"all params", "asset_code=XLM&from_date=2026-01-01&to_date=2026-01-31", "", StatementQueryParams{AssetCode: "XLM", FromDate: jan1, ToDate: jan31}},
		{"asset_code is optional", "from_date=2026-01-01&to_date=2026-01-31", "", StatementQueryParams{FromDate: jan1, ToDate: jan31}},
		{"single-day range", "from_date=2026-01-01&to_date=2026-01-01", "", StatementQueryParams{FromDate: jan1, ToDate: jan1}},
		{"missing from_date", "to_date=2026-01-31", "from_date", StatementQueryParams{}},
		{"missing to_date", "from_date=2026-01-01", "to_date", StatementQueryParams{}},
		{"malformed from_date", "from_date=01/01/2026&to_date=2026-01-31", "from_date", StatementQueryParams{}},
		{"malformed to_date", "from_date=2026-01-01&to_date=31-01-2026", "to_date", StatementQueryParams{}},
		{"from_date after to_date", "from_date=2026-02-01&to_date=2026-01-31", "from_date", StatementQueryParams{}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/reports/statement?"+tc.query, nil)
			v := NewStatementQueryValidator()
			params := v.ValidateAndGetStatementParams(req)
			if tc.expectedError != "" {
				assert.True(t, v.HasErrors())
				assert.Contains(t, v.Validator.Errors, tc.expectedError)
				return
			}
			assert.False(t, v.HasErrors(), v.Validator.Errors)
			assert.Equal(t, tc.expected, params)
		})
	}
}
