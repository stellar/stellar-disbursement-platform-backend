package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
)

const txSubmitterEngineInstanceName = "tx_submitter_engine_instance"

type TxSubmitterEngineOptions struct {
	HorizonURL              string
	SignatureServiceOptions engine.SignatureServiceOptions
	MaxBaseFee              int
}

// NewTxSubmitterEngine creates a new ledger number tracker instance, or retrives an instance that was already
// created before.
func NewTxSubmitterEngine(ctx context.Context, opts TxSubmitterEngineOptions) (engine.SubmitterEngine, error) {
	instanceName := txSubmitterEngineInstanceName

	// Already initialized
	if instance, ok := GetInstance(instanceName); ok {
		if castedInstance, ok2 := instance.(engine.SubmitterEngine); ok2 {
			return castedInstance, nil
		}
		return engine.SubmitterEngine{}, fmt.Errorf("trying to cast an existing ledger number tracker instance")
	}

	horizonClient, err := NewHorizonClient(ctx, opts.HorizonURL)
	if err != nil {
		return engine.SubmitterEngine{}, fmt.Errorf("grabbing horizon client instance: %w", err)
	}

	ledgerNumberTracker, err := NewLedgerNumberTracker(ctx, horizonClient)
	if err != nil {
		return engine.SubmitterEngine{}, fmt.Errorf("grabbing ledger number tracker instance: %w", err)
	}

	signatureService, err := NewSignatureService(ctx, opts.SignatureServiceOptions)
	if err != nil {
		return engine.SubmitterEngine{}, fmt.Errorf("grabbing signature service instance: %w", err)
	}

	// Setup a new instance
	log.Ctx(ctx).Infof("⚙️ Setting up Tx Submitter Engine")
	newInstance := engine.SubmitterEngine{
		HorizonClient:       horizonClient,
		LedgerNumberTracker: ledgerNumberTracker,
		SignatureService:    signatureService,
		MaxBaseFee:          opts.MaxBaseFee,
	}
	if err = newInstance.Validate(); err != nil {
		return engine.SubmitterEngine{}, fmt.Errorf("validating new instance submitter engine: %w", err)
	}

	SetInstance(instanceName, newInstance)

	return newInstance, nil
}
