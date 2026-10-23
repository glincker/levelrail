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
)

// State is what the controller last decided for an app in a period.
type State struct {
	App    string
	Period string
	// Enforced is "" or the action ingress must apply; Nudged is the value
	// ingress was last told about.
	Enforced    string
	Nudged      string
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
		st = State{App: cp.App, Period: period, Nudged: st.Enforced}
		if !ok {
			st.Nudged = ""
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
	st.Pct, st.EgressBytes, st.Cost, st.UpdatedAt = ev.Pct, u.EgressBytes, u.Cost, now
	if err := c.Store.SaveSpendCapState(ctx, st, events); err != nil {
		return fmt.Errorf("save state: %w", err)
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
	}
	return out, nil
}
