package spendcap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Event kinds recorded in an app's history.
const (
	EventThreshold  = "threshold"
	EventEnforced   = "enforced"
	EventReleased   = "released"
	EventRollover   = "rollover"
	EventConfigured = "configured"
	EventCleared    = "cleared"
	EventOverride   = "override"
	EventCapPaused  = "cap_paused"
	EventCapResumed = "cap_resumed"
)

// StopState values: whether the cap stopped the app's containers.
const (
	StopPaused  = "paused"
	StopResumed = "resumed"
)

// ErrNotPaused means the app is not held stopped by its spend cap.
var ErrNotPaused = errors.New("app is not paused by a spend cap")

// Sleeper stops and starts an app through the sleep service.
type Sleeper interface {
	Pause(ctx context.Context, app string) error
	Resume(ctx context.Context, app string) error
}

// State is what the controller last decided for an app in a period.
type State struct {
	App    string
	Period string
	// Enforced is "" or the action ingress must apply; Nudged is the value
	// ingress was last told about.
	Enforced string
	Nudged   string
	// StopState is "", StopPaused (containers stopped by the cap) or StopResumed
	// (started by hand, not stopped again until the cap lifts or the period ends).
	StopState   string
	Highest     int
	Pct         float64
	EgressBytes int64
	Cost        float64
	UpdatedAt   time.Time
}

// Event is one entry of an app's history.
type Event struct {
	ID     int64     `json:"id"`
	App    string    `json:"app"`
	Period string    `json:"period"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail"`
	Pct    float64   `json:"pct"`
	At     time.Time `json:"at"`
}

// Enforcement is one app's action ingress must apply now.
type Enforcement struct {
	App         string
	Action      string
	ThrottleRPS int
}

// Store is the persistence the controller needs; *store.DB satisfies it.
type Store interface {
	ListSpendCaps(ctx context.Context) ([]Cap, error)
	GetSpendCap(ctx context.Context, app string) (Cap, error)
	SaveSpendCap(ctx context.Context, c Cap, ev Event, now time.Time) error
	DeleteSpendCap(ctx context.Context, app string, ev Event) error
	GetSpendCapState(ctx context.Context, app string) (State, bool, error)
	SaveSpendCapState(ctx context.Context, st State, events []Event) error
	ListSpendCapEvents(ctx context.Context, app string, limit int) ([]Event, error)
}

// UsageSource reports month to date usage by app name.
type UsageSource interface {
	SpendCapUsage(ctx context.Context, now time.Time) (map[string]Usage, error)
}

// Controller converges each capped app's enforcement on its usage. It holds
// no state of its own: every tick re-derives the decision from the store
// and usage, so a restart or a missed tick changes nothing.
type Controller struct {
	Store  Store
	Usage  UsageSource
	Nudge  func(ctx context.Context) error
	Logger *slog.Logger
	Now    func() time.Time
	// Sleeper and StopApps make a pause really stop containers; with either
	// unset or StopApps false the pause stays a maintenance response.
	Sleeper  Sleeper
	StopApps func(ctx context.Context) bool
	// Notify tells the owner a cap stopped or restarted an app.
	Notify func(ctx context.Context, app, text string)
}

func (c *Controller) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Tick reconciles every capped app. A usage failure leaves enforcement as it
// is, so an outage in telemetry never lifts or applies a limit by accident.
func (c *Controller) Tick(ctx context.Context) error {
	now := c.now()
	caps, err := c.Store.ListSpendCaps(ctx)
	if err != nil {
		return fmt.Errorf("spendcap: list caps: %w", err)
	}
	if len(caps) == 0 {
		return nil
	}
	usage, err := c.Usage.SpendCapUsage(ctx, now)
	if err != nil {
		return fmt.Errorf("spendcap: usage: %w", err)
	}
	var errs []error
	for _, cp := range caps {
		if err := c.reconcile(ctx, cp, usage[cp.App], now); err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", cp.App, err))
		}
	}
	return errors.Join(errs...)
}

func enforcedValue(action string) string {
	if action == ActionNone {
		return ""
	}
	return action
}

func (c *Controller) reconcile(ctx context.Context, cp Cap, u Usage, now time.Time) error {
	period := Period(now)
	st, ok, err := c.Store.GetSpendCapState(ctx, cp.App)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	var events []Event
	add := func(kind, detail string, pct float64) {
		events = append(events, Event{App: cp.App, Period: period, Kind: kind, Detail: detail, Pct: pct, At: now})
	}
	if !ok || st.Period != period {
		if ok {
			add(EventRollover, fmt.Sprintf("period %s ended at %.0f%% with %q in force; counters reset", st.Period, st.Pct, orNone(st.Enforced)), st.Pct)
		}
		// Ingress may still hold last month's action: keep what it was told
		// so the change to "" gets nudged.
		st = State{App: cp.App, Period: period, Nudged: st.Enforced, StopState: st.StopState}
		if !ok {
			st.Nudged = ""
		}
		// A new month ends a manual resume; a held stop is released below.
		if st.StopState == StopResumed {
			st.StopState = ""
		}
	}
	ev := Evaluate(cp, u)
	if h := ev.Highest(); h > st.Highest {
		add(EventThreshold, fmt.Sprintf("reached %d%% of the limit", h), ev.Pct)
		st.Highest = h
	}
	desired := enforcedValue(ev.Desired)
	switch {
	case desired != st.Enforced && desired != "":
		add(EventEnforced, fmt.Sprintf("%s applied at %.0f%%", desired, ev.Pct), ev.Pct)
	case desired != st.Enforced:
		add(EventReleased, fmt.Sprintf("%s lifted at %.0f%%", st.Enforced, ev.Pct), ev.Pct)
	}
	st.Enforced = desired
	st.Pct = ev.Pct
	stopErr := c.syncStop(ctx, &st, desired, add)
	st.Pct, st.EgressBytes, st.Cost, st.UpdatedAt = ev.Pct, u.EgressBytes, u.Cost, now
	if err := c.Store.SaveSpendCapState(ctx, st, events); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if stopErr != nil {
		return stopErr
	}
	if st.Enforced == st.Nudged {
		return nil
	}
	if c.Nudge != nil {
		if err := c.Nudge(ctx); err != nil {
			return fmt.Errorf("nudge ingress: %w", err)
		}
	}
	st.Nudged = st.Enforced
	if err := c.Store.SaveSpendCapState(ctx, st, nil); err != nil {
		return fmt.Errorf("record nudge: %w", err)
	}
	if c.Logger != nil {
		c.Logger.InfoContext(ctx, "spendcap: enforcement changed", slog.String("app", cp.App), slog.String("action", orNone(st.Enforced)), slog.Float64("pct", ev.Pct))
	}
	return nil
}

// Status is one app's cap with live usage and condition.
type Status struct {
	Cap
	Period      string    `json:"period"`
	EgressBytes int64     `json:"egress_bytes"`
	Cost        float64   `json:"cost"`
	EgressPct   float64   `json:"egress_pct"`
	CostPct     float64   `json:"cost_pct"`
	Pct         float64   `json:"pct"`
	Enforced    string    `json:"enforced"`
	ResetsAt    time.Time `json:"resets_at"`
	Condition   Condition `json:"condition"`
}

// Status returns the cap, its live position and the controller's condition.
func (c *Controller) Status(ctx context.Context, app string) (Status, error) {
	cp, err := c.Store.GetSpendCap(ctx, app)
	if err != nil {
		return Status{}, err
	}
	now := c.now()
	usage, err := c.Usage.SpendCapUsage(ctx, now)
	if err != nil {
		return Status{}, fmt.Errorf("spendcap: usage: %w", err)
	}
	st, ok, err := c.Store.GetSpendCapState(ctx, app)
	if err != nil {
		return Status{}, fmt.Errorf("spendcap: load state: %w", err)
	}
	if !ok || st.Period != Period(now) {
		st = State{App: app, Period: Period(now), Nudged: st.Enforced}
		if !ok {
			st.Nudged = ""
		}
	}
	u := usage[app]
	ev := Evaluate(cp, u)
	return Status{
		Cap: cp, Period: Period(now), EgressBytes: u.EgressBytes, Cost: u.Cost,
		EgressPct: ev.EgressPct, CostPct: ev.CostPct, Pct: ev.Pct,
		Enforced: orNone(st.Enforced), ResetsAt: NextPeriodStart(now),
		Condition: Conditions(cp, ev, st.Enforced, st.Nudged),
	}, nil
}

// Breach is one crossed threshold, keyed so a new level notifies again.
type Breach struct {
	Key  string
	Text string
}

// Breaches lists the highest threshold each capped app crossed this month,
// for the budget_threshold alert rule.
func (c *Controller) Breaches(ctx context.Context) ([]Breach, error) {
	caps, err := c.Store.ListSpendCaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("spendcap: list caps: %w", err)
	}
	period := Period(c.now())
	var out []Breach
	for _, cp := range caps {
		st, ok, err := c.Store.GetSpendCapState(ctx, cp.App)
		if err != nil {
			return nil, fmt.Errorf("spendcap: load state for %q: %w", cp.App, err)
		}
		if !ok || st.Period != period || st.Highest == 0 {
			continue
		}
		text := fmt.Sprintf("app %q has used %.0f%% of its monthly limit", cp.App, st.Pct)
		switch {
		case st.Enforced == ActionPause:
			text += "; it is paused until next month or until the limit is raised"
		case st.Enforced == ActionThrottle:
			text += fmt.Sprintf("; each client is limited to %d requests per second", cp.ThrottleRPS)
		case st.Highest >= 100 && cp.GuardMode == GuardBlockDeploys:
			text += "; new deploys are blocked until next month or until the limit is raised"
		case st.Highest >= 100:
			text += "; the action is alert only, so it keeps serving"
		}
		out = append(out, Breach{Key: fmt.Sprintf("spendcap:%s:%s:%d", cp.App, period, st.Highest), Text: text})
		if st.StopState == StopPaused {
			out = append(out, Breach{Key: fmt.Sprintf("spendcap:%s:%s:stopped", cp.App, period), Text: fmt.Sprintf("app %q was stopped by its spend cap and stays down until the limit resets or is raised", cp.App)})
		}
	}
	return out, nil
}

// syncStop stops the app when the cap pauses it and starts it when the cap
// lifts. A failed stop or start leaves the state unchanged so the next tick retries.
func (c *Controller) syncStop(ctx context.Context, st *State, desired string, add func(kind, detail string, pct float64)) error {
	if c.Sleeper == nil {
		return nil
	}
	switch {
	case desired == ActionPause && st.StopState == "" && c.StopApps != nil && c.StopApps(ctx):
		if err := c.Sleeper.Pause(ctx, st.App); err != nil {
			return fmt.Errorf("stop capped app: %w", err)
		}
		st.StopState = StopPaused
		add(EventCapPaused, "containers stopped by the spend cap; they stay down until the limit resets or is raised", st.Pct)
		c.notify(ctx, st.App, "stopped by its spend cap")
	case desired != ActionPause && st.StopState != "":
		if st.StopState == StopPaused {
			if err := c.Sleeper.Resume(ctx, st.App); err != nil {
				return fmt.Errorf("start capped app: %w", err)
			}
			add(EventCapResumed, "spend cap lifted; the app was started again", st.Pct)
			c.notify(ctx, st.App, "started again, its spend cap was lifted")
		}
		st.StopState = ""
	}
	return nil
}

func (c *Controller) notify(ctx context.Context, app, text string) {
	if c.Notify != nil {
		c.Notify(ctx, app, text)
	}
}

// ResumeNow starts a cap-stopped app by hand. The cap keeps its action, but
// the app is not stopped again until the limit lifts or the month rolls over.
func (c *Controller) ResumeNow(ctx context.Context, app, actor, reason string) error {
	st, ok, err := c.Store.GetSpendCapState(ctx, app)
	if err != nil {
		return fmt.Errorf("spendcap: load state: %w", err)
	}
	if !ok || st.StopState != StopPaused || c.Sleeper == nil {
		return ErrNotPaused
	}
	if err := c.Sleeper.Resume(ctx, app); err != nil {
		return fmt.Errorf("spendcap: start app: %w", err)
	}
	st.StopState = StopResumed
	ev := Event{App: app, Period: st.Period, Kind: EventCapResumed, Detail: fmt.Sprintf("%s started the app by hand: %s", actor, reason), Pct: st.Pct, At: c.now()}
	if err := c.Store.SaveSpendCapState(ctx, st, []Event{ev}); err != nil {
		return fmt.Errorf("spendcap: save state: %w", err)
	}
	return nil
}
