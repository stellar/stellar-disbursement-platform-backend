package validators

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/stellar/go/strkey"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var validTenantName *regexp.Regexp = regexp.MustCompile(`^[a-z-]+$`)

type TenantRequest struct {
	Name                  string                 `json:"name"`
	OwnerEmail            string                 `json:"owner_email"`
	OwnerFirstName        string                 `json:"owner_first_name"`
	OwnerLastName         string                 `json:"owner_last_name"`
	OrganizationName      string                 `json:"organization_name"`
	EmailSenderType       tenant.EmailSenderType `json:"email_sender_type"`
	SMSSenderType         tenant.SMSSenderType   `json:"sms_sender_type"`
	SEP10SigningPublicKey string                 `json:"sep10_signing_public_key"`
	DistributionPublicKey string                 `json:"distribution_public_key"`
	EnableMFA             bool                   `json:"enable_mfa"`
	EnableReCAPTCHA       bool                   `json:"enable_recaptcha"`
	CORSAllowedOrigins    []string               `json:"cors_allowed_origins"`
	BaseURL               string                 `json:"base_url"`
	SDPUIBaseURL          string                 `json:"sdp_ui_base_url"`
}

type TenantValidator struct {
	*Validator
}

func NewTenantValidator() *TenantValidator {
	return &TenantValidator{Validator: NewValidator()}
}

func (tv *TenantValidator) ValidateCreateTenantRequest(reqBody *TenantRequest) *TenantRequest {
	tv.Check(reqBody != nil, "body", "request body is empty")
	if tv.HasErrors() {
		return nil
	}

	tv.Check(validTenantName.MatchString(reqBody.Name), "name", "invalid tenant name. It should only contains lower case letters and dash (-)")
	tv.CheckError(utils.ValidateEmail(reqBody.OwnerEmail), "owner_email", "invalid email")
	tv.Check(reqBody.OwnerFirstName != "", "owner_first_name", "owner_first_name is required")
	tv.Check(reqBody.OwnerLastName != "", "owner_last_name", "owner_last_name is required")
	tv.Check(reqBody.OrganizationName != "", "organization_name", "organization_name is required")

	var err error
	reqBody.EmailSenderType, err = tenant.ParseEmailSenderType(string(reqBody.EmailSenderType))
	tv.CheckError(err, "email_sender_type", fmt.Sprintf("invalid email sender type. Expected one of these values: %s", []tenant.EmailSenderType{tenant.AWSEmailSenderType, tenant.DryRunEmailSenderType}))

	reqBody.SMSSenderType, err = tenant.ParseSMSSenderType(string(reqBody.SMSSenderType))
	tv.CheckError(err, "sms_sender_type", fmt.Sprintf("invalid sms sender type. Expected one of these values: %s", []tenant.SMSSenderType{tenant.TwilioSMSSenderType, tenant.AWSSMSSenderType, tenant.DryRunSMSSenderType}))

	tv.Check(strkey.IsValidEd25519PublicKey(reqBody.SEP10SigningPublicKey), "sep10_signing_public_key", "invalid public key")
	tv.Check(strkey.IsValidEd25519PublicKey(reqBody.DistributionPublicKey), "distribution_public_key", "invalid public key")

	if _, err = url.ParseRequestURI(reqBody.BaseURL); err != nil {
		tv.Check(false, "base_url", "invalid base URL value")
	}

	if _, err = url.ParseRequestURI(reqBody.SDPUIBaseURL); err != nil {
		tv.Check(false, "sdp_ui_base_url", "invalid SDP UI base URL value")
	}

	tv.Check(len(reqBody.CORSAllowedOrigins) != 0, "cors_allowed_origins", "provide at least one CORS allowed origins")
	for i, cors := range reqBody.CORSAllowedOrigins {
		if _, err = url.ParseRequestURI(cors); err != nil {
			tv.Check(false, "cors_allowed_origins", fmt.Sprintf("invalid URL value for cors_allowed_origins[%d]=%s", i, cors))
		}
	}

	if tv.HasErrors() {
		return nil
	}

	return reqBody
}
