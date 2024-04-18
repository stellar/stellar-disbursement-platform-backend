package tenant

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

type QueryParams struct {
	Query     string
	Page      int
	PageLimit int
	SortBy    data.SortField
	SortOrder data.SortOrder
	Filters   map[FilterKey]interface{}
}

type FilterKey string

const (
	FilterKeyOutStatus FilterKey = "status"
	FilterKeyName      FilterKey = "name"
	FilterKeyID        FilterKey = "id"
	FilterKeyNameOrID  FilterKey = "name_or_id"
	FilterKeyIsDefault FilterKey = "is_default"
)
