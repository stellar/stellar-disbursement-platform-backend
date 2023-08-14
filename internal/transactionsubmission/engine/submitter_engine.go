package engine

import (
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
)

// SubmitterEngine aggregates the dependencies that are shared between all Submitter instances, such as the Ledger
// number tracker.
type SubmitterEngine struct {
	HorizonClient horizonclient.ClientInterface
	LedgerNumberTracker
}

func NewSubmitterEngine(hClient horizonclient.ClientInterface) (*SubmitterEngine, error) {
	ledgerNumberTracker, err := NewLedgerNumberTracker(hClient)
	if err != nil {
		return nil, fmt.Errorf("creating ledger keeper: %w", err)
	}

	return &SubmitterEngine{
		HorizonClient:       hClient,
		LedgerNumberTracker: ledgerNumberTracker,
	}, nil
}
