package dependencyinjection

import (
	"context"
	"fmt"
	"time"

	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/wallet"
)

const WebAuthnServiceInstanceName = "webauthn_service_instance"

type WebAuthnServiceOptions struct {
	MTNDBConnectionPool    db.DBConnectionPool
	SessionTTL             time.Duration
	SessionCacheMaxEntries int
}

// NewWebAuthnService creates a new WebAuthn service instance, or retrieves an instance that was previously created.
func NewWebAuthnService(ctx context.Context, opts WebAuthnServiceOptions) (wallet.WebAuthnServiceInterface, error) {
	instanceName := WebAuthnServiceInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if webauthnServiceInstance, ok2 := instance.(wallet.WebAuthnServiceInterface); ok2 {
			return webauthnServiceInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing webauthn service instance")
	}

	log.Ctx(ctx).Info("⚙️ Setting up WebAuthn Service")

	if opts.MTNDBConnectionPool == nil {
		return nil, fmt.Errorf("MTNDBConnectionPool is required")
	}
	if opts.SessionTTL <= 0 {
		return nil, fmt.Errorf("SessionTTL must be greater than zero")
	}
	if opts.SessionCacheMaxEntries <= 0 {
		return nil, fmt.Errorf("SessionCacheMaxEntries must be greater than zero")
	}

	// Create SDP models from MTN DB connection pool
	sdpModels, err := data.NewModels(opts.MTNDBConnectionPool)
	if err != nil {
		return nil, fmt.Errorf("creating SDP models: %w", err)
	}

	sessionCache, err := wallet.NewInMemorySessionCache(opts.SessionTTL, opts.SessionCacheMaxEntries)
	if err != nil {
		return nil, fmt.Errorf("creating WebAuthn session cache: %w", err)
	}

	newInstance, err := wallet.NewWebAuthnService(sdpModels, sessionCache)
	if err != nil {
		return nil, fmt.Errorf("creating a new webauthn service instance: %w", err)
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
