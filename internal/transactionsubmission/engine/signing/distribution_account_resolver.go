package signing

import (
	"fmt"

	"github.com/stellar/go/strkey"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

// DistributionAccountResolver is an interface that provides the distribution iven the provided keyword.
//
//go:generate mockery --name=DistributionAccountResolver --case=underscore --structname=MockDistributionAccountResolver
type DistributionAccountResolver interface {
	DistributionAccount() string
	HostDistributionAccount() string
}

type DistributionAccountResolverConfig struct {
	AdminDBConnectionPool            db.DBConnectionPool
	HostDistributionAccountPublicKey string
}

func (c DistributionAccountResolverConfig) Validate() error {
	if c.AdminDBConnectionPool == nil {
		return fmt.Errorf("AdminDBConnectionPool cannot be nil")
	}

	if c.HostDistributionAccountPublicKey == "" {
		return fmt.Errorf("HostDistributionAccountPublicKey cannot be empty")
	}
	if !strkey.IsValidEd25519PublicKey(c.HostDistributionAccountPublicKey) {
		return fmt.Errorf("HostDistributionAccountPublicKey is not a valid ed25519 public key")
	}

	return nil
}

func NewDistributionAccountResolver(config DistributionAccountResolverConfig) (DistributionAccountResolver, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validating config in NewDistributionAccountResolver: %w", err)
	}

	return &DistributionAccountResolverImpl{
		dbConnectionPool:              config.AdminDBConnectionPool,
		hostDistributionAccountPubKey: config.HostDistributionAccountPublicKey,
	}, nil
}

var _ DistributionAccountResolver = (*DistributionAccountResolverImpl)(nil)

// DistributionAccountResolverImpl is a DistributionAccountResolver that resolves the distribution account from the database.
type DistributionAccountResolverImpl struct {
	dbConnectionPool              db.DBConnectionPool
	hostDistributionAccountPubKey string
}

// DistributionAccount returns the distribution account from the database.
func (r *DistributionAccountResolverImpl) DistributionAccount() string {
	return "TODO"
}

// HostDistributionAccount returns the host distribution account from the database.
func (r *DistributionAccountResolverImpl) HostDistributionAccount() string {
	return r.hostDistributionAccountPubKey
}
