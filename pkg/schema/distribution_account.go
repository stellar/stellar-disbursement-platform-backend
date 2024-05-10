package schema

import (
	"slices"
)

type AccountType string

const (
	DistributionAccountStellarEnv     AccountType = "DISTRIBUTION_ACCOUNT.STELLAR.ENV"      // was "ENV_STELLAR"
	DistributionAccountStellarDBVault AccountType = "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT" // was "DB_VAULT_STELLAR"
	DistributionAccountCircleDBVault  AccountType = "DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT"  // was "DB_VAULT_CIRCLE"
)

func (t AccountType) IsStellar() bool {
	return slices.Contains([]AccountType{DistributionAccountStellarEnv, DistributionAccountStellarDBVault}, t)
}

func (t AccountType) IsCircle() bool {
	return slices.Contains([]AccountType{DistributionAccountCircleDBVault}, t)
}

type AccountStatus string

const (
	AccountStatusActive                AccountStatus = "ACTIVE"
	AccountStatusPendingUserActivation AccountStatus = "PENDING_USER_ACTIVATION"
)

type DistributionAccount struct {
	Address string        `json:"address" db:"address"`
	Type    AccountType   `json:"type" db:"type"`
	Status  AccountStatus `json:"status" db:"status"`
}

func (da DistributionAccount) IsStellar() bool {
	return da.Type.IsStellar()
}

func (da DistributionAccount) IsCircle() bool {
	return da.Type.IsCircle()
}

func (da DistributionAccount) IsActive() bool {
	return da.Status == AccountStatusActive
}

func (da DistributionAccount) IsPendingUserActivation() bool {
	return da.Status == AccountStatusPendingUserActivation
}

func NewDefaultStellarDistributionAccount(stellarID string) *DistributionAccount {
	return &DistributionAccount{
		Address: stellarID,
		Type:    DistributionAccountStellarDBVault,
		Status:  AccountStatusActive,
	}
}
