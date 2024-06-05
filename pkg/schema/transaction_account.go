package schema

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

func NewDefaultStellarTransactionAccount(stellarID string) TransactionAccount {
	return TransactionAccount{
		Address: stellarID,
		Type:    DistributionAccountStellarDBVault,
		Status:  AccountStatusActive,
	}
}
