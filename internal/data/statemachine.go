package data

import "fmt"

type State string

type StateTransition struct {
	From State
	To   State
}

type StateMachine struct {
	CurrentState State
	Transitions  map[State]map[State]bool
}

func NewStateMachine(initialState State, transitions []StateTransition) *StateMachine {
	sm := &StateMachine{
		CurrentState: initialState,
		Transitions:  make(map[State]map[State]bool),
	}

	for _, t := range transitions {
		if sm.Transitions[t.From] == nil {
			sm.Transitions[t.From] = make(map[State]bool)
		}
		sm.Transitions[t.From][t.To] = true
	}

	return sm
}

func (sm *StateMachine) CanTransitionTo(targetState State) bool {
	if _, ok := sm.Transitions[sm.CurrentState]; !ok {
		return false
	}
	return sm.Transitions[sm.CurrentState][targetState]
}

func (sm *StateMachine) TransitionTo(targetState State) error {
	if sm.CanTransitionTo(targetState) {
		sm.CurrentState = targetState
		return nil
	}
	return fmt.Errorf("cannot transition from %s to %s", sm.CurrentState, targetState)
}
