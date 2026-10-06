package data

import (
	"fmt"
	"strings"
)

type DisbursementStatus string

const (
	DraftDisbursementStatus     DisbursementStatus = "DRAFT"
	ReadyDisbursementStatus     DisbursementStatus = "READY"
	StartedDisbursementStatus   DisbursementStatus = "STARTED"
	PausedDisbursementStatus    DisbursementStatus = "PAUSED"
	CompletedDisbursementStatus DisbursementStatus = "COMPLETED"
	// CanceledDisbursementStatus is a terminal state for disbursements that were manually
	// canceled before being started. Only DRAFT/READY disbursements can reach it - once a
	// disbursement is STARTED, real on-chain submissions may already be in flight, so PAUSE
	// (not cancel) is the correct action.
	CanceledDisbursementStatus DisbursementStatus = "CANCELED"
)

var NotStartedDisbursementStatuses = []DisbursementStatus{DraftDisbursementStatus, ReadyDisbursementStatus}

// TransitionTo transitions the disbursement status to the target state
func (status DisbursementStatus) TransitionTo(targetState DisbursementStatus) error {
	return DisbursementStateMachineWithInitialState(status).TransitionTo(targetState.State())
}

// DisbursementStatuses returns a list of all possible disbursement statuses
func DisbursementStatuses() []DisbursementStatus {
	return []DisbursementStatus{DraftDisbursementStatus, ReadyDisbursementStatus, StartedDisbursementStatus, PausedDisbursementStatus, CompletedDisbursementStatus, CanceledDisbursementStatus}
}

// SourceStatuses returns a list of states that the payment status can transition from given the target state
func (status DisbursementStatus) SourceStatuses() []DisbursementStatus {
	stateMachine := DisbursementStateMachineWithInitialState(DraftDisbursementStatus)
	fromStates := []DisbursementStatus{}
	for _, fromState := range DisbursementStatuses() {
		if stateMachine.Transitions[fromState.State()][status.State()] {
			fromStates = append(fromStates, fromState)
		}
	}
	return fromStates
}

// DisbursementStateMachineWithInitialState returns a state machine for disbursements initialized with the given state
func DisbursementStateMachineWithInitialState(initialState DisbursementStatus) *StateMachine {
	transitions := []StateTransition{
		{From: DraftDisbursementStatus.State(), To: ReadyDisbursementStatus.State()},       // instructions uploaded successfully
		{From: ReadyDisbursementStatus.State(), To: ReadyDisbursementStatus.State()},       // user re-uploads instructions
		{From: ReadyDisbursementStatus.State(), To: StartedDisbursementStatus.State()},     // user starts disbursement
		{From: StartedDisbursementStatus.State(), To: PausedDisbursementStatus.State()},    // user pauses disbursement
		{From: PausedDisbursementStatus.State(), To: StartedDisbursementStatus.State()},    // user resumes disbursement
		{From: StartedDisbursementStatus.State(), To: CompletedDisbursementStatus.State()}, // all payments went through
		{From: DraftDisbursementStatus.State(), To: CanceledDisbursementStatus.State()},    // user cancels before starting
		{From: ReadyDisbursementStatus.State(), To: CanceledDisbursementStatus.State()},    // user cancels before starting
	}

	return NewStateMachine(initialState.State(), transitions)
}

// Validate validates the disbursement status
func (status DisbursementStatus) Validate() error {
	switch DisbursementStatus(strings.ToUpper(string(status))) {
	case DraftDisbursementStatus, ReadyDisbursementStatus, StartedDisbursementStatus, PausedDisbursementStatus, CompletedDisbursementStatus, CanceledDisbursementStatus:
		return nil
	default:
		return fmt.Errorf("invalid disbursement status: %s", status)
	}
}

// ToDisbursementStatus converts a string to a DisbursementStatus
func ToDisbursementStatus(s string) (DisbursementStatus, error) {
	err := DisbursementStatus(s).Validate()
	if err != nil {
		return "", err
	}
	return DisbursementStatus(strings.ToUpper(s)), nil
}

func (status DisbursementStatus) State() State {
	return State(status)
}
