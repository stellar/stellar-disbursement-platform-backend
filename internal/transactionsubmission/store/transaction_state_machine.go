package store

import (
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"golang.org/x/exp/slices"
)

type TransactionStatus string

const (
	// TransactionStatusPending indicates that a transaction has been created and added to the queue.
	TransactionStatusPending TransactionStatus = "PENDING" // TODO: rename to TransactionStatusQueued
	// TransactionStatusProcessing indicates that a transaction has been read from the queue and is being processed.
	TransactionStatusProcessing TransactionStatus = "PROCESSING"
	// TransactionStatusSuccess indicates that the transaction was successfully sent and included in the ledger.
	TransactionStatusSuccess TransactionStatus = "SUCCESS"
	// TransactionStatusError indicates that there was an error when trying to send this transaction.
	TransactionStatusError TransactionStatus = "ERROR"
)

func (status TransactionStatus) All() []TransactionStatus {
	return []TransactionStatus{TransactionStatusPending, TransactionStatusProcessing, TransactionStatusSuccess, TransactionStatusError}
}

// Validate validates the disbursement status
func (status TransactionStatus) Validate() error {
	if slices.Contains(TransactionStatus("").All(), status) {
		return nil
	}
	return fmt.Errorf("invalid disbursement status: %s", status)
}

// State will parse the TransactionState into a data.State.
func (status TransactionStatus) State() data.State {
	return data.State(status)
}

// CanTransitionTo verifies if the transition is allowed.
func (status TransactionStatus) CanTransitionTo(targetState TransactionStatus) error {
	return tssTransactionStateMachineWithInitialState(status).TransitionTo(targetState.State())
}

// tssTransactionStateMachineWithInitialState returns a state machine for TSS transactions, initialized with the given state.
func tssTransactionStateMachineWithInitialState(initialState TransactionStatus) *data.StateMachine {
	transitions := []data.StateTransition{
		{From: TransactionStatusPending.State(), To: TransactionStatusProcessing.State()}, // TSS loads the transaction from the DB for the first time.
		{From: TransactionStatusProcessing.State(), To: TransactionStatusSuccess.State()}, // TSS receives a success response from Stellar Horizon.
		{From: TransactionStatusProcessing.State(), To: TransactionStatusError.State()},   // TSS receives an error response from Stellar Horizon.
	}

	return data.NewStateMachine(initialState.State(), transitions)
}
