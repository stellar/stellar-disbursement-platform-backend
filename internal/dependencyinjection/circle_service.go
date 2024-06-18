package dependencyinjection

import (
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

const CircleServiceInstanceName = "circle_service_instance"

// NewCircleService creates a new circle service instance, or retrieves an instance that was previously created.
func NewCircleService(circleClientFactory circle.ClientFactory, configModel circle.ClientConfigModelInterface, networkType utils.NetworkType, passphrase string) (circle.ServiceInterface, error) {
	instanceName := CircleServiceInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if circleServiceInstance, ok2 := instance.(circle.ServiceInterface); ok2 {
			return circleServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing circle service instance")
	}

	newInstance := circle.NewService(circleClientFactory, configModel, networkType, passphrase)
	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
