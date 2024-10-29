package data

import (
	"fmt"
	"strings"
)

type ReceiversWalletStatus string

const (
	DraftReceiversWalletStatus      ReceiversWalletStatus = "DRAFT"
	ReadyReceiversWalletStatus      ReceiversWalletStatus = "READY"
	RegisteredReceiversWalletStatus ReceiversWalletStatus = "REGISTERED"
	FlaggedReceiversWalletStatus    ReceiversWalletStatus = "FLAGGED"
)

// TransitionTo transitions the receiver wallet status to the target state
func (status ReceiversWalletStatus) TransitionTo(targetState ReceiversWalletStatus) error {
	return ReceiversWalletStateMachineWithInitialState(status).TransitionTo(targetState.State())
}

// ReceiversWalletStateMachineWithInitialState returns a state machine for ReceiversWallets initialized with the given state
func ReceiversWalletStateMachineWithInitialState(initialState ReceiversWalletStatus) *StateMachine {
	transitions := []StateTransition{
		{From: DraftReceiversWalletStatus.State(), To: ReadyReceiversWalletStatus.State()},        // disbursement started
		{From: ReadyReceiversWalletStatus.State(), To: RegisteredReceiversWalletStatus.State()},   // receiver signed up
		{From: ReadyReceiversWalletStatus.State(), To: FlaggedReceiversWalletStatus.State()},      // flagged
		{From: FlaggedReceiversWalletStatus.State(), To: ReadyReceiversWalletStatus.State()},      // unflagged
		{From: RegisteredReceiversWalletStatus.State(), To: FlaggedReceiversWalletStatus.State()}, // flagged
		{From: FlaggedReceiversWalletStatus.State(), To: RegisteredReceiversWalletStatus.State()}, // unflagged
	}

	return NewStateMachine(initialState.State(), transitions)
}

func (status ReceiversWalletStatus) State() State {
	return State(status)
}

// Validate validates the receiver wallet status
func (status ReceiversWalletStatus) Validate() error {
	switch ReceiversWalletStatus(strings.ToUpper(string(status))) {
	case DraftReceiversWalletStatus, ReadyReceiversWalletStatus, RegisteredReceiversWalletStatus, FlaggedReceiversWalletStatus:
		return nil
	default:
		return fmt.Errorf("invalid receiver wallet status %q", status)
	}
}
