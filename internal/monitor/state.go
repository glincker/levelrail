package monitor

import "sync"

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

type track struct {
	state string
	fails int
	oks   int
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
		case StateRecovering:
			// A failure while recovering returns to down without a new notification.
			tr.state = StateDown
		}
	}
	state, n := tr.state, t.notifier
	t.mu.Unlock()
	if fire != nil && n != nil {
		n(*fire)
	}
	return state
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

// Forget drops a deleted monitor.
func (t *Tracker) Forget(id string) {
	t.mu.Lock()
	delete(t.byID, id)
	t.mu.Unlock()
}
