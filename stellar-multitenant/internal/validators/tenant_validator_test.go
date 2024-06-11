package validators

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func TestTenantValidator_ValidateCreateTenantRequest(t *testing.T) {
	sdpUIBaseURL := "http://localhost:3000"
	baseURL := "http://localhost:8000"
	invalidURL := "%invalid%"

	t.Run("returns error when request body is empty", func(t *testing.T) {
		tv := NewTenantValidator()
		tv.ValidateCreateTenantRequest(nil)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{"body": "request body is empty"}, tv.Errors)
	})

	t.Run("returns error when request body has empty fields", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"name":                      "invalid tenant name. It should only contains lower case letters and dash (-)",
			"owner_email":               "invalid email",
			"owner_first_name":          "owner_first_name is required",
			"owner_last_name":           "owner_last_name is required",
			"organization_name":         "organization_name is required",
			"distribution_account_type": "distribution_account_type is required. DISTRIBUTION_ACCOUNT.STELLAR.ENV, DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT, DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT",
		}, tv.Errors)

		reqBody.Name = "aid-org"
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"owner_email":               "invalid email",
			"owner_first_name":          "owner_first_name is required",
			"owner_last_name":           "owner_last_name is required",
			"organization_name":         "organization_name is required",
			"distribution_account_type": "distribution_account_type is required",
		}, tv.Errors)
	})

	t.Run("returns error when name is invalid", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                    "aid org",
			OwnerEmail:              "owner@email.org",
			OwnerFirstName:          "Owner",
			OwnerLastName:           "Owner",
			OrganizationName:        "Aid Org",
			DistributionAccountType: string(schema.DistributionAccountStellarEnv),
			SDPUIBaseURL:            &sdpUIBaseURL,
			BaseURL:                 &baseURL,
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"name": "invalid tenant name. It should only contains lower case letters and dash (-)",
		}, tv.Errors)
	})

	t.Run("returns error when owner info is invalid", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                    "aid-org",
			OwnerEmail:              "invalid",
			OwnerFirstName:          "",
			OwnerLastName:           "",
			OrganizationName:        "",
			DistributionAccountType: string(schema.DistributionAccountStellarEnv),
			BaseURL:                 &sdpUIBaseURL,
			SDPUIBaseURL:            &baseURL,
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"owner_email":       "invalid email",
			"owner_first_name":  "owner_first_name is required",
			"owner_last_name":   "owner_last_name is required",
			"organization_name": "organization_name is required",
		}, tv.Errors)

		reqBody.OwnerEmail = "owner@email.org"
		reqBody.OwnerFirstName = "Owner"
		reqBody.OwnerLastName = "Owner"
		reqBody.OrganizationName = "Aid Org"
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{}, tv.Errors)
	})

	t.Run("returns error when distribution account type is invalid", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                    "aid-org",
			OwnerEmail:              "owner@email.org",
			OwnerFirstName:          "Owner",
			OwnerLastName:           "Owner",
			OrganizationName:        "Aid Org",
			DistributionAccountType: "foobar",
			SDPUIBaseURL:            &sdpUIBaseURL,
			BaseURL:                 &baseURL,
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"distribution_account_type": "invalid distribution account type. valid values are: DISTRIBUTION_ACCOUNT.STELLAR.ENV, DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT, DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT",
		}, tv.Errors)

		for _, accountType := range []string{string(schema.DistributionAccountStellarEnv), string(schema.DistributionAccountStellarDBVault), string(schema.DistributionAccountCircleDBVault)} {
			reqBody.DistributionAccountType = accountType
			tv.Errors = map[string]interface{}{}
			tv.ValidateCreateTenantRequest(reqBody)
			assert.False(t, tv.HasErrors())
			assert.Equal(t, map[string]interface{}{}, tv.Errors)
		}
	})

	t.Run("validates the URLs successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                    "aid-org",
			OwnerEmail:              "owner@email.org",
			OwnerFirstName:          "Owner",
			OwnerLastName:           "Owner",
			OrganizationName:        "Aid Org",
			DistributionAccountType: string(schema.DistributionAccountStellarEnv),
			SDPUIBaseURL:            &invalidURL,
			BaseURL:                 &invalidURL,
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"base_url":        "invalid base URL value",
			"sdp_ui_base_url": "invalid SDP UI base URL value",
		}, tv.Errors)
	})

	t.Run("validates request successfully without URLs", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                    "aid-org",
			OwnerEmail:              "owner@email.org",
			OwnerFirstName:          "Owner",
			OwnerLastName:           "Owner",
			OrganizationName:        "Aid Org",
			DistributionAccountType: string(schema.DistributionAccountStellarEnv),
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{}, tv.Errors)
	})
}

func TestTenantValidator_ValidateUpdateTenantRequest(t *testing.T) {
	t.Run("returns error when request body is empty", func(t *testing.T) {
		tv := NewTenantValidator()
		tv.ValidateUpdateTenantRequest(nil)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{"body": "request body is empty"}, tv.Errors)

		reqBody := &UpdateTenantRequest{}
		tv.ValidateUpdateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{"body": "request body is empty"}, tv.Errors)
	})

	t.Run("returns error when fields are invalid", func(t *testing.T) {
		tv := NewTenantValidator()
		invalidValue := "invalid"
		reqBody := &UpdateTenantRequest{
			BaseURL:      &[]string{invalidValue}[0],
			SDPUIBaseURL: &[]string{invalidValue}[0],
		}
		tv.ValidateUpdateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"base_url":        "invalid base URL value",
			"sdp_ui_base_url": "invalid SDP UI base URL value",
		}, tv.Errors)
	})

	t.Run("validates request body successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		url := "valid.com:3000"
		reqBody := &UpdateTenantRequest{
			BaseURL:      &url,
			SDPUIBaseURL: &url,
		}
		tv.ValidateUpdateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
		assert.Empty(t, tv.Errors)
	})
}
