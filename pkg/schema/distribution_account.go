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

// TransactionAccount represents an account that is used for transactions, either directly with the STellar network or with Circle.
type TransactionAccount struct {
	Address string        `json:"address" db:"address"`
	Type    AccountType   `json:"type" db:"type"`
	Status  AccountStatus `json:"status" db:"status"`
}

func (da TransactionAccount) IsStellar() bool {
	return da.Type.IsStellar()
}

func (da TransactionAccount) IsCircle() bool {
	return da.Type.IsCircle()
}

func (da TransactionAccount) IsActive() bool {
	return da.Status == AccountStatusActive
}

func (da TransactionAccount) IsPendingUserActivation() bool {
	return da.Status == AccountStatusPendingUserActivation
}

func NewDefaultStellarTransactionAccount(stellarID string) *TransactionAccount {
	return &TransactionAccount{
		Address: stellarID,
		Type:    DistributionAccountStellarDBVault,
		Status:  AccountStatusActive,
	}
}
