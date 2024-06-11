package validators

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var validTenantName *regexp.Regexp = regexp.MustCompile(`^[a-z-]+$`)

type TenantRequest struct {
	Name                    string  `json:"name"`
	OwnerEmail              string  `json:"owner_email"`
	OwnerFirstName          string  `json:"owner_first_name"`
	OwnerLastName           string  `json:"owner_last_name"`
	OrganizationName        string  `json:"organization_name"`
	DistributionAccountType string  `json:"distribution_account_type"`
	BaseURL                 *string `json:"base_url"`
	SDPUIBaseURL            *string `json:"sdp_ui_base_url"`
}

type UpdateTenantRequest struct {
	BaseURL      *string              `json:"base_url"`
	SDPUIBaseURL *string              `json:"sdp_ui_base_url"`
	Status       *tenant.TenantStatus `json:"status"`
}

type DefaultTenantRequest struct {
	ID string `json:"id"`
}

func (r *DefaultTenantRequest) Validate() error {
	r.ID = strings.TrimSpace(r.ID)
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	return nil
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

	tv.validateDistributionAccountType(reqBody.DistributionAccountType)

	var err error
	if reqBody.BaseURL != nil {
		if _, err = url.ParseRequestURI(*reqBody.BaseURL); err != nil {
			tv.Check(false, "base_url", "invalid base URL value")
		}
	}

	if reqBody.SDPUIBaseURL != nil {
		if _, err = url.ParseRequestURI(*reqBody.SDPUIBaseURL); err != nil {
			tv.Check(false, "sdp_ui_base_url", "invalid SDP UI base URL value")
		}
	}

	if tv.HasErrors() {
		return nil
	}

	return reqBody
}

func (tv *TenantValidator) validateDistributionAccountType(distributionAccountType string) {
	tv.Check(distributionAccountType != "", "distribution_account_type", fmt.Sprintf("distribution_account_type is required. valid values are: %v", schema.DistributionAccountTypes()))

	if distributionAccountType != "" {
		switch schema.AccountType(distributionAccountType) {
		case schema.DistributionAccountStellarEnv, schema.DistributionAccountStellarDBVault, schema.DistributionAccountCircleDBVault:
		default:
			tv.Check(false, "distribution_account_type", fmt.Sprintf("invalid distribution_account_type. valid values are: %v", schema.DistributionAccountTypes()))
		}
	}
}

func (tv *TenantValidator) ValidateUpdateTenantRequest(reqBody *UpdateTenantRequest) *UpdateTenantRequest {
	tv.Check(reqBody != nil, "body", "request body is empty")
	if tv.HasErrors() {
		return nil
	}

	var err error
	if reqBody.BaseURL != nil {
		if _, err = url.ParseRequestURI(*reqBody.BaseURL); err != nil {
			tv.Check(false, "base_url", "invalid base URL value")
		}

		if _, ok := tv.Errors["base_url"]; !ok {
			*reqBody.BaseURL, err = utils.GetURLWithScheme(*reqBody.BaseURL)
			if err != nil {
				tv.Check(false, "base_url", "invalid base URL value. Verify the URL protocol scheme")
			}
		}
	}

	if reqBody.SDPUIBaseURL != nil {
		if _, err = url.ParseRequestURI(*reqBody.SDPUIBaseURL); err != nil {
			tv.Check(false, "sdp_ui_base_url", "invalid SDP UI base URL value")
		}

		if _, ok := tv.Errors["sdp_ui_base_url"]; !ok {
			*reqBody.SDPUIBaseURL, err = utils.GetURLWithScheme(*reqBody.SDPUIBaseURL)
			if err != nil {
				tv.Check(false, "sdp_ui_base_url", "invalid SDP UI base URL value. Verify the URL protocol scheme")
			}
		}
	}

	if reqBody.Status != nil {
		tv.Check((*reqBody.Status).IsValid(), "status", "invalid status value")
	}

	if tv.HasErrors() {
		return nil
	}

	return reqBody
}
