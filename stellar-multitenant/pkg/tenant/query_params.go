package tenant

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

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
	FilterKeyOutStatus FilterKey = "out_status"
	FilterKeyDeleted   FilterKey = "deleted"
	FilterKeyStatus    FilterKey = "status"
	FilterKeyName      FilterKey = "name"
	FilterKeyID        FilterKey = "id"
	FilterKeyNameOrID  FilterKey = "name_or_id"
	FilterKeyIsDefault FilterKey = "is_default"
)

func excludeInactiveTenantsFilters() map[FilterKey]interface{} {
	return map[FilterKey]interface{}{
		FilterKeyOutStatus: schema.DeactivatedTenantStatus,
		FilterKeyDeleted:   true,
	}
}

func getDeactivatedTenantsFilters() map[FilterKey]interface{} {
	return map[FilterKey]interface{}{
		FilterKeyStatus: schema.DeactivatedTenantStatus,
	}
}
