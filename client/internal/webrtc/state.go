package webrtc

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// StateMachine manages ConnectionState transitions with optional change callbacks.
// It is safe for concurrent use.
type StateMachine struct {
	state    atomic.Int32
	mu       sync.Mutex
	onChange func(from, to ConnectionState)
}

// NewStateMachine creates a StateMachine starting in the given initial state.
func NewStateMachine(initial ConnectionState) *StateMachine {
	sm := &StateMachine{}
	sm.state.Store(int32(initial))
	return sm
}

// State returns the current ConnectionState.
func (sm *StateMachine) State() ConnectionState {
	return ConnectionState(sm.state.Load())
}

// SetOnChange registers a callback that is invoked after every valid state
// transition. The callback is called with the previous and new state while
// the StateMachine's internal lock is NOT held, so it is safe to call
// StateMachine methods from within the callback.
func (sm *StateMachine) SetOnChange(fn func(from, to ConnectionState)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onChange = fn
}

// TransitionTo attempts to move the state machine to the target state.
// It returns an error if the transition is not valid according to the
// transition table defined in types.go.
// The operation is atomic: the state is updated and the callback is fired
// exactly once per successful call, even under concurrent access.
func (sm *StateMachine) TransitionTo(to ConnectionState) error {
	sm.mu.Lock()

	from := ConnectionState(sm.state.Load())
	if !isValidTransition(from, to) {
		sm.mu.Unlock()
		return fmt.Errorf("webrtc: invalid state transition %s -> %s", from, to)
	}

	sm.state.Store(int32(to))
	fn := sm.onChange
	sm.mu.Unlock()

	// Invoke callback outside the lock to avoid re-entrancy deadlocks.
	if fn != nil {
		fn(from, to)
	}
	return nil
}
