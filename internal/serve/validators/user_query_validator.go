package validators

import (
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type UserQueryValidator struct {
	QueryValidator
}

// NewUserQueryValidator creates a new UserQueryValidator with the provided configuration.
func NewUserQueryValidator() *UserQueryValidator {
	return &UserQueryValidator{
		QueryValidator: QueryValidator{
			Validator:         NewValidator(),
			DefaultSortField:  auth.DefaultUserSortField,
			DefaultSortOrder:  auth.DefaultUserSortOrder,
			AllowedSortFields: auth.AllowedUserSorts,
		},
	}
}
