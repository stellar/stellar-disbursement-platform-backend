package engine

import (
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// SubmitterEngine aggregates the dependencies that are shared between all Submitter instances, such as the Ledger
// number tracker.
type SubmitterEngine struct {
	HorizonClient horizonclient.ClientInterface
	preconditions.LedgerNumberTracker
	signing.SignatureService
	MaxBaseFee int
}

func (se *SubmitterEngine) Validate() error {
	if se.HorizonClient == nil {
		return fmt.Errorf("horizon client cannot be nil")
	}

	if se.LedgerNumberTracker == nil {
		return fmt.Errorf("ledger number tracker cannot be nil")
	}

	if utils.IsEmpty(se.SignatureService) {
		return fmt.Errorf("signature service cannot be empty")
	}
	if err := se.SignatureService.Validate(); err != nil {
		return fmt.Errorf("validating signature service: %w", err)
	}

	if se.MaxBaseFee < txnbuild.MinBaseFee {
		return fmt.Errorf("maxBaseFee must be greater than or equal to %d", txnbuild.MinBaseFee)
	}

	return nil
}
