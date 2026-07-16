package data

import "fmt"

type UserRole string

func (u UserRole) String() string {
    return string(u)
}

func (u UserRole) IsValid() bool {
    switch u {
    case OwnerUserRole, FinancialControllerUserRole, DeveloperUserRole, BusinessUserRole, InitiatorUserRole, ApproverUserRole, UploaderUserRole, FinanceOfficerUserRole:
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
	// DeveloperUserRole has only configuration permissions. (wallets, assets management. Also, statistics access permission)
	DeveloperUserRole UserRole = "developer"
	// BusinessUserRole has read-only permissions - except for user management that they can't read any data.
	BusinessUserRole UserRole = "business"
	// InitiatorUserRole can create and save disbursements but not submit them. Mutually exclusive with ApproverUserRole.
	InitiatorUserRole UserRole = "initiator"
	// ApproverUserRole can submit disbursements but not create or save new ones. Mutually exclusive with InitiatorUserRole.
	ApproverUserRole UserRole = "approver"
	// UploaderUserRole can create drafts by uploading CSV files.
	UploaderUserRole UserRole = "uploader"
	// FinanceOfficerUserRole can submit approved disbursements to Stellar.
	FinanceOfficerUserRole UserRole = "finance_officer"
)

func GetAllRoles() []UserRole {
    return []UserRole{
        OwnerUserRole,
        FinancialControllerUserRole,
        DeveloperUserRole,
        BusinessUserRole,
        InitiatorUserRole,
        ApproverUserRole,
        UploaderUserRole,
        FinanceOfficerUserRole,
    }
}

func GetBusinessOperationRoles() []UserRole {
	return []UserRole{
		OwnerUserRole,
		FinancialControllerUserRole,
		BusinessUserRole,
		InitiatorUserRole,
		ApproverUserRole,
		UploaderUserRole,
	}
}

func FromUserRoleArrayToStringArray(roles []UserRole) []string {
    rolesString := make([]string, 0, len(roles))
    for _, role := range roles {
        rolesString = append(rolesString, role.String())
    }
    return rolesString
}

func ValidateRoleMutualExclusivity(roles []UserRole) error {
    hasInitiator := false
    hasApprover := false

    for _, role := range roles {
        if role == InitiatorUserRole {
            hasInitiator = true
        }
        if role == ApproverUserRole {
            hasApprover = true
        }
    }

    if hasInitiator && hasApprover {
        return fmt.Errorf("initiator and approver roles are mutually exclusive")
    }

    return nil
}