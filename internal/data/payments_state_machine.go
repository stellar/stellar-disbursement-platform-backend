package data

import (
	"fmt"
	"slices"
	"strings"
)

type PaymentStatus string

const (
	// DraftPaymentStatus is a non-terminal state for payments that were registered in the database but their disbursement has not started yet. Payments in this state can be deleted or transitioned to READY.
	DraftPaymentStatus PaymentStatus = "DRAFT"
	// ReadyPaymentStatus is a non-terminal state for payments that are waiting for the receiver to register. As soon as the receiver registers, the state is transitioned to PENDING.
	ReadyPaymentStatus PaymentStatus = "READY"
	// PendingPaymentStatus is a non-terminal state for payments that were marked for submission to the Stellar network. They can or can not have been submitted yet.
	PendingPaymentStatus PaymentStatus = "PENDING"
	// PausedPaymentStatus is a non-terminal state for payments that were manually paused. Payments in this state can be resumed.
	PausedPaymentStatus PaymentStatus = "PAUSED"
	// SuccessPaymentStatus is a terminal state for payments that were successfully submitted to the Stellar network.
	SuccessPaymentStatus PaymentStatus = "SUCCESS"
	// FailedPaymentStatus is a terminal state for payments that failed when submitted to the Stellar network. Payments in this state can be retried.
	FailedPaymentStatus PaymentStatus = "FAILED"
	// CanceledPaymentStatus is a terminal state for payments that were either manually or automatically canceled.
	CanceledPaymentStatus PaymentStatus = "CANCELED"
)

// Validate validates the payment status
func (status PaymentStatus) Validate() error {
	uppercaseStatus := PaymentStatus(strings.TrimSpace(strings.ToUpper(string(status))))
	if slices.Contains(PaymentStatuses(), uppercaseStatus) {
		return nil
	}

	return fmt.Errorf("invalid payment status: %s", status)
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

// PaymentInProgressStatuses returns a list of payment statuses that are in progress and could block potential new payments
// from being initiated if the distribution balance is low.
func PaymentInProgressStatuses() []PaymentStatus {
	return []PaymentStatus{ReadyPaymentStatus, PendingPaymentStatus, PausedPaymentStatus}
}

func PaymentActiveStatuses() []PaymentStatus {
	return []PaymentStatus{ReadyPaymentStatus, PendingPaymentStatus}
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
