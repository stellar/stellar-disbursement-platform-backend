package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
)

const CircleServiceInstanceName = "circle_service_instance"

// NewCircleService creates a new circle service instance, or retrieves an instance that was previously created.
func NewCircleService(ctx context.Context, opts circle.ServiceOptions) (circle.ServiceInterface, error) {
	instanceName := CircleServiceInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if circleServiceInstance, ok2 := instance.(circle.ServiceInterface); ok2 {
			return circleServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing circle service instance")
	}

	log.Ctx(ctx).Info("⚙️ Setting up Circle Service")
	newInstance, err := circle.NewService(opts)
	if err != nil {
		return nil, fmt.Errorf("creating a new circle service instance: %w", err)
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
