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

type DistributionAccountStatus string

const (
	DistributionAccountStatusActive                DistributionAccountStatus = "ACTIVE"
	DistributionAccountStatusPendingUserActivation DistributionAccountStatus = "PENDING_USER_ACTIVATION"
)

type DistributionAccount struct {
	Address string                    `json:"address" db:"address"`
	Type    AccountType               `json:"type" db:"type"`
	Status  DistributionAccountStatus `json:"status" db:"status"`
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
		Type:    DistributionAccountStellarDBVault,
		Status:  DistributionAccountStatusActive,
	}
}
