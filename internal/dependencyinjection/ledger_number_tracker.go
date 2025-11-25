package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
)

const LedgerNumberTrackerInstanceName = "ledger_number_tracker_instance"

// NewLedgerNumberTracker creates a new ledger number tracker instance, or retrives an instance that was already
// created before.
func NewLedgerNumberTracker(ctx context.Context, horizonClient horizonclient.ClientInterface) (preconditions.LedgerNumberTracker, error) {
	instanceName := LedgerNumberTrackerInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if castedInstance, ok2 := instance.(preconditions.LedgerNumberTracker); ok2 {
			return castedInstance, nil
		}
		return nil, fmt.Errorf("trying to cast an existing ledger number tracker instance")
	}

	// Setup a new instance
	log.Ctx(ctx).Infof("⚙️ Setting up Ledger Number Tracker")
	newInstance, err := preconditions.NewLedgerNumberTracker(horizonClient)
	if err != nil {
		return nil, fmt.Errorf("creating a new ledger number tracker instance: %w", err)
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
