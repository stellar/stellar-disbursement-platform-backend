package data

import (
	"fmt"
	"strings"
)

type PaymentStatus string

const (
	DraftPaymentStatus    PaymentStatus = "DRAFT"
	ReadyPaymentStatus    PaymentStatus = "READY"
	PendingPaymentStatus  PaymentStatus = "PENDING"
	PausedPaymentStatus   PaymentStatus = "PAUSED"
	SuccessPaymentStatus  PaymentStatus = "SUCCESS"
	FailedPaymentStatus   PaymentStatus = "FAILED"
	CanceledPaymentStatus PaymentStatus = "CANCELED"
)

// Validate validates the payment status
func (status PaymentStatus) Validate() error {
	switch PaymentStatus(strings.ToUpper(string(status))) {
	case DraftPaymentStatus, ReadyPaymentStatus, PendingPaymentStatus, PausedPaymentStatus,
		SuccessPaymentStatus, FailedPaymentStatus, CanceledPaymentStatus:
		return nil
	default:
		return fmt.Errorf("invalid payment status: %s", status)
	}
}

// TransitionTo transitions the payment status to the target state
func (status PaymentStatus) TransitionTo(targetState PaymentStatus) error {
	return PaymentStateMachineWithInitialState(status).TransitionTo(targetState.State())
}

// PaymentStateMachineWithInitialState returns a state machine for Payments initialized with the given state
func PaymentStateMachineWithInitialState(initialState PaymentStatus) *StateMachine {
	transitions := []StateTransition{
		{From: DraftPaymentStatus.State(), To: ReadyPaymentStatus.State()},     // disbursement started
		{From: ReadyPaymentStatus.State(), To: PendingPaymentStatus.State()},   // payment gets submitted if user is ready
		{From: ReadyPaymentStatus.State(), To: PausedPaymentStatus.State()},    // payment paused (when disbursement paused)
		{From: ReadyPaymentStatus.State(), To: CanceledPaymentStatus.State()},  // automatic cancellation of ready payments
		{From: PausedPaymentStatus.State(), To: ReadyPaymentStatus.State()},    // payment resumed (when disbursement resumed)
		{From: PendingPaymentStatus.State(), To: FailedPaymentStatus.State()},  // payment fails
		{From: FailedPaymentStatus.State(), To: PendingPaymentStatus.State()},  // payment retried
		{From: PendingPaymentStatus.State(), To: SuccessPaymentStatus.State()}, // payment succeeds
	}

	return NewStateMachine(initialState.State(), transitions)
}

// PaymentStatuses returns a list of all possible payment statuses
func PaymentStatuses() []PaymentStatus {
	return []PaymentStatus{DraftPaymentStatus, ReadyPaymentStatus, PendingPaymentStatus, PausedPaymentStatus, SuccessPaymentStatus, FailedPaymentStatus, CanceledPaymentStatus}
}

// SourceStatuses returns a list of states that the payment status can transition from given the target state
func (status PaymentStatus) SourceStatuses() []PaymentStatus {
	stateMachine := PaymentStateMachineWithInitialState(DraftPaymentStatus)
	fromStates := []PaymentStatus{}
	for _, fromState := range PaymentStatuses() {
		if stateMachine.Transitions[fromState.State()][status.State()] {
			fromStates = append(fromStates, fromState)
		}
	}
	return fromStates
}

// ToPaymentStatus converts a string to a PaymentStatus
func ToPaymentStatus(s string) (PaymentStatus, error) {
	err := PaymentStatus(s).Validate()
	if err != nil {
		return "", err
	}

	return PaymentStatus(strings.ToUpper(s)), nil
}

func (status PaymentStatus) State() State {
	return State(status)
}
