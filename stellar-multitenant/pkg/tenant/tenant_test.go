package tenant

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		tenantStatus := TenantStatus("invalid")
		tu.Status = &tenantStatus
		err = tu.Validate()
		assert.EqualError(t, err, `invalid tenant status: "invalid"`)
	})

	t.Run("valid values", func(t *testing.T) {
		tu := TenantUpdate{
			ID:           "abc",
			BaseURL:      &[]string{"https://myorg.backend.io"}[0],
			SDPUIBaseURL: &[]string{"https://myorg.frontend.io"}[0],
			Status:       &[]TenantStatus{ProvisionedTenantStatus}[0],
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
		status TenantStatus
		expect bool
	}{
		{
			status: CreatedTenantStatus,
			expect: true,
		},
		{
			status: ProvisionedTenantStatus,
			expect: true,
		},
		{
			status: ActivatedTenantStatus,
			expect: true,
		},
		{
			status: DeactivatedTenantStatus,
			expect: true,
		},
		{
			status: TenantStatus("invalid"),
			expect: false,
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expect, tc.status.IsValid())
	}
}

func Test_ValidateNativeAssetBootstrapAmount(t *testing.T) {
	testCases := []struct {
		amount int
		errStr string
	}{
		{
			amount: 0,
			errStr: "invalid amount of native asset to send",
		},
		{
			amount: -1,
			errStr: "invalid amount of native asset to send",
		},
		{
			amount: 4,
			errStr: fmt.Sprintf("amount of native asset to send must be between %d and %d", MinTenantDistributionAccountAmount, MaxTenantDistributionAccountAmount),
		},
		{
			amount: 51,
			errStr: fmt.Sprintf("amount of native asset to send must be between %d and %d", MinTenantDistributionAccountAmount, MaxTenantDistributionAccountAmount),
		},
		{
			amount: 20,
		},
	}

	for _, tc := range testCases {
		err := ValidateNativeAssetBootstrapAmount(tc.amount)

		if tc.errStr != "" {
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errStr)
		} else {
			require.NoError(t, err)
		}
	}
}
