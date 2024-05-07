package schema

type DistributionAccountType string

const (
	DistributionAccountTypeStellar DistributionAccountType = "STELLAR"
	DistributionAccountTypeCircle  DistributionAccountType = "CIRCLE"
)

type DistributionAccountStatus string

const (
	DistributionAccountStatusActive                DistributionAccountStatus = "ACTIVE"
	DistributionAccountStatusPendingUserActivation DistributionAccountStatus = "PENDING_USER_ACTIVATION"
)

type DistributionAccount struct {
	ID     string                    `json:"id"`
	Type   DistributionAccountType   `json:"type"`
	Status DistributionAccountStatus `json:"status"`
}

func (da DistributionAccount) IsStellar() bool {
	return da.Type == DistributionAccountTypeStellar
}

func (da DistributionAccount) IsCircle() bool {
	return da.Type == DistributionAccountTypeCircle
}

func (da DistributionAccount) IsActive() bool {
	return da.Status == DistributionAccountStatusActive
}

func (da DistributionAccount) IsPendingUserActivation() bool {
	return da.Status == DistributionAccountStatusPendingUserActivation
}

func NewStellarDistributionAccount(stellarID string) *DistributionAccount {
	return &DistributionAccount{
		ID:     stellarID,
		Type:   DistributionAccountTypeStellar,
		Status: DistributionAccountStatusActive,
	}
}
