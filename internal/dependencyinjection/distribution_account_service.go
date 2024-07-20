package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const DistributionAccountServiceInstanceName = "distribution_account_service_instance"

func NewDistributionAccountService(ctx context.Context, opts services.DistributionAccountServiceOptions) (services.DistributionAccountServiceInterface, error) {
	instanceName := DistributionAccountServiceInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if distributionAccountServiceInstance, ok2 := instance.(services.DistributionAccountServiceInterface); ok2 {
			return distributionAccountServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast a new distribution account service instance")
	}

	log.Ctx(ctx).Info("⚙️ Setting up Distribution Account Service")
	newInstance, err := services.NewDistributionAccountService(opts)
	if err != nil {
		return nil, fmt.Errorf("initializing new distribution account service: %w", err)
	}
	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
