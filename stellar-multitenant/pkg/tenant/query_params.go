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
	FilterKeyOutStatus  FilterKey = "out_status"
	FilterKeyOutDeleted FilterKey = "out_deleted"
	FilterKeyStatus     FilterKey = "status"
	FilterKeyName       FilterKey = "name"
	FilterKeyID         FilterKey = "id"
	FilterKeyNameOrID   FilterKey = "name_or_id"
	FilterKeyIsDefault  FilterKey = "is_default"
)

func ExcludeInactiveTenantFilters() map[FilterKey]interface{} {
	return map[FilterKey]interface{}{
		FilterKeyOutStatus:  DeactivatedTenantStatus,
		FilterKeyOutDeleted: true,
	}
}
