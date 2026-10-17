package autoscale

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// Event kinds recorded in an app's history.
const (
	EventScaledUp   = "scaled_up"
	EventScaledDown = "scaled_down"
	EventHeld       = "held"
	EventFailed     = "failed"
	EventConfigured = "configured"
	EventCleared    = "cleared"
)

const (
	flapWindow    = time.Hour
	flapScanLimit = 100
)

// State is what the controller last decided for an app, plus the memory a
// later decision needs (cooldown timestamps and the stabilization window).
type State struct {
	App           string
	Current       int
	Desired       int
	Value         float64
	ValueKnown    bool
	Reason        string
	BlockedBy     string
	Message       string
	LastScaleUp   time.Time
	LastScaleDown time.Time
	DownSince     time.Time
	DownFloor     int
	UpdatedAt     time.Time
}

// Event is one entry of an app's decision history.
type Event struct {
	ID        int64     `json:"id"`
	App       string    `json:"app"`
	Kind      string    `json:"kind"`
	Current   int       `json:"current"`
	Desired   int       `json:"desired"`
	Value     float64   `json:"value"`
	Reason    string    `json:"reason"`
	BlockedBy string    `json:"blocked_by"`
	Detail    string    `json:"detail"`
	At        time.Time `json:"at"`
}

// Store is the persistence the controller needs; *store.DB satisfies it.
type Store interface {
	ListAutoscalePolicies(ctx context.Context) ([]Policy, error)
	GetAutoscalePolicy(ctx context.Context, app string) (Policy, error)
	SaveAutoscalePolicy(ctx context.Context, p Policy, ev Event, now time.Time) error
	DeleteAutoscalePolicy(ctx context.Context, app string, ev Event) error
	GetAutoscaleState(ctx context.Context, app string) (State, bool, error)
	SaveAutoscaleState(ctx context.Context, st State, events []Event) error
	ListAutoscaleEvents(ctx context.Context, app string, limit int) ([]Event, error)
	// AutoscaleReplicas returns the app's desired replica count and whether it
	// is suspended, or ErrNotFound.
	AutoscaleReplicas(ctx context.Context, app string) (replicas int, suspended bool, err error)
	// SetAutoscaleReplicas writes the app's desired replica count. It is the
	// only writer of that column besides a manual update.
	SetAutoscaleReplicas(ctx context.Context, app string, replicas int) error
}

// Reading is one metric observation for a policy.
type Reading struct {
	Value float64
	Known bool
	// Note says why the value is unknown.
	Note string
}

// Signals is the observed world the controller reads.
type Signals interface {
	Metric(ctx context.Context, p Policy, current int, now time.Time) (Reading, error)
	// Headroom reports how many of extra more replicas fit on the app's node,
	// or a negative number when that cannot be told.
	Headroom(ctx context.Context, app string, extra int) (int, error)
	// SpendEnforced returns the spend cap action in force for the app, "" when none.
	SpendEnforced(ctx context.Context, app string) (string, error)
}

// Gauge is one app's snapshot, recorded as metrics so a plain threshold alert
// rule can watch NoCapacity and flapping.
type Gauge struct {
	App        string
	Current    int
	Desired    int
	NoCapacity bool
	Flaps      int
}

// Recorder persists gauges. It is optional.
type Recorder interface {
	RecordGauges(ctx context.Context, gauges []Gauge, at time.Time) error
}

// Controller converges each autoscaled app's replica count on its load. It
// holds no state of its own: every tick re-derives the decision from the
// store and the signals, so a restart or a missed tick changes nothing.
type Controller struct {
	Store         Store
	Signals       Signals
	Recorder      Recorder
	Nudge         func()
	Ceiling       int
	FlapThreshold int
	Logger        *slog.Logger
	Now           func() time.Time
}

func (c *Controller) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Tick reconciles every policy. One app failing never stops the others.
func (c *Controller) Tick(ctx context.Context) error {
	policies, err := c.Store.ListAutoscalePolicies(ctx)
	if err != nil {
		return fmt.Errorf("autoscale: list policies: %w", err)
	}
	now := c.now()
	var errs []error
	for _, p := range policies {
		if err := c.Reconcile(ctx, p, now); err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", p.App, err))
		}
	}
	return errors.Join(errs...)
}

// Reconcile makes one decision for one app and applies it.
func (c *Controller) Reconcile(ctx context.Context, p Policy, now time.Time) error {
	current, suspended, err := c.Store.AutoscaleReplicas(ctx, p.App)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load replicas: %w", err)
	}
	current = max(current, 1)
	st, _, err := c.Store.GetAutoscaleState(ctx, p.App)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	dec := Decision{Current: current, Desired: current, Reason: ReasonSuspended, BlockedBy: BlockedSuspended,
		Message: "the app is stopped, asleep or frozen, so its replicas are left alone"}
	if !suspended {
		dec = Decide(p, c.input(ctx, p, st, current, now))
	}
	return c.apply(ctx, p, st, dec, now)
}

func (c *Controller) input(ctx context.Context, p Policy, st State, current int, now time.Time) Input {
	in := Input{
		Now: now, Current: current, Ceiling: c.Ceiling,
		LastScaleUp: st.LastScaleUp, LastScaleDown: st.LastScaleDown,
		DownSince: st.DownSince, DownFloor: st.DownFloor,
	}
	reading, err := c.Signals.Metric(ctx, p, current, now)
	switch {
	case err != nil:
		in.MetricNote = "the metric query failed: " + err.Error()
		c.warn(ctx, "autoscale: metric query failed", p.App, err)
	default:
		in.Value, in.ValueKnown, in.MetricNote = reading.Value, reading.Known, reading.Note
	}
	enforced, err := c.Signals.SpendEnforced(ctx, p.App)
	if err != nil {
		in.SpendBlocked = true
		c.warn(ctx, "autoscale: spend cap state unreadable, not adding replicas", p.App, err)
	} else {
		in.SpendBlocked = enforced != ""
	}
	in.Headroom = func(extra int) int {
		n, err := c.Signals.Headroom(ctx, p.App, extra)
		if err != nil {
			c.warn(ctx, "autoscale: capacity check failed, treating as unknown", p.App, err)
			return -1
		}
		return n
	}
	return in
}

func (c *Controller) warn(ctx context.Context, msg, app string, err error) {
	if c.Logger != nil {
		c.Logger.WarnContext(ctx, msg, slog.String("app", app), slog.String("error", err.Error()))
	}
}

func (c *Controller) apply(ctx context.Context, p Policy, prev State, dec Decision, now time.Time) error {
	next := State{
		App: p.App, Current: dec.Current, Desired: dec.Desired, Value: dec.Value,
		ValueKnown: dec.Reason != ReasonMetricsUnavailable && dec.Reason != ReasonSuspended,
		Reason:     dec.Reason, BlockedBy: dec.BlockedBy, Message: dec.Message,
		LastScaleUp: prev.LastScaleUp, LastScaleDown: prev.LastScaleDown,
		DownSince: dec.DownSince, DownFloor: dec.DownFloor, UpdatedAt: now,
	}
	ev := Event{App: p.App, Current: dec.Current, Desired: dec.Desired, Value: dec.Value,
		Reason: dec.Reason, BlockedBy: dec.BlockedBy, Detail: dec.Message, At: now}
	var events []Event
	switch {
	case dec.Changed() && dec.Desired > dec.Current:
		next.LastScaleUp = now
		ev.Kind = EventScaledUp
		events = append(events, ev)
	case dec.Changed():
		next.LastScaleDown = now
		ev.Kind = EventScaledDown
		events = append(events, ev)
	case prev.Reason != dec.Reason || prev.BlockedBy != dec.BlockedBy:
		if prev.Reason != "" || dec.Reason != ReasonWithinTarget {
			ev.Kind = EventHeld
			events = append(events, ev)
		}
	}
	// Intent and cooldown are saved before the replica write: a crash between
	// the two can only make the next scale slower, never faster or doubled.
	if err := c.Store.SaveAutoscaleState(ctx, next, events); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if dec.Changed() {
		if err := c.Store.SetAutoscaleReplicas(ctx, p.App, dec.Desired); err != nil {
			c.undo(ctx, p, prev, next, ev, err)
			return fmt.Errorf("set replicas to %d: %w", dec.Desired, err)
		}
		if c.Nudge != nil {
			c.Nudge()
		}
		if c.Logger != nil {
			c.Logger.InfoContext(ctx, "autoscale: replicas changed", slog.String("app", p.App),
				slog.Int("from", dec.Current), slog.Int("to", dec.Desired), slog.String("reason", dec.Reason), slog.String("blocked_by", dec.BlockedBy))
		}
	}
	c.record(ctx, next, now)
	return nil
}

// undo gives back the cooldown a failed replica write consumed and says so in the history.
func (c *Controller) undo(ctx context.Context, p Policy, prev, next State, ev Event, cause error) {
	next.LastScaleUp, next.LastScaleDown = prev.LastScaleUp, prev.LastScaleDown
	next.Desired, next.Reason, next.BlockedBy = next.Current, ReasonScaleFailed, ""
	next.Message = "could not write the new replica count: " + cause.Error()
	ev.Kind, ev.Reason, ev.BlockedBy, ev.Desired, ev.Detail = EventFailed, ReasonScaleFailed, "", next.Current, next.Message
	if err := c.Store.SaveAutoscaleState(ctx, next, []Event{ev}); err != nil {
		c.warn(ctx, "autoscale: could not record a failed scale", p.App, err)
	}
}

func (c *Controller) record(ctx context.Context, st State, now time.Time) {
	if c.Recorder == nil {
		return
	}
	events, err := c.Store.ListAutoscaleEvents(ctx, st.App, flapScanLimit)
	if err != nil {
		c.warn(ctx, "autoscale: could not read history for flap count", st.App, err)
	}
	gauge := Gauge{
		App: st.App, Current: st.Current, Desired: st.Desired,
		NoCapacity: st.BlockedBy == BlockedNoCapacity, Flaps: Flaps(events, now, flapWindow),
	}
	if err := c.Recorder.RecordGauges(ctx, []Gauge{gauge}, now); err != nil {
		c.warn(ctx, "autoscale: could not record gauges", st.App, err)
	}
}

// ReasonScaleFailed is recorded when the replica write itself failed.
const ReasonScaleFailed = "ScaleFailed"

// Flaps counts direction reversals between consecutive scale events within
// window of now. events may be in any order.
func Flaps(events []Event, now time.Time, window time.Duration) int {
	var scales []Event
	for _, e := range events {
		if (e.Kind == EventScaledUp || e.Kind == EventScaledDown) && now.Sub(e.At) <= window {
			scales = append(scales, e)
		}
	}
	sort.Slice(scales, func(i, j int) bool { return scales[i].At.Before(scales[j].At) })
	n := 0
	for i := 1; i < len(scales); i++ {
		if scales[i].Kind != scales[i-1].Kind {
			n++
		}
	}
	return n
}
