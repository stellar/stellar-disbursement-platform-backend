package schema

type DistributionAccountType string

const (
	DistributionAccountTypeStellar DistributionAccountType = "STELLAR"
	DistributionAccountTypeCircle  DistributionAccountType = "CIRCLE"
)

type DistributionAccount struct {
	ID     string
	Type   DistributionAccountType
	Status string
}
