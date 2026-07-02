package webrtc

import (
	"sync"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newSM(initial ConnectionState) *StateMachine {
	return NewStateMachine(initial)
}

// ---------------------------------------------------------------------------
// Valid transitions
// ---------------------------------------------------------------------------

func TestStateMachine_ValidTransitions(t *testing.T) {
	cases := []struct {
		from ConnectionState
		to   ConnectionState
	}{
		{StateIdle, StateGatheringICE},
		{StateIdle, StateTCPRelay},
		{StateIdle, StateClosing},
		{StateGatheringICE, StateConnecting},
		{StateGatheringICE, StateWebRTCFailed},
		{StateGatheringICE, StateClosing},
		{StateConnecting, StateWebRTCConnected},
		{StateConnecting, StateWebRTCFailed},
		{StateConnecting, StateClosing},
		{StateWebRTCConnected, StateWebRTCReconnecting},
		{StateWebRTCConnected, StateTCPRelay},
		{StateWebRTCConnected, StateClosing},
		{StateWebRTCFailed, StateTCPRelay},
		{StateWebRTCFailed, StateWebRTCReconnecting},
		{StateWebRTCFailed, StateClosing},
		{StateWebRTCReconnecting, StateGatheringICE},
		{StateWebRTCReconnecting, StateTCPRelay},
		{StateWebRTCReconnecting, StateClosing},
		{StateTCPRelay, StateGatheringICE},
		{StateTCPRelay, StateClosing},
		{StateClosing, StateClosed},
	}

	for _, tc := range cases {
		sm := newSM(tc.from)
		if err := sm.TransitionTo(tc.to); err != nil {
			t.Errorf("expected valid transition %s -> %s, got error: %v", tc.from, tc.to, err)
		}
		if sm.State() != tc.to {
			t.Errorf("after %s -> %s: state is %s", tc.from, tc.to, sm.State())
		}
	}
}

// ---------------------------------------------------------------------------
// Invalid transitions
// ---------------------------------------------------------------------------

func TestStateMachine_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from ConnectionState
		to   ConnectionState
	}{
		{StateIdle, StateConnecting},
		{StateIdle, StateWebRTCConnected},
		{StateIdle, StateClosed},
		{StateWebRTCConnected, StateIdle},
		{StateWebRTCConnected, StateGatheringICE},
		{StateClosed, StateIdle},
		{StateClosed, StateGatheringICE},
		{StateClosed, StateClosing},
		{StateClosing, StateIdle},
		{StateClosing, StateGatheringICE},
	}

	for _, tc := range cases {
		sm := newSM(tc.from)
		err := sm.TransitionTo(tc.to)
		if err == nil {
			t.Errorf("expected error for invalid transition %s -> %s, got nil", tc.from, tc.to)
		}
		// State must remain unchanged after a rejected transition.
		if sm.State() != tc.from {
			t.Errorf("state changed after invalid transition %s -> %s: now %s", tc.from, tc.to, sm.State())
		}
	}
}

// ---------------------------------------------------------------------------
// Callback fires on valid transition
// ---------------------------------------------------------------------------

func TestStateMachine_CallbackFiredOnValidTransition(t *testing.T) {
	sm := newSM(StateIdle)

	var gotFrom, gotTo ConnectionState
	var called int
	sm.SetOnChange(func(from, to ConnectionState) {
		called++
		gotFrom = from
		gotTo = to
	})

	if err := sm.TransitionTo(StateGatheringICE); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if called != 1 {
		t.Fatalf("callback called %d times, want 1", called)
	}
	if gotFrom != StateIdle || gotTo != StateGatheringICE {
		t.Errorf("callback args: got (%s, %s), want (%s, %s)", gotFrom, gotTo, StateIdle, StateGatheringICE)
	}
}

// ---------------------------------------------------------------------------
// Callback does NOT fire on invalid transition
// ---------------------------------------------------------------------------

func TestStateMachine_CallbackNotFiredOnInvalidTransition(t *testing.T) {
	sm := newSM(StateIdle)

	var called int
	sm.SetOnChange(func(from, to ConnectionState) {
		called++
	})

	_ = sm.TransitionTo(StateWebRTCConnected) // invalid

	if called != 0 {
		t.Errorf("callback called %d times on invalid transition, want 0", called)
	}
}

// ---------------------------------------------------------------------------
// Concurrent transitions are safe
// ---------------------------------------------------------------------------

func TestStateMachine_ConcurrentTransitions(t *testing.T) {
	// Run many goroutines all trying to advance the state machine from Idle.
	// Only one should succeed per valid target; the rest should get errors.
	// The important thing is no data race and no panic.
	const goroutines = 200

	sm := newSM(StateIdle)

	var callbackCount atomic.Int64
	sm.SetOnChange(func(from, to ConnectionState) {
		callbackCount.Add(1)
	})

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			// Mix of valid and invalid targets to stress the lock path.
			_ = sm.TransitionTo(StateGatheringICE)
			_ = sm.TransitionTo(StateWebRTCConnected) // invalid from Idle
		}()
	}
	wg.Wait()

	// Exactly one goroutine should have succeeded in moving to GatheringICE.
	// Callback count must equal the number of successful transitions.
	finalState := sm.State()
	if finalState != StateGatheringICE {
		t.Errorf("expected final state GatheringICE, got %s", finalState)
	}
	if n := callbackCount.Load(); n != 1 {
		t.Errorf("callback fired %d times, want 1", n)
	}
}

// ---------------------------------------------------------------------------
// State() reflects the latest committed state
// ---------------------------------------------------------------------------

func TestStateMachine_StateReflectsLatest(t *testing.T) {
	sm := newSM(StateIdle)
	if sm.State() != StateIdle {
		t.Fatalf("initial state: got %s, want Idle", sm.State())
	}

	steps := []ConnectionState{
		StateGatheringICE,
		StateConnecting,
		StateWebRTCConnected,
		StateClosing,
		StateClosed,
	}
	for _, s := range steps {
		if err := sm.TransitionTo(s); err != nil {
			t.Fatalf("transition to %s failed: %v", s, err)
		}
		if sm.State() != s {
			t.Errorf("after transition: got %s, want %s", sm.State(), s)
		}
	}
}
