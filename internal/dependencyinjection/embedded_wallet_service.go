package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

const EmbeddedWalletServiceInstanceName = "embedded_wallet_service_instance"

func NewEmbeddedWalletService(ctx context.Context, opts services.EmbeddedWalletServiceOptions) (services.EmbeddedWalletServiceInterface, error) {
	if opts.MTNDBConnectionPool == nil {
		return nil, fmt.Errorf("mtn db connection pool cannot be nil")
	}
	if opts.TSSDBConnectionPool == nil {
		return nil, fmt.Errorf("tss db connection pool cannot be nil")
	}

	instanceName := EmbeddedWalletServiceInstanceName

	if instance, ok := GetInstance(instanceName); ok {
		if existingService, ok := instance.(services.EmbeddedWalletServiceInterface); ok {
			return existingService, nil
		}
		return nil, fmt.Errorf("trying to cast pre-existing embedded wallet service for dependency injection")
	}

	log.Infof("⚙️ Setting up Embedded Wallet Service")
	sdpModels, err := data.NewModels(opts.MTNDBConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("creating models: %w", err)
	}

	embeddedWalletService := services.NewEmbeddedWalletService(sdpModels, store.NewTransactionModel(opts.TSSDBConnectionPool))
	SetInstance(instanceName, embeddedWalletService)

	return embeddedWalletService, nil
}
