package engine

import (
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/txnbuild"
)

// SubmitterEngine aggregates the dependencies that are shared between all Submitter instances, such as the Ledger
// number tracker.
type SubmitterEngine struct {
	HorizonClient horizonclient.ClientInterface
	LedgerNumberTracker
	SignatureService
	MaxBaseFee int
}

func (se *SubmitterEngine) Validate() error {
	if se.HorizonClient == nil {
		return fmt.Errorf("horizon client cannot be nil")
	}

	if se.LedgerNumberTracker == nil {
		return fmt.Errorf("ledger number tracker cannot be nil")
	}

	if se.SignatureService == nil {
		return fmt.Errorf("signature service cannot be nil")
	}

	if se.MaxBaseFee < txnbuild.MinBaseFee {
		return fmt.Errorf("maxBaseFee must be greater than or equal to %d", txnbuild.MinBaseFee)
	}

	return nil
}
