package schema

import (
	"slices"
	"time"
)

type DistributionAccountType string

const (
	DistributionAccountTypeEnvStellar     DistributionAccountType = "ENV_STELLAR"
	DistributionAccountTypeDBVaultStellar DistributionAccountType = "DB_VAULT_STELLAR"
	DistributionAccountTypeDBVaultCircle  DistributionAccountType = "DB_VAULT_CIRCLE"
)

func (t DistributionAccountType) IsStellar() bool {
	return slices.Contains([]DistributionAccountType{DistributionAccountTypeEnvStellar, DistributionAccountTypeDBVaultStellar}, t)
}

func (t DistributionAccountType) IsCircle() bool {
	return slices.Contains([]DistributionAccountType{DistributionAccountTypeDBVaultCircle}, t)
}

type DistributionAccountStatus string

const (
	DistributionAccountStatusActive                DistributionAccountStatus = "ACTIVE"
	DistributionAccountStatusPendingUserActivation DistributionAccountStatus = "PENDING_USER_ACTIVATION"
)

type DistributionAccount struct {
	Address   string                    `json:"address" db:"address"`
	TenantID  string                    `json:"-" db:"tenant_id"`
	Type      DistributionAccountType   `json:"type" db:"type"`
	Status    DistributionAccountStatus `json:"status" db:"status"`
	CreatedAt time.Time                 `json:"-" db:"created_at"`
	UpdatedAt time.Time                 `json:"-" db:"updated_at"`
}

func (da DistributionAccount) IsStellar() bool {
	return da.Type.IsStellar()
}

func (da DistributionAccount) IsCircle() bool {
	return da.Type.IsCircle()
}

func (da DistributionAccount) IsActive() bool {
	return da.Status == DistributionAccountStatusActive
}

func (da DistributionAccount) IsPendingUserActivation() bool {
	return da.Status == DistributionAccountStatusPendingUserActivation
}

func NewDefaultStellarDistributionAccount(stellarID string) *DistributionAccount {
	return &DistributionAccount{
		Address: stellarID,
		Type:    DistributionAccountTypeDBVaultStellar,
		Status:  DistributionAccountStatusActive,
	}
}
