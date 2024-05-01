package tenant

import (
	"fmt"
	"net/url"
	"time"

	"github.com/stellar/go/strkey"
	"golang.org/x/exp/slices"
)

type Tenant struct {
	ID                  string       `json:"id" db:"id"`
	Name                string       `json:"name" db:"name"`
	BaseURL             *string      `json:"base_url" db:"base_url"`
	SDPUIBaseURL        *string      `json:"sdp_ui_base_url" db:"sdp_ui_base_url"`
	Status              TenantStatus `json:"status" db:"status"`
	DistributionAccount *string      `json:"distribution_account" db:"distribution_account"`
	IsDefault           bool         `json:"is_default" db:"is_default"`
	CreatedAt           time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at" db:"updated_at"`
	DeletedAt           *time.Time   `json:"deleted_at" db:"deleted_at"`
}

type TenantUpdate struct {
	ID                  string        `db:"id"`
	BaseURL             *string       `db:"base_url"`
	SDPUIBaseURL        *string       `db:"sdp_ui_base_url"`
	Status              *TenantStatus `db:"status"`
	DistributionAccount *string       `db:"distribution_account"`
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
	return tu.BaseURL == nil &&
		tu.SDPUIBaseURL == nil &&
		tu.Status == nil &&
		tu.DistributionAccount == nil
}

func isValidURL(u string) bool {
	_, err := url.ParseRequestURI(u)
	return err == nil
}

const (
	// MinTenantDistributionAccountAmount is the minimum amount of the native asset that the host distribution account is allowed to
	// send to the tenant distribution account at a time. It is also used as the default amount to bootstrap a tenant distribution account,
	// when non is specified.
	MinTenantDistributionAccountAmount = 5

	// MaxTenantDistributionAccountAmount is the maximum amount of the native asset that the host distribution account is allowed to
	// send to the tenant distribution account at a time.
	MaxTenantDistributionAccountAmount = 50
)

func ValidateNativeAssetBootstrapAmount(amount int) error {
	if amount < MinTenantDistributionAccountAmount || amount > MaxTenantDistributionAccountAmount {
		if amount <= 0 {
			return fmt.Errorf("invalid amount of native asset to send: %d", amount)
		}

		return fmt.Errorf("amount of native asset to send must be between %d and %d", MinTenantDistributionAccountAmount, MaxTenantDistributionAccountAmount)
	}

	return nil
}
