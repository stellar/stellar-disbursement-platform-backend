package data

type QueryParams struct {
	Query     string
	Page      int
	PageLimit int
	SortBy    SortField
	SortOrder SortOrder
	Filters   map[FilterKey]interface{}
}

type SortOrder string

const (
	SortOrderASC  SortOrder = "ASC"
	SortOrderDESC SortOrder = "DESC"
)

type SortField string

const (
	SortFieldName      SortField = "name"
	SortFieldEmail     SortField = "email"
	SortFieldIsActive  SortField = "is_active"
	SortFieldCreatedAt SortField = "created_at"
	SortFieldUpdatedAt SortField = "updated_at"
)

type FilterKey string

const (
	FilterKeyStatus          FilterKey = "status"
	FilterKeyReceiverID      FilterKey = "receiver_id"
	FilterKeyCreatedAtAfter  FilterKey = "created_at_after"
	FilterKeyCreatedAtBefore FilterKey = "created_at_before"
)
