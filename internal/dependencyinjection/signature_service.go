package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

type SignatureServiceOptions struct {
	// Used for routing:
	DistributionSignerType signing.SignatureClientType

	// Shared:
	NetworkPassphrase string

	// DistributionAccountEnv:
	DistributionPrivateKey string

	// ChannelAccountDB:
	DBConnectionPool     db.DBConnectionPool
	EncryptionPassphrase string
	LedgerNumberTracker  preconditions.LedgerNumberTracker
}

const SignatureServiceInstanceName = "signature_service_instance"

// buildSignatureServiceInstanceName creates a new Signature Service instance, or retrives a instance that was already
// created before.
func buildSignatureServiceInstanceName(sigType signing.SignatureClientType) string {
	return fmt.Sprintf("%s-%s", SignatureServiceInstanceName, string(sigType))
}

// NewSignatureService creates a new signature service instance, or retrives an instance that was already
// created before.
func NewSignatureService(ctx context.Context, opts SignatureServiceOptions) (signing.SignatureService, error) {
	instanceName := buildSignatureServiceInstanceName(opts.DistributionSignerType)

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if signatureServiceInstance, ok2 := instance.(signing.SignatureService); ok2 {
			return signatureServiceInstance, nil
		}
		return signing.SignatureService{}, fmt.Errorf("trying to cast an existing signature service instance")
	}

	log.Ctx(ctx).Infof("⚙️ Setting up Signature Service to: %v", opts.DistributionSignerType)
	newInstance, err := signing.NewSignatureService(opts.DistributionSignerType, signing.SignatureServiceOptions{
		NetworkPassphrase:      opts.NetworkPassphrase,
		DistributionPrivateKey: opts.DistributionPrivateKey,
		DBConnectionPool:       opts.DBConnectionPool,
		EncryptionPassphrase:   opts.EncryptionPassphrase,
		LedgerNumberTracker:    opts.LedgerNumberTracker,
		Encrypter:              &utils.DefaultPrivateKeyEncrypter{},
	})
	if err != nil {
		return signing.SignatureService{}, fmt.Errorf("creating a new signature service instance: %w", err)
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
