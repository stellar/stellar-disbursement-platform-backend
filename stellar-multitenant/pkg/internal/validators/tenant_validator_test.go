package validators

import (
	"testing"

	"github.com/stellar/go/keypair"
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
			"name":                     "invalid tenant name. It should only contains lower case letters and dash (-)",
			"owner_email":              "invalid email",
			"owner_first_name":         "owner_first_name is required",
			"owner_last_name":          "owner_last_name is required",
			"organization_name":        "organization_name is required",
			"base_url":                 "invalid base URL value",
			"email_sender_type":        "invalid email sender type. Expected one of these values: [AWS_EMAIL DRY_RUN]",
			"sms_sender_type":          "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]",
			"cors_allowed_origins":     "provide at least one CORS allowed origins",
			"network_type":             "invalid network type provided. Expected one of these values: pubnet or testnet",
			"sdp_ui_base_url":          "invalid SDP UI base URL value",
			"sep10_signing_public_key": "invalid public key",
			"distribution_public_key":  "invalid public key",
		}, tv.Errors)

		reqBody.Name = "aid-org"
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"owner_email":              "invalid email",
			"owner_first_name":         "owner_first_name is required",
			"owner_last_name":          "owner_last_name is required",
			"organization_name":        "organization_name is required",
			"base_url":                 "invalid base URL value",
			"email_sender_type":        "invalid email sender type. Expected one of these values: [AWS_EMAIL DRY_RUN]",
			"sms_sender_type":          "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]",
			"cors_allowed_origins":     "provide at least one CORS allowed origins",
			"network_type":             "invalid network type provided. Expected one of these values: pubnet or testnet",
			"sdp_ui_base_url":          "invalid SDP UI base URL value",
			"sep10_signing_public_key": "invalid public key",
			"distribution_public_key":  "invalid public key",
		}, tv.Errors)
	})

	t.Run("returns error when name is invalid", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                  "aid org",
			OwnerEmail:            "owner@email.org",
			OwnerFirstName:        "Owner",
			OwnerLastName:         "Owner",
			OrganizationName:      "Aid Org",
			NetworkType:           "pubnet",
			EmailSenderType:       tenant.AWSEmailSenderType,
			SMSSenderType:         tenant.TwilioSMSSenderType,
			SEP10SigningPublicKey: keypair.MustRandom().Address(),
			DistributionPublicKey: keypair.MustRandom().Address(),
			EnableMFA:             true,
			EnableReCAPTCHA:       true,
			CORSAllowedOrigins:    []string{"*"},
			SDPUIBaseURL:          "http://localhost:3000",
			BaseURL:               "http://localhost:8000",
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
			Name:                  "aid-org",
			OwnerEmail:            "invalid",
			OwnerFirstName:        "",
			OwnerLastName:         "",
			OrganizationName:      "",
			NetworkType:           "pubnet",
			EmailSenderType:       tenant.AWSEmailSenderType,
			SMSSenderType:         tenant.TwilioSMSSenderType,
			SEP10SigningPublicKey: keypair.MustRandom().Address(),
			DistributionPublicKey: keypair.MustRandom().Address(),
			EnableMFA:             true,
			EnableReCAPTCHA:       true,
			CORSAllowedOrigins:    []string{"*"},
			BaseURL:               "http://localhost:8000",
			SDPUIBaseURL:          "http://localhost:3000",
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

	t.Run("validates the network type successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                  "aid-org",
			OwnerEmail:            "owner@email.org",
			OwnerFirstName:        "Owner",
			OwnerLastName:         "Owner",
			OrganizationName:      "Aid Org",
			NetworkType:           "invalid",
			EmailSenderType:       tenant.AWSEmailSenderType,
			SMSSenderType:         tenant.TwilioSMSSenderType,
			SEP10SigningPublicKey: keypair.MustRandom().Address(),
			DistributionPublicKey: keypair.MustRandom().Address(),
			EnableMFA:             true,
			EnableReCAPTCHA:       true,
			CORSAllowedOrigins:    []string{"*"},
			SDPUIBaseURL:          "http://localhost:3000",
			BaseURL:               "http://localhost:8000",
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{"network_type": "invalid network type provided. Expected one of these values: pubnet or testnet"}, tv.Errors)

		reqBody.NetworkType = "pubnet"
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())

		reqBody.NetworkType = "testnet"
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
	})

	t.Run("validates the email sender type successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                  "aid-org",
			OwnerEmail:            "owner@email.org",
			OwnerFirstName:        "Owner",
			OwnerLastName:         "Owner",
			OrganizationName:      "Aid Org",
			NetworkType:           "pubnet",
			EmailSenderType:       "invalid",
			SMSSenderType:         tenant.TwilioSMSSenderType,
			SEP10SigningPublicKey: keypair.MustRandom().Address(),
			DistributionPublicKey: keypair.MustRandom().Address(),
			EnableMFA:             true,
			EnableReCAPTCHA:       true,
			CORSAllowedOrigins:    []string{"*"},
			SDPUIBaseURL:          "http://localhost:3000",
			BaseURL:               "http://localhost:8000",
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
			Name:                  "aid-org",
			OwnerEmail:            "owner@email.org",
			OwnerFirstName:        "Owner",
			OwnerLastName:         "Owner",
			OrganizationName:      "Aid Org",
			NetworkType:           "pubnet",
			EmailSenderType:       tenant.AWSEmailSenderType,
			SMSSenderType:         "invalid",
			SEP10SigningPublicKey: keypair.MustRandom().Address(),
			DistributionPublicKey: keypair.MustRandom().Address(),
			EnableMFA:             true,
			EnableReCAPTCHA:       true,
			CORSAllowedOrigins:    []string{"*"},
			SDPUIBaseURL:          "http://localhost:3000",
			BaseURL:               "http://localhost:8000",
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{"sms_sender_type": "invalid sms sender type. Expected one of these values: [TWILIO_SMS AWS_SMS DRY_RUN]"}, tv.Errors)

		reqBody.SMSSenderType = tenant.DryRunSMSSenderType
		tv.Errors = map[string]interface{}{}
		tv.ValidateCreateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
	})

	t.Run("validates the public keys successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                  "aid-org",
			OwnerEmail:            "owner@email.org",
			OwnerFirstName:        "Owner",
			OwnerLastName:         "Owner",
			OrganizationName:      "Aid Org",
			NetworkType:           "pubnet",
			EmailSenderType:       tenant.AWSEmailSenderType,
			SMSSenderType:         tenant.TwilioSMSSenderType,
			SEP10SigningPublicKey: "invalid",
			DistributionPublicKey: "invalid",
			EnableMFA:             true,
			EnableReCAPTCHA:       true,
			CORSAllowedOrigins:    []string{"*"},
			SDPUIBaseURL:          "http://localhost:3000",
			BaseURL:               "http://localhost:8000",
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"sep10_signing_public_key": "invalid public key",
			"distribution_public_key":  "invalid public key",
		}, tv.Errors)
	})

	t.Run("validates the URLs successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		reqBody := &TenantRequest{
			Name:                  "aid-org",
			OwnerEmail:            "owner@email.org",
			OwnerFirstName:        "Owner",
			OwnerLastName:         "Owner",
			OrganizationName:      "Aid Org",
			NetworkType:           "pubnet",
			EmailSenderType:       tenant.AWSEmailSenderType,
			SMSSenderType:         tenant.TwilioSMSSenderType,
			SEP10SigningPublicKey: keypair.MustRandom().Address(),
			DistributionPublicKey: keypair.MustRandom().Address(),
			EnableMFA:             true,
			EnableReCAPTCHA:       true,
			CORSAllowedOrigins:    []string{"http://valid.com", "%invalid%"},
			SDPUIBaseURL:          "%invalid%",
			BaseURL:               "%invalid%",
		}

		tv.ValidateCreateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"base_url":             "invalid base URL value",
			"sdp_ui_base_url":      "invalid SDP UI base URL value",
			"cors_allowed_origins": "invalid URL value for cors_allowed_origins[1] = %invalid%",
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
			SEP10SigningPublicKey: &invalidValue,
			DistributionPublicKey: &invalidValue,
			CORSAllowedOrigins:    []string{invalidValue},
			BaseURL:               &invalidValue,
			SDPUIBaseURL:          &invalidValue,
		}
		tv.ValidateUpdateTenantRequest(reqBody)
		assert.True(t, tv.HasErrors())
		assert.Equal(t, map[string]interface{}{
			"base_url":                 "invalid base URL value",
			"cors_allowed_origins":     "invalid URL value for cors_allowed_origins[0] = invalid",
			"distribution_public_key":  "invalid public key",
			"sdp_ui_base_url":          "invalid SDP UI base URL value",
			"sep10_signing_public_key": "invalid public key",
		}, tv.Errors)
	})

	t.Run("validates request body successfully", func(t *testing.T) {
		tv := NewTenantValidator()
		key := keypair.MustRandom().Address()
		enable := false
		url := "http://valid.com"
		reqBody := &UpdateTenantRequest{
			EmailSenderType:       &tenant.AWSEmailSenderType,
			SMSSenderType:         &tenant.AWSSMSSenderType,
			SEP10SigningPublicKey: &key,
			DistributionPublicKey: &key,
			EnableMFA:             &enable,
			EnableReCAPTCHA:       &enable,
			CORSAllowedOrigins:    []string{url},
			BaseURL:               &url,
			SDPUIBaseURL:          &url,
		}
		tv.ValidateUpdateTenantRequest(reqBody)
		assert.False(t, tv.HasErrors())
		assert.Empty(t, tv.Errors)
	})
}
