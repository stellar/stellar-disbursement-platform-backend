package data

type UserRole string

func (u UserRole) String() string {
	return string(u)
}

func (u UserRole) IsValid() bool {
	switch u {
	case OwnerUserRole, FinancialControllerUserRole, DeveloperUserRole, BusinessUserRole, APIOwnerUserRole:
		return true
	}
	return false
}

// Roles description reference: https://stellarfoundation.slack.com/archives/C04C9MLM9UZ/p1681238994830149
const (
	// OwnerUserRole has permission to do everything. Also, it's in charge of creating new users and managing Org account.
	OwnerUserRole UserRole = "owner"
	// FinancialControllerUserRole has the same permissions as the OwnerUserRole except for user management.
	FinancialControllerUserRole UserRole = "financial_controller"
	// DeveloperUserRole has only configuration permissions. (wallets, assets, countries management. Also, statistics access permission)
	DeveloperUserRole UserRole = "developer"
	// BusinessUserRole has read-only permissions - except for user management that they can't read any data.
	BusinessUserRole UserRole = "business"
	// APIOwnerUserRole has permissions to access to the same suite of APIs as OwnerUserRole, without needing to perform verification through reCAPTCHA.
	APIOwnerUserRole UserRole = "api_owner"
)

// GetAllRoles returns all roles available
func GetAllRoles() []UserRole {
	return []UserRole{
		OwnerUserRole,
		FinancialControllerUserRole,
		DeveloperUserRole,
		BusinessUserRole,
		APIOwnerUserRole,
	}
}

// FromUserRoleArrayToStringArray converts an array of UserRole type to an array of string
func FromUserRoleArrayToStringArray(roles []UserRole) []string {
	rolesString := make([]string, 0, len(roles))
	for _, role := range roles {
		rolesString = append(rolesString, role.String())
	}
	return rolesString
}
