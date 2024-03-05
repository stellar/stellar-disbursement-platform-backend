package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
)

const DistributionAccountResolverInstanceName = "distribution_account_resolver_instance"

// NewDistributionAccountResolver creates a new distribution account resolver instance, or retrieves an instance that
// was already created before.
func NewDistributionAccountResolver(ctx context.Context, opts signing.DistributionAccountResolverOptions) (signing.DistributionAccountResolver, error) {
	instanceName := DistributionAccountResolverInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if distAccResolverInstance, ok2 := instance.(signing.DistributionAccountResolver); ok2 {
			return distAccResolverInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing distribution account resolver instance")
	}

	log.Ctx(ctx).Info("⚙️ Setting up Distribution Account Resolver")
	newInstance, err := signing.NewDistributionAccountResolver(opts)
	if err != nil {
		return nil, fmt.Errorf("creating a new distribution account resolver instance: %w", err)
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
