package tenant

import (
	"fmt"
	"net/url"
	"time"

	"github.com/stellar/go/strkey"
	"golang.org/x/exp/slices"
)

type EmailSenderType string

var (
	AWSEmailSenderType    EmailSenderType = "AWS_EMAIL"
	DryRunEmailSenderType EmailSenderType = "DRY_RUN"
)

func ParseEmailSenderType(emailSenderTypeStr string) (EmailSenderType, error) {
	validTypes := []EmailSenderType{AWSEmailSenderType, DryRunEmailSenderType}
	esType := EmailSenderType(emailSenderTypeStr)
	if slices.Contains(validTypes, esType) {
		return esType, nil
	}
	return "", fmt.Errorf("invalid email sender type %q", emailSenderTypeStr)
}

type SMSSenderType string

var (
	TwilioSMSSenderType SMSSenderType = "TWILIO_SMS"
	AWSSMSSenderType    SMSSenderType = "AWS_SMS"
	DryRunSMSSenderType SMSSenderType = "DRY_RUN"
)

func ParseSMSSenderType(smsSenderTypeStr string) (SMSSenderType, error) {
	validTypes := []SMSSenderType{TwilioSMSSenderType, AWSSMSSenderType, DryRunSMSSenderType}
	smsSenderType := SMSSenderType(smsSenderTypeStr)
	if slices.Contains(validTypes, smsSenderType) {
		return smsSenderType, nil
	}
	return "", fmt.Errorf("invalid sms sender type %q", smsSenderTypeStr)
}

type Tenant struct {
	ID                  string          `json:"id" db:"id"`
	Name                string          `json:"name" db:"name"`
	EmailSenderType     EmailSenderType `json:"email_sender_type" db:"email_sender_type"`
	SMSSenderType       SMSSenderType   `json:"sms_sender_type" db:"sms_sender_type"`
	BaseURL             *string         `json:"base_url" db:"base_url"`
	SDPUIBaseURL        *string         `json:"sdp_ui_base_url" db:"sdp_ui_base_url"`
	Status              TenantStatus    `json:"status" db:"status"`
	DistributionAccount *string         `json:"distribution_account" db:"distribution_account"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

type TenantUpdate struct {
	ID                  string           `db:"id"`
	EmailSenderType     *EmailSenderType `db:"email_sender_type"`
	SMSSenderType       *SMSSenderType   `db:"sms_sender_type"`
	BaseURL             *string          `db:"base_url"`
	SDPUIBaseURL        *string          `db:"sdp_ui_base_url"`
	Status              *TenantStatus    `db:"status"`
	DistributionAccount *string          `db:"distribution_account"`
}

type TenantStatus string

const (
	CreatedTenantStatus     TenantStatus = "TENANT_CREATED"
	ProvisionedTenantStatus TenantStatus = "TENANT_PROVISIONED"
	ActivatedTenantStatus   TenantStatus = "TENANT_ACTIVATED"
	DeactivatedTenantStatus TenantStatus = "TENANT_DEACTIVATED"
)

func (s TenantStatus) IsValid() bool {
	validStatuses := []TenantStatus{CreatedTenantStatus, ProvisionedTenantStatus, ActivatedTenantStatus, DeactivatedTenantStatus}
	return slices.Contains(validStatuses, s)
}

func (tu *TenantUpdate) Validate() error {
	if tu.ID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	if tu.areAllFieldsEmpty() {
		return ErrEmptyUpdateTenant
	}

	if tu.EmailSenderType != nil {
		if _, err := ParseEmailSenderType(string(*tu.EmailSenderType)); err != nil {
			return fmt.Errorf("invalid email sender type: %w", err)
		}
	}

	if tu.SMSSenderType != nil {
		if _, err := ParseSMSSenderType(string(*tu.SMSSenderType)); err != nil {
			return fmt.Errorf("invalid SMS sender type: %w", err)
		}
	}

	if tu.BaseURL != nil && !isValidURL(*tu.BaseURL) {
		return fmt.Errorf("invalid base URL")
	}

	if tu.SDPUIBaseURL != nil && !isValidURL(*tu.SDPUIBaseURL) {
		return fmt.Errorf("invalid SDP UI base URL")
	}

	if tu.Status != nil && !tu.Status.IsValid() {
		return fmt.Errorf("invalid tenant status: %q", *tu.Status)
	}

	if tu.DistributionAccount != nil && !strkey.IsValidEd25519PublicKey(*tu.DistributionAccount) {
		return fmt.Errorf("invalid distribution account: %q", *tu.DistributionAccount)
	}

	return nil
}

func (tu *TenantUpdate) areAllFieldsEmpty() bool {
	return (tu.EmailSenderType == nil &&
		tu.SMSSenderType == nil &&
		tu.BaseURL == nil &&
		tu.SDPUIBaseURL == nil &&
		tu.Status == nil &&
		tu.DistributionAccount == nil)
}

func isValidURL(u string) bool {
	_, err := url.ParseRequestURI(u)
	return err == nil
}
