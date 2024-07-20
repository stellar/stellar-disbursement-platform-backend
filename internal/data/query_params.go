package data

import "fmt"

type QueryParams struct {
	Query               string
	Page                int
	PageLimit           int
	SortBy              SortField
	SortOrder           SortOrder
	Filters             map[FilterKey]interface{}
	ForUpdateSkipLocked bool
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
	FilterKeyPaymentID       FilterKey = "payment_id"
	FilterKeyCompletedAt     FilterKey = "completed_at"
	FilterKeyCreatedAtAfter  FilterKey = "created_at_after"
	FilterKeyCreatedAtBefore FilterKey = "created_at_before"
	FilterKeySyncAttempts    FilterKey = "sync_attempts"
)

func (fk FilterKey) Equals() string {
	return fmt.Sprintf("%s = ?", fk)
}

func (fk FilterKey) LowerThan() string {
	return fmt.Sprintf("%s < ?", fk)
}

// IsNull returns `{filterKey} IS NULL`.
func IsNull(filterKey FilterKey) FilterKey {
	return FilterKey(fmt.Sprintf("%s IS NULL", filterKey))
}

// LowerThan returns `{filterKey} < ?`.
func LowerThan(filterKey FilterKey) FilterKey {
	return FilterKey(fmt.Sprintf("%s < ?", filterKey))
}
