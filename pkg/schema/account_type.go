package schema

import "slices"

type AccountType string

const (
	HostStellarEnv                    AccountType = "HOST.STELLAR.ENV"
	ChannelAccountStellarDB           AccountType = "CHANNEL_ACCOUNT.STELLAR.DB"
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
