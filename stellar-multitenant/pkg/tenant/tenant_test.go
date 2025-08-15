package tenant

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_TenantUpdate_Validate(t *testing.T) {
	t.Run("invalid values", func(t *testing.T) {
		tu := TenantUpdate{}
		err := tu.Validate()
		assert.EqualError(t, err, "tenant ID is required")

		tu.ID = "abc"
		err = tu.Validate()
		assert.EqualError(t, err, "provide at least one field to be updated")

		tu.SDPUIBaseURL = nil
		tenantStatus := schema.TenantStatus("invalid")
		tu.Status = &tenantStatus
		err = tu.Validate()
		assert.EqualError(t, err, `invalid tenant status: "invalid"`)
	})

	t.Run("valid values", func(t *testing.T) {
		tu := TenantUpdate{
			ID:           "abc",
			BaseURL:      &[]string{"https://myorg.backend.io"}[0],
			SDPUIBaseURL: &[]string{"https://myorg.frontend.io"}[0],
			Status:       &[]schema.TenantStatus{schema.ProvisionedTenantStatus}[0],
		}
		err := tu.Validate()
		assert.NoError(t, err)
	})
}

func Test_TenantUpdate_areAllFieldsEmpty(t *testing.T) {
	tu := TenantUpdate{}
	assert.True(t, tu.areAllFieldsEmpty())
	tu.SDPUIBaseURL = &[]string{"https://myorg.backend.io"}[0]
	assert.False(t, tu.areAllFieldsEmpty())
}

func Test_TenantStatus_IsValid(t *testing.T) {
	testCases := []struct {
		status schema.TenantStatus
		expect bool
	}{
		{
			status: schema.CreatedTenantStatus,
			expect: true,
		},
		{
			status: schema.ProvisionedTenantStatus,
			expect: true,
		},
		{
			status: schema.ActivatedTenantStatus,
			expect: true,
		},
		{
			status: schema.DeactivatedTenantStatus,
			expect: true,
		},
		{
			status: schema.TenantStatus("invalid"),
			expect: false,
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expect, tc.status.IsValid())
	}
}
