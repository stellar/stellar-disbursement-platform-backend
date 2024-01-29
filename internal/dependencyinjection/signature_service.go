package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
)

const signatureServiceInstanceName = "signature_service_instance"

type SignatureServiceOptions struct {
	NetworkPassphrase      string
	DBConnectionPool       db.DBConnectionPool
	DistributionPrivateKey string
	EncryptionPassphrase   string
}

// NewSignatureService creates a new signature service instance, or retrives an instance that was already
// created before.
func NewSignatureService(ctx context.Context, opts engine.SignatureServiceOptions) (engine.SignatureService, error) {
	instanceName := signatureServiceInstanceName

	// Already initialized
	if instance, ok := dependenciesStoreMap[instanceName]; ok {
		if signatureServiceInstance, ok2 := instance.(engine.SignatureService); ok2 {
			return signatureServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing signature service instance")
	}

	// Setup a new signature service instance
	log.Ctx(ctx).Infof("⚙️ Setting up Signature Service to: %v", opts.Type)
	newSignatureService, err := engine.NewSignatureService(opts)
	if err != nil {
		return nil, fmt.Errorf("creating a new signature service instance: %w", err)
	}

	SetInstance(instanceName, newSignatureService)

	return newSignatureService, nil
}
