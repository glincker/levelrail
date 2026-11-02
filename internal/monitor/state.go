package monitor

import (
	"math"
	"sync"
)

// Confirmed monitor states (ADR 054). Suspect and recovering are the unconfirmed steps either side of down.
const (
	StateUp         = "up"
	StateSuspect    = "suspect"
	StateDown       = "down"
	StateRecovering = "recovering"
)

// Transition is a confirmed change worth telling someone about: down or back up.
type Transition struct {
	Monitor Monitor
	State   string
	Reason  string
}

// Notifier receives confirmed transitions. It must not block the probe worker.
type Notifier func(Transition)

// StateDegraded is reported for a passing monitor that is near failing, such as a certificate
// inside its warning window. Confirmed tracking itself stays up while degraded.
const StateDegraded = "degraded"

type track struct {
	degraded bool
	state    string
	fails    int
	oks      int
}

// Tracker turns raw probe results into confirmed states so one bad sample
// never pages. Flap control stays in alerting, this only confirms.
type Tracker struct {
	mu       sync.Mutex
	confirm  int
	recover  int
	byID     map[string]*track
	notifier Notifier
}

// NewTracker builds a Tracker; counts below 1 are treated as 1.
func NewTracker(confirmFailures, recoverySuccesses int, n Notifier) *Tracker {
	return &Tracker{confirm: max(confirmFailures, 1), recover: max(recoverySuccesses, 1), byID: map[string]*track{}, notifier: n}
}

// Observe feeds one result and returns the monitor's state afterwards.
func (t *Tracker) Observe(m Monitor, r Result) string {
	t.mu.Lock()
	tr, ok := t.byID[m.ID]
	if !ok {
		tr = &track{state: StateUp}
		t.byID[m.ID] = tr
	}
	var fire *Transition
	tr.degraded = r.OK && r.Degraded
	if r.OK {
		tr.fails = 0
		switch tr.state {
		case StateSuspect:
			tr.state = StateUp
		case StateDown:
			tr.state, tr.oks = StateRecovering, 1
			if tr.oks >= t.recover {
				tr.state = StateUp
				fire = &Transition{Monitor: m, State: StateUp}
			}
		case StateRecovering:
			tr.oks++
			if tr.oks >= t.recover {
				tr.state, tr.oks = StateUp, 0
				fire = &Transition{Monitor: m, State: StateUp}
			}
		}
	} else {
		tr.oks = 0
		switch tr.state {
		case StateUp:
			tr.state, tr.fails = StateSuspect, 1
			if tr.fails >= t.confirm {
				tr.state = StateDown
				fire = &Transition{Monitor: m, State: StateDown, Reason: r.Reason}
			}
		case StateSuspect:
			tr.fails++
			if tr.fails >= t.confirm {
				tr.state = StateDown
				fire = &Transition{Monitor: m, State: StateDown, Reason: r.Reason}
			}
		case StateDown:
			// Keep counting so a per-monitor link threshold above the global one can still be met.
			if tr.fails < math.MaxInt32 {
				tr.fails++
			}
		case StateRecovering:
			// A failure while recovering returns to down without a new notification.
			tr.state, tr.fails = StateDown, t.confirm
		}
	}
	state, n := tr.state, t.notifier
	t.mu.Unlock()
	if fire != nil && n != nil {
		n(*fire)
	}
	return state
}

// Force sets a monitor up or down at once, for kinds whose own timing already confirms
// the change. It notifies on a real transition and returns the state afterwards.
func (t *Tracker) Force(m Monitor, down bool, reason string, degraded bool) string {
	t.mu.Lock()
	tr, ok := t.byID[m.ID]
	if !ok {
		tr = &track{state: StateUp}
		t.byID[m.ID] = tr
	}
	var fire *Transition
	tr.degraded = !down && degraded
	switch {
	case down && tr.state == StateDown:
		// Count repeats so a link threshold above the global one can still be met.
		if tr.fails < math.MaxInt32 {
			tr.fails++
		}
	case down && tr.state != StateDown:
		if tr.state != StateRecovering {
			fire = &Transition{Monitor: m, State: StateDown, Reason: reason}
		}
		tr.state, tr.fails, tr.oks = StateDown, t.confirm, 0
	case !down && (tr.state == StateDown || tr.state == StateRecovering):
		tr.state, tr.fails, tr.oks = StateUp, 0, 0
		fire = &Transition{Monitor: m, State: StateUp}
	case !down:
		tr.state, tr.fails, tr.oks = StateUp, 0, 0
	}
	state, n := tr.state, t.notifier
	t.mu.Unlock()
	if fire != nil && n != nil {
		n(*fire)
	}
	return state
}

// Degraded reports whether the last passing result was flagged degraded.
func (t *Tracker) Degraded(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	tr, ok := t.byID[id]
	return ok && tr.degraded && tr.state == StateUp
}

// State returns the current state, or "" for a monitor not probed yet.
func (t *Tracker) State(id string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tr, ok := t.byID[id]; ok {
		return tr.state
	}
	return ""
}

// Status is a monitor's confirmed state plus the consecutive counts behind it.
type Status struct {
	State         string
	FailStreak    int
	SuccessStreak int
}

// Status returns the state and streaks, or a zero Status for a monitor not probed yet.
func (t *Tracker) Status(id string) Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tr, ok := t.byID[id]; ok {
		return Status{State: tr.state, FailStreak: tr.fails, SuccessStreak: tr.oks}
	}
	return Status{}
}

// Forget drops a deleted monitor.
func (t *Tracker) Forget(id string) {
	t.mu.Lock()
	delete(t.byID, id)
	t.mu.Unlock()
}
