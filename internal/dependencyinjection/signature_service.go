package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
)

const SignatureServiceInstanceName = "signature_service_instance"

// NewSignatureService creates a new signature service instance, or retrives an instance that was already
// created before.
func NewSignatureService(ctx context.Context, opts engine.DefaultSignatureServiceOptions) (engine.SignatureService, error) {
	instanceName := SignatureServiceInstanceName

	// Already initialized
	if instance, ok := dependenciesStoreMap[instanceName]; ok {
		if signatureServiceInstance, ok := instance.(engine.SignatureService); ok {
			return signatureServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing signature service instance")
	}

	// Setup a new signature service instance
	newSignatureService, err := engine.NewDefaultSignatureServiceNew(opts)
	if err != nil {
		return nil, fmt.Errorf("creating a new signature service instance: %w", err)
	}

	setInstance(instanceName, newSignatureService)

	return newSignatureService, nil
}
