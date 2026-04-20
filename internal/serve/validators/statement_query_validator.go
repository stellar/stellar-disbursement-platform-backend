package validators

import (
	"net/http"
	"strings"
	"time"
)

// StatementQueryParams holds validated query parameters for GET /reports/statement.
type StatementQueryParams struct {
	AssetCode         string
	FromDate          time.Time
	ToDate            time.Time
	OperatedByBaseURL string // optional
}

// StatementQueryValidator validates query parameters for GET /reports/statement.
type StatementQueryValidator struct {
	QueryValidator
}

// NewStatementQueryValidator creates a new StatementQueryValidator.
func NewStatementQueryValidator() *StatementQueryValidator {
	return &StatementQueryValidator{
		QueryValidator: QueryValidator{
			Validator: NewValidator(),
		},
	}
}

// ValidateAndGetStatementParams validates the request query and returns statement params.
// Required: from_date, to_date. Optional: asset_code (empty = all assets). Dates must be YYYY-MM-DD; from_date must be <= to_date.
func (v *StatementQueryValidator) ValidateAndGetStatementParams(r *http.Request) StatementQueryParams {
	query := r.URL.Query()
	assetCode := strings.TrimSpace(query.Get("asset_code"))
	fromDateStr := strings.TrimSpace(query.Get("from_date"))
	toDateStr := strings.TrimSpace(query.Get("to_date"))

	var fromDate, toDate time.Time
	if fromDateStr != "" {
		fromDate = v.ValidateAndGetTimeParams("from_date", fromDateStr)
	} else {
		v.Check(false, "from_date", "from_date is required")
	}
	if toDateStr != "" {
		toDate = v.ValidateAndGetTimeParams("to_date", toDateStr)
	} else {
		v.Check(false, "to_date", "to_date is required")
	}

	if !v.HasErrors() && !fromDate.IsZero() && !toDate.IsZero() {
		// to_date is inclusive; allow same day
		v.Check(!fromDate.After(toDate), "from_date", "from_date must be before or equal to to_date")
	}

	operatedByBaseURL := strings.TrimSpace(query.Get("base_url"))

	return StatementQueryParams{
		AssetCode:         assetCode,
		FromDate:          fromDate,
		ToDate:            toDate,
		OperatedByBaseURL: operatedByBaseURL,
	}
}
