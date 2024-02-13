package signing

// DistributionAccountResolver is an interface that provides the distribution iven the provided keyword.
//
//go:generate mockery --name=DistributionAccountResolver --case=underscore --structname=MockDistributionAccountResolver
type DistributionAccountResolver interface {
	NetworkPassphrase() string
	DistributionAccount() string
	HostDistributionAccount() string
}
