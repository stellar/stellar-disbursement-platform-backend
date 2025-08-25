package schema

import (
	"slices"
	"time"
)

type Tenant struct {
	ID           string       `json:"id" db:"id"`
	Name         string       `json:"name" db:"name"`
	BaseURL      *string      `json:"base_url" db:"base_url"`
	SDPUIBaseURL *string      `json:"sdp_ui_base_url" db:"sdp_ui_base_url"`
	Status       TenantStatus `json:"status" db:"status"`
	IsDefault    bool         `json:"is_default" db:"is_default"`
	CreatedAt    time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time   `json:"deleted_at" db:"deleted_at"`
	// Distribution Account fields:
	DistributionAccountAddress *string       `json:"distribution_account_address" db:"distribution_account_address"`
	DistributionAccountType    AccountType   `json:"distribution_account_type" db:"distribution_account_type"`
	DistributionAccountStatus  AccountStatus `json:"distribution_account_status" db:"distribution_account_status"`
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
