package services

import (
	"context"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

type DistributionAccountServiceInterface interface {
	GetBalance(ctx context.Context, asset string) (int64, error)
}

type StellarNativeDistributionAccountService struct {
	distributionAccountResolver signing.DistributionAccountResolver
	tenantID                    string
}

func NewStellarNativeDistributionAccountService(distributionAccountResolver signing.DistributionAccountResolver, tenantID string) *StellarNativeDistributionAccountService {
	return &StellarNativeDistributionAccountService{
		distributionAccountResolver: distributionAccountResolver,
		tenantID:                    tenantID,
	}
}

func (s StellarNativeDistributionAccountService) GetBalance(ctx context.Context, asset string) (int64, error) {
	// TODO: Implement this method by calling Horizon
	return 0, nil
}
