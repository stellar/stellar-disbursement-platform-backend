package validators

import (
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

type UserQueryValidator struct {
	QueryValidator
}

var (
	DefaultUserSortField = data.SortFieldEmail
	DefaultUserSortOrder = data.SortOrderASC
	AllowedUserSorts     = []data.SortField{data.SortFieldEmail, data.SortFieldIsActive}
)

// NewUserQueryValidator creates a new UserQueryValidator with the provided configuration.
func NewUserQueryValidator() *UserQueryValidator {
	return &UserQueryValidator{
		QueryValidator: QueryValidator{
			Validator:         NewValidator(),
			DefaultSortField:  DefaultUserSortField,
			DefaultSortOrder:  DefaultUserSortOrder,
			AllowedSortFields: AllowedUserSorts,
		},
	}
}
