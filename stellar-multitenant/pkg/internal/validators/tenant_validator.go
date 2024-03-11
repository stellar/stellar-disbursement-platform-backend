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
	Name                string                 `json:"name"`
	OwnerEmail          string                 `json:"owner_email"`
	OwnerFirstName      string                 `json:"owner_first_name"`
	OwnerLastName       string                 `json:"owner_last_name"`
	OrganizationName    string                 `json:"organization_name"`
	EmailSenderType     tenant.EmailSenderType `json:"email_sender_type"`
	SMSSenderType       tenant.SMSSenderType   `json:"sms_sender_type"`
	BaseURL             string                 `json:"base_url"`
	SDPUIBaseURL        string                 `json:"sdp_ui_base_url"`
	DistributionAccount string                 `json:"distribution_account"`
}

type UpdateTenantRequest struct {
	EmailSenderType     *tenant.EmailSenderType `json:"email_sender_type"`
	SMSSenderType       *tenant.SMSSenderType   `json:"sms_sender_type"`
	BaseURL             *string                 `json:"base_url"`
	SDPUIBaseURL        *string                 `json:"sdp_ui_base_url"`
	Status              *tenant.TenantStatus    `json:"status"`
	DistributionAccount *string                 `json:"distribution_account"`
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

	if _, err = url.ParseRequestURI(reqBody.BaseURL); err != nil {
		tv.Check(false, "base_url", "invalid base URL value")
	}

	if _, err = url.ParseRequestURI(reqBody.SDPUIBaseURL); err != nil {
		tv.Check(false, "sdp_ui_base_url", "invalid SDP UI base URL value")
	}

	if reqBody.DistributionAccount != "" {
		tv.Check(strkey.IsValidEd25519PublicKey(reqBody.DistributionAccount), "distribution_account", "invalid distribution account value")
	}

	if tv.HasErrors() {
		return nil
	}

	return reqBody
}

func (tv *TenantValidator) ValidateUpdateTenantRequest(reqBody *UpdateTenantRequest) *UpdateTenantRequest {
	tv.Check(reqBody != nil, "body", "request body is empty")
	if tv.HasErrors() {
		return nil
	}

	var err error
	if reqBody.EmailSenderType != nil {
		emailSenderType, emailErr := tenant.ParseEmailSenderType(string(*reqBody.EmailSenderType))
		tv.CheckError(emailErr, "email_sender_type", fmt.Sprintf("invalid email sender type. Expected one of these values: %s", []tenant.EmailSenderType{tenant.AWSEmailSenderType, tenant.DryRunEmailSenderType}))
		reqBody.EmailSenderType = &emailSenderType
	}

	if reqBody.SMSSenderType != nil {
		SMSSenderType, smsErr := tenant.ParseSMSSenderType(string(*reqBody.SMSSenderType))
		tv.CheckError(smsErr, "sms_sender_type", fmt.Sprintf("invalid sms sender type. Expected one of these values: %s", []tenant.SMSSenderType{tenant.TwilioSMSSenderType, tenant.AWSSMSSenderType, tenant.DryRunSMSSenderType}))
		reqBody.SMSSenderType = &SMSSenderType
	}

	if reqBody.BaseURL != nil {
		if _, err = url.ParseRequestURI(*reqBody.BaseURL); err != nil {
			tv.Check(false, "base_url", "invalid base URL value")
		}

		*reqBody.BaseURL, err = utils.GetURLWithScheme(*reqBody.BaseURL)
		if err != nil {
			tv.Check(false, "base_url", "invalid base URL value. Verify the URL protocol scheme")
		}
	}

	if reqBody.SDPUIBaseURL != nil {
		if _, err = url.ParseRequestURI(*reqBody.SDPUIBaseURL); err != nil {
			tv.Check(false, "sdp_ui_base_url", "invalid SDP UI base URL value")
		}

		*reqBody.SDPUIBaseURL, err = utils.GetURLWithScheme(*reqBody.SDPUIBaseURL)
		if err != nil {
			tv.Check(false, "sdp_ui_base_url", "invalid SDP UI base URL value. Verify the URL protocol scheme")
		}
	}

	if reqBody.Status != nil {
		tv.Check((*reqBody.Status).IsValid(), "status", "invalid status value")
	}

	if reqBody.DistributionAccount != nil {
		tv.Check(strkey.IsValidEd25519PublicKey(*reqBody.DistributionAccount), "distribution_account", "invalid distribution account value")
	}

	if tv.HasErrors() {
		return nil
	}

	return reqBody
}
