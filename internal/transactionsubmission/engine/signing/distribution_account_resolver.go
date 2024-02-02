package signing

// DistributionAccountResolver is an interface that provides the distribution iven the provided keyword.
//
//go:generate mockery --name=DistributionAccountResolver --case=underscore --structname=DistributionAccountResolver
type DistributionAccountResolver interface {
	DistributionAccount(keyword string) string
}
