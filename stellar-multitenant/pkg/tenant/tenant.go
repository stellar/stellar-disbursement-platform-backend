package tenant

import (
	"fmt"
	"net/url"
	"time"

	"github.com/lib/pq"
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
	ID                    string          `json:"id" db:"id"`
	Name                  string          `json:"name" db:"name"`
	EmailSenderType       EmailSenderType `json:"email_sender_type" db:"email_sender_type"`
	SMSSenderType         SMSSenderType   `json:"sms_sender_type" db:"sms_sender_type"`
	SEP10SigningPublicKey *string         `json:"sep10_signing_public_key" db:"sep10_signing_public_key"`
	DistributionPublicKey *string         `json:"distribution_public_key" db:"distribution_public_key"`
	EnableMFA             bool            `json:"enable_mfa" db:"enable_mfa"`
	EnableReCAPTCHA       bool            `json:"enable_recaptcha" db:"enable_recaptcha"`
	CORSAllowedOrigins    pq.StringArray  `json:"cors_allowed_origins" db:"cors_allowed_origins"`
	BaseURL               *string         `json:"base_url" db:"base_url"`
	SDPUIBaseURL          *string         `json:"sdp_ui_base_url" db:"sdp_ui_base_url"`
	Status                TenantStatus    `json:"status" db:"status"`
	CreatedAt             time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at" db:"updated_at"`
}

type TenantUpdate struct {
	ID                    string           `db:"id"`
	EmailSenderType       *EmailSenderType `db:"email_sender_type"`
	SMSSenderType         *SMSSenderType   `db:"sms_sender_type"`
	SEP10SigningPublicKey *string          `db:"sep10_signing_public_key"`
	DistributionPublicKey *string          `db:"distribution_public_key"`
	EnableMFA             *bool            `db:"enable_mfa"`
	EnableReCAPTCHA       *bool            `db:"enable_recaptcha"`
	CORSAllowedOrigins    []string         `db:"cors_allowed_origins"`
	BaseURL               *string          `db:"base_url"`
	SDPUIBaseURL          *string          `db:"sdp_ui_base_url"`
	Status                *TenantStatus    `db:"status"`
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
		return fmt.Errorf("provide at least one field to be updated")
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

	if tu.SEP10SigningPublicKey != nil && !strkey.IsValidEd25519PublicKey(*tu.SEP10SigningPublicKey) {
		return fmt.Errorf("invalid SEP-10 signing public key")
	}

	if tu.DistributionPublicKey != nil && !strkey.IsValidEd25519PublicKey(*tu.DistributionPublicKey) {
		return fmt.Errorf("invalid distribution public key")
	}

	if tu.BaseURL != nil && !isValidURL(*tu.BaseURL) {
		return fmt.Errorf("invalid base URL")
	}

	if tu.SDPUIBaseURL != nil && !isValidURL(*tu.SDPUIBaseURL) {
		return fmt.Errorf("invalid SDP UI base URL")
	}

	for _, u := range tu.CORSAllowedOrigins {
		if !isValidURL(u) {
			return fmt.Errorf("invalid CORS allowed origin url: %q", u)
		}
	}

	if tu.Status != nil && !tu.Status.IsValid() {
		return fmt.Errorf("invalid tenant status: %q", *tu.Status)
	}

	return nil
}

func (tu *TenantUpdate) areAllFieldsEmpty() bool {
	return (tu.EmailSenderType == nil &&
		tu.SMSSenderType == nil &&
		tu.SEP10SigningPublicKey == nil &&
		tu.DistributionPublicKey == nil &&
		tu.EnableMFA == nil &&
		tu.EnableReCAPTCHA == nil &&
		tu.CORSAllowedOrigins == nil &&
		tu.BaseURL == nil &&
		tu.SDPUIBaseURL == nil &&
		tu.Status == nil)
}

func isValidURL(u string) bool {
	_, err := url.ParseRequestURI(u)
	return err == nil
}
