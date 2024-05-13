package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/clients/horizonclient"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const DistributionAccountServiceInstanceName = "distribution_account_service_instance"

func NewDistributionAccountService(ctx context.Context, horizonClient horizonclient.ClientInterface) (services.DistributionAccountServiceInterface, error) {
	instanceName := DistributionAccountServiceInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if distributionAccountServiceInstance, ok2 := instance.(services.DistributionAccountServiceInterface); ok2 {
			return distributionAccountServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast a new distribution account service instance")
	}

	newInstance := services.NewDistributionAccountService(horizonClient)
	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
