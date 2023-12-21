package validators

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
)

func TestTenantValidator_ValidateCreateTenantRequest(t *testing.T) {
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
			"name":              "invalid tenant name. It should only contains lower case letters and dash (-)",
			"owner_email":       "invalid email",
			"owner_first_name":  "owner_first_name is required",
			"owner_last_name":   "owner_last_name is required",
			"organization_name": "organization_name is required",
			"base_url":          "invalid base URL value",
			"email_sender_type": "invalid email sender type. Expected one of these values: [AWS_EMAIL DRY_RUN]",
			"sms_sender_type":   "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]",
			"sdp_ui_base_url":   "invalid SDP UI base URL value",
		}, tv.Errors)

		reqBody.Name = "aid-org"
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"owner_email":       "invalid email",
			"owner_first_name":  "owner_first_name is required",
			"owner_last_name":   "owner_last_name is required",
			"organization_name": "organization_name is required",
			"base_url":          "invalid base URL value",
			"email_sender_type": "invalid email sender type. Expected one of these values: [AWS_EMAIL DRY_RUN]",
			"sms_sender_type":   "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]",
			"sdp_ui_base_url":   "invalid SDP UI base URL value",
		}, tv.Errors)
	})

	t.Run("returns error when name is invalid", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:             "aid org",
			OwnerEmail:       "owner@email.org",
			OwnerFirstName:   "Owner",
			OwnerLastName:    "Owner",
			OrganizationName: "Aid Org",
			EmailSenderType:  tenant.AWSEmailSenderType,
			SMSSenderType:    tenant.TwilioSMSSenderType,
			EnableMFA:        true,
			EnableReCAPTCHA:  true,
			SDPUIBaseURL:     "http://localhost:3000",
			BaseURL:          "http://localhost:8000",
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
			Name:             "aid-org",
			OwnerEmail:       "invalid",
			OwnerFirstName:   "",
			OwnerLastName:    "",
			OrganizationName: "",
			EmailSenderType:  tenant.AWSEmailSenderType,
			SMSSenderType:    tenant.TwilioSMSSenderType,
			EnableMFA:        true,
			EnableReCAPTCHA:  true,
			BaseURL:          "http://localhost:8000",
			SDPUIBaseURL:     "http://localhost:3000",
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

	t.Run("validates the email sender type successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:             "aid-org",
			OwnerEmail:       "owner@email.org",
			OwnerFirstName:   "Owner",
			OwnerLastName:    "Owner",
			OrganizationName: "Aid Org",
			EmailSenderType:  "invalid",
			SMSSenderType:    tenant.TwilioSMSSenderType,
			EnableMFA:        true,
			EnableReCAPTCHA:  true,
			SDPUIBaseURL:     "http://localhost:3000",
			BaseURL:          "http://localhost:8000",
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{"email_sender_type": "invalid email sender type. Expected one of these values: [AWS_EMAIL DRY_RUN]"}, tv.Errors)

		reqBody.EmailSenderType = tenant.DryRunEmailSenderType
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
	})

	t.Run("validates the sms sender type successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:             "aid-org",
			OwnerEmail:       "owner@email.org",
			OwnerFirstName:   "Owner",
			OwnerLastName:    "Owner",
			OrganizationName: "Aid Org",
			EmailSenderType:  tenant.AWSEmailSenderType,
			SMSSenderType:    "invalid",
			EnableMFA:        true,
			EnableReCAPTCHA:  true,
			SDPUIBaseURL:     "http://localhost:3000",
			BaseURL:          "http://localhost:8000",
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{"sms_sender_type": "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]"}, tv.Errors)

		reqBody.SMSSenderType = tenant.DryRunSMSSenderType
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
	})

	t.Run("validates the URLs successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:             "aid-org",
			OwnerEmail:       "owner@email.org",
			OwnerFirstName:   "Owner",
			OwnerLastName:    "Owner",
			OrganizationName: "Aid Org",
			EmailSenderType:  tenant.AWSEmailSenderType,
			SMSSenderType:    tenant.TwilioSMSSenderType,
			EnableMFA:        true,
			EnableReCAPTCHA:  true,
			SDPUIBaseURL:     "%invalid%",
			BaseURL:          "%invalid%",
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"base_url":        "invalid base URL value",
			"sdp_ui_base_url": "invalid SDP UI base URL value",
		}, tv.Errors)
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
			BaseURL:      &invalidValue,
			SDPUIBaseURL: &invalidValue,
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
		enable := false
		url := "http://valid.com"
		reqBody := &UpdateTenantRequest{
			EmailSenderType: &tenant.AWSEmailSenderType,
			SMSSenderType:   &tenant.AWSSMSSenderType,
			EnableMFA:       &enable,
			EnableReCAPTCHA: &enable,
			BaseURL:         &url,
			SDPUIBaseURL:    &url,
		}
		tv.ValidateUpdateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
		assert.Empty(t, tv.Errors)
	})
}
