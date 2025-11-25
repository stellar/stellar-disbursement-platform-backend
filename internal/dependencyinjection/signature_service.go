package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

const SignatureServiceInstanceName = "signature_service_instance"

// NewSignatureService creates a new signature service instance, or retrieves an instance that was already
// created before.
func NewSignatureService(ctx context.Context, opts signing.SignatureServiceOptions) (signing.SignatureService, error) {
	instanceName := SignatureServiceInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if signatureServiceInstance, ok2 := instance.(signing.SignatureService); ok2 {
			return signatureServiceInstance, nil
		}
		return signing.SignatureService{}, fmt.Errorf("trying to cast an existing signature service instance")
	}

	// TODO: in SDP-1077, implement a `NewDistributionAccountResolver` in the depencency injection and inject it into
	// the SignatureServiceOptions before calling NewSignatureService.
	log.Ctx(ctx).Info("⚙️ Setting up Signature Service")
	newInstance, err := signing.NewSignatureService(opts)
	if err != nil {
		return signing.SignatureService{}, fmt.Errorf("creating a new signature service instance: %w", err)
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
