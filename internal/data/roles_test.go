package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_UserRole_IsValid(t *testing.T) {
	testCases := []struct {
		role     UserRole
		expected bool
	}{
		{OwnerUserRole, true},
		{FinancialControllerUserRole, true},
		{DeveloperUserRole, true},
		{BusinessUserRole, true},
		{InitiatorUserRole, true},
		{ApproverUserRole, true},
		{UserRole("invalid"), false},
		{UserRole(""), false},
		{UserRole("unknown"), false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.role), func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.role.IsValid())
		})
	}
}

func Test_GetAllRoles(t *testing.T) {
	roles := GetAllRoles()
	expectedRoles := []UserRole{
		OwnerUserRole,
		FinancialControllerUserRole,
		DeveloperUserRole,
		BusinessUserRole,
		InitiatorUserRole,
		ApproverUserRole,
	}

	assert.Equal(t, len(expectedRoles), len(roles))

	for _, expectedRole := range expectedRoles {
		assert.Contains(t, roles, expectedRole)
	}
}

func Test_ValidateRoleMutualExclusivity(t *testing.T) {
	testCases := []struct {
		name          string
		roles         []UserRole
		expectedError string
	}{
		{
			name:          "initiator and approver roles are mutually exclusive",
			roles:         []UserRole{InitiatorUserRole, ApproverUserRole},
			expectedError: "initiator and approver roles are mutually exclusive",
		},
		{
			name:          "single initiator role is not mutually exclusive",
			roles:         []UserRole{InitiatorUserRole},
			expectedError: "",
		},
		{
			name:          "single approver role is not mutually exclusive",
			roles:         []UserRole{ApproverUserRole},
			expectedError: "",
		},
		{
			name:          "owner role with initiator is not mutually exclusive",
			roles:         []UserRole{OwnerUserRole, InitiatorUserRole},
			expectedError: "",
		},
		{
			name:          "owner role with approver is not mutually exclusive",
			roles:         []UserRole{OwnerUserRole, ApproverUserRole},
			expectedError: "",
		},
		{
			name:          "all non-initiator-approver roles are not mutually exclusive",
			roles:         []UserRole{OwnerUserRole, FinancialControllerUserRole, DeveloperUserRole, BusinessUserRole},
			expectedError: "",
		},
		{
			name:          "empty roles are not mutually exclusive",
			roles:         []UserRole{},
			expectedError: "",
		},
		{
			name:          "three roles including both initiator and approver are mutually exclusive",
			roles:         []UserRole{OwnerUserRole, InitiatorUserRole, ApproverUserRole},
			expectedError: "initiator and approver roles are mutually exclusive",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRoleMutualExclusivity(tc.roles)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())
			}
		})
	}
}

func Test_FromUserRoleArrayToStringArray(t *testing.T) {
	roles := []UserRole{OwnerUserRole, InitiatorUserRole, ApproverUserRole}
	expected := []string{"owner", "initiator", "approver"}

	result := FromUserRoleArrayToStringArray(roles)
	assert.Equal(t, expected, result)
}
