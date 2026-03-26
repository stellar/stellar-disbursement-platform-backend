package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const EmbeddedWalletServiceInstanceName = "embedded_wallet_service_instance"

func NewEmbeddedWalletService(ctx context.Context, opts services.EmbeddedWalletServiceOptions) (services.EmbeddedWalletServiceInterface, error) {
	instanceName := EmbeddedWalletServiceInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if embeddedWalletServiceInstance, ok2 := instance.(services.EmbeddedWalletServiceInterface); ok2 {
			return embeddedWalletServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast a new embedded wallet service instance")
	}

	log.Ctx(ctx).Info("⚙️ Setting up Embedded Wallet Service")

	// Create SDP models from MTN DB connection pool
	sdpModels, err := data.NewModels(opts.MTNDBConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("creating SDP models: %w", err)
	}

	if opts.RPCClient == nil {
		return nil, fmt.Errorf("rpc client cannot be nil")
	}

	newInstance, err := services.NewEmbeddedWalletService(sdpModels, opts.WasmHash, opts.RPCClient)
	if err != nil {
		return nil, fmt.Errorf("creating embedded wallet service: %w", err)
	}
	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
