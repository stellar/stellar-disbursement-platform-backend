package engine

// DistributionAccountResolver is an interface that provides the distribution iven the provided keyword.
//
//go:generate mockery --name=DistributionAccountResolver --case=underscore --structname=MockDistributionAccountResolver
type DistributionAccountResolver interface {
	DistributionAccount() string
	HostDistributionAccount() string
}
