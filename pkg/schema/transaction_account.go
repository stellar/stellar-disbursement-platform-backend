package schema

import "fmt"

type AccountStatus string

const (
	AccountStatusActive                AccountStatus = "ACTIVE"
	AccountStatusPendingUserActivation AccountStatus = "PENDING_USER_ACTIVATION"
)

// TransactionAccount represents an account that is used for transactions, either directly with the STellar network or with Circle.
type TransactionAccount struct {
	Address        string        `json:"address,omitempty"`
	CircleWalletID string        `json:"circle_wallet_id,omitempty"`
	Type           AccountType   `json:"type"`
	Status         AccountStatus `json:"status"`
}

func (da TransactionAccount) ID() string {
	platform := da.Type.Platform()
	switch platform {
	case StellarPlatform:
		return fmt.Sprintf("%s:%s", platform, da.Address)
	case CirclePlatform:
		return fmt.Sprintf("%s:%s", platform, da.CircleWalletID)
	default:
		panic("unsupported type!")
	}
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

func (da TransactionAccount) String() string {
	return fmt.Sprintf("TransactionAccount{Type: %s, Status: %s, Address: %s}", da.Type, da.Status, da.Address)
}

func NewDefaultStellarTransactionAccount(stellarAddress string) TransactionAccount {
	return TransactionAccount{
		Address: stellarAddress,
		Type:    DistributionAccountStellarDBVault,
		Status:  AccountStatusActive,
	}
}

func NewStellarEnvTransactionAccount(stellarAddress string) TransactionAccount {
	return TransactionAccount{
		Address: stellarAddress,
		Type:    DistributionAccountStellarEnv,
		Status:  AccountStatusActive,
	}
}

func NewDefaultChannelAccount(stellarAddress string) TransactionAccount {
	return TransactionAccount{
		Address: stellarAddress,
		Type:    ChannelAccountStellarDB,
		Status:  AccountStatusActive,
	}
}

func NewDefaultHostAccount(stellarAddress string) TransactionAccount {
	return TransactionAccount{
		Address: stellarAddress,
		Type:    HostStellarEnv,
		Status:  AccountStatusActive,
	}
}
