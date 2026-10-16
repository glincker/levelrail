// Package appsleep stops opted-in apps that have had no requests for a while
// and wakes them on the next request, so idle apps cost no memory or CPU.
package appsleep

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// DefaultInterval is how often idle apps are looked for.
const DefaultInterval = time.Minute

// SleepStore is the sleep settings table.
type SleepStore interface {
	ListAppSleep(ctx context.Context) ([]store.AppSleep, error)
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
}

// Apps is the slice of the app store the scheduler needs.
type Apps interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	UpdateServiceSuspended(ctx context.Context, name string, suspended bool) error
}

// Traffic reports each app's most recent request sample.
type Traffic interface {
	LatestByMetric(ctx context.Context, metric string) ([]telemetry.Sample, error)
}

// Nudger asks the reconcilers to run now.
type Nudger interface{ Nudge() }

// Metrics reads stored samples for the network activity check.
type Metrics interface {
	QueryMany(ctx context.Context, resourceIDs, metrics []string, from, to time.Time) ([]telemetry.Sample, error)
}

// Breakdowns reads per-route request counts, used to discount probe routes.
type Breakdowns interface {
	QueryBreakdown(ctx context.Context, resourceID string, from, to time.Time) (telemetry.Breakdown, error)
}

// ConnCounter counts established connections to an app's pinned host ports.
type ConnCounter interface {
	Open(ctx context.Context, app string, ports []int) int
}

// ActiveOps lists apps with a deploy, build or rollback in flight.
type ActiveOps interface {
	ServicesWithActiveDeploy(ctx context.Context) (map[string]bool, error)
}

// PortSyncer makes the port wake listeners match desired state.
type PortSyncer interface{ Sync(ctx context.Context) }

// Scheduler puts idle opted-in apps to sleep.
type Scheduler struct {
	Sleep   SleepStore
	Apps    Apps
	Traffic Traffic
	Nudger  Nudger
	// Events records slept and woke history; nil skips it.
	Events EventStore
	Logger *slog.Logger
	Now    func() time.Time

	// Optional signals. Without one, that signal is not consulted.
	Metrics    Metrics
	Breakdowns Breakdowns
	Conns      ConnCounter
	Ops        ActiveOps
	Fleet      FleetStore
	PortWake   PortWaker
	PortSync   PortSyncer
	Cfg        Config
	// ObservedFrom starts the observation window; zero means the first tick.
	ObservedFrom time.Time

	mu       sync.Mutex
	lastTick time.Time
	tr       map[string]*appTrack
}

// appTrack is what the scheduler remembers about one app between ticks.
type appTrack struct {
	reqSeen     time.Time
	lastReal    time.Time
	lastNet     time.Time
	netChecked  time.Time
	connections int
	decision    Decision
	decided     bool
}

// Snapshot is the latest activity picture of one app.
type Snapshot struct {
	LastRequest     time.Time
	LastNetwork     time.Time
	OpenConnections int
	Decision        Decision
}

// Snapshot returns what the last tick saw for name.
func (s *Scheduler) Snapshot(name string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tr[name]
	if t == nil || !t.decided {
		return Snapshot{}, false
	}
	return Snapshot{LastRequest: t.lastReal, LastNetwork: t.lastNet, OpenConnections: t.connections, Decision: t.decision}, true
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Scheduler) cfg() Config {
	c := s.Cfg
	if c == (Config{}) {
		return ConfigFromEnv(nil)
	}
	return c
}

func (s *Scheduler) track(name string) *appTrack {
	if s.tr == nil {
		s.tr = map[string]*appTrack{}
	}
	t := s.tr[name]
	if t == nil {
		t = &appTrack{}
		s.tr[name] = t
	}
	return t
}

// observe starts or restarts the observation window after a long gap between
// ticks (control plane down, host suspended), since nothing was seen then.
func (s *Scheduler) observe(now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ObservedFrom.IsZero() || (!s.lastTick.IsZero() && now.Sub(s.lastTick) > 3*DefaultInterval) {
		s.ObservedFrom = now
	}
	s.lastTick = now
	return s.ObservedFrom
}

// Tick sleeps every opted-in app idle past its threshold and clears the
// sleeping flag of any app someone started by hand.
func (s *Scheduler) Tick(ctx context.Context) error {
	settings, err := s.Sleep.ListAppSleep(ctx)
	if err != nil {
		return fmt.Errorf("appsleep: list settings: %w", err)
	}
	samples, err := s.Traffic.LatestByMetric(ctx, telemetry.MetricHTTPRequests)
	if err != nil {
		return fmt.Errorf("appsleep: read request metrics: %w", err)
	}
	latest := make(map[string]time.Time, len(samples))
	for _, smp := range samples {
		latest[smp.ResourceID] = smp.Timestamp
	}
	now := s.now()
	observed := s.observe(now)
	var fleet *Fleet
	if s.Fleet != nil {
		if fleet, err = Load(ctx, s.Fleet); err != nil {
			return err
		}
	}
	ops := map[string]bool{}
	if s.Ops != nil {
		if ops, err = s.Ops.ServicesWithActiveDeploy(ctx); err != nil {
			return fmt.Errorf("appsleep: read active deploys: %w", err)
		}
	}
	net := s.networkActivity(ctx, settings, now)
	cfg := s.cfg()
	var errs []error
	for _, a := range settings {
		svc, err := s.Apps.GetDesiredService(ctx, a.ServiceName)
		if errors.Is(err, store.ErrServiceNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", a.ServiceName, err))
			continue
		}
		if a.Sleeping {
			if !svc.Suspended {
				if err := s.Sleep.SetAppSleeping(ctx, a.ServiceName, false, now); err != nil {
					errs = append(errs, err)
					continue
				}
				e := WokeEvent(a.ServiceName, store.SleepReasonAPI, "system", a.Since, now)
				e.Detail = "started outside sleep when idle"
				Record(ctx, s.Events, s.Logger, e)
			}
			continue
		}
		if svc.Suspended {
			continue
		}
		facts := Facts{App: a.ServiceName, PinnedPorts: PinnedPorts(svc), PortWakeEnabled: a.PortWake}
		if fleet != nil {
			facts = fleet.Facts(svc, a, s.PortWake)
		}
		if bl := Evaluate(facts).Blockers; len(bl) > 0 {
			s.Logger.Debug("appsleep: app cannot sleep", slog.String("app", a.ServiceName), slog.String("reason", bl[0].Code))
			continue
		}
		sig := s.signals(ctx, svc, a, latest[resourceID(a.ServiceName)], now, net[a.ServiceName])
		sig.OperationInFlight = ops[a.ServiceName]
		d := Decide(DecideInput{Now: now, IdleMinutes: a.IdleMinutes, Since: a.Since, ObservedFrom: observed, Cooldown: cfg.WakeCooldown, Signals: sig})
		s.record(a.ServiceName, sig, d)
		if !d.Sleep {
			continue
		}
		if err := s.sleep(ctx, a.ServiceName, a.IdleMinutes, now); err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", a.ServiceName, err))
		}
	}
	if s.PortSync != nil {
		s.PortSync.Sync(ctx)
	}
	return errors.Join(errs...)
}

func (s *Scheduler) record(name string, sig Signals, d Decision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.track(name)
	t.connections, t.decision, t.decided = sig.OpenConnections, d, true
}

// signals gathers the request and connection evidence for one app.
func (s *Scheduler) signals(ctx context.Context, svc *store.DesiredService, a store.AppSleep, reqTS, now time.Time, lastNet time.Time) Signals {
	s.mu.Lock()
	t := s.track(a.ServiceName)
	seen, lastReal := t.reqSeen, t.lastReal
	s.mu.Unlock()
	if reqTS.After(seen) {
		from := seen
		if floor := now.Add(-time.Duration(a.IdleMinutes) * time.Minute); from.Before(floor) {
			from = floor
		}
		if s.hasRealRequests(ctx, svc, a, from.Add(time.Second), now) {
			lastReal = reqTS
		}
		s.mu.Lock()
		t.reqSeen, t.lastReal = reqTS, lastReal
		s.mu.Unlock()
	}
	if reqTS.After(now) && !lastReal.IsZero() {
		lastReal = now
	}
	sig := Signals{LastRequest: lastReal, LastNetwork: lastNet}
	if s.Conns != nil {
		sig.OpenConnections = s.Conns.Open(ctx, a.ServiceName, PinnedPorts(svc))
	}
	return sig
}

// hasRealRequests is true when requests in [from, to] include any outside the
// ignored routes. When it cannot tell, it says true: waking costs less than a
// wrong sleep.
func (s *Scheduler) hasRealRequests(ctx context.Context, svc *store.DesiredService, a store.AppSleep, from, to time.Time) bool {
	if s.Metrics == nil || s.Breakdowns == nil {
		return true
	}
	id := resourceID(a.ServiceName)
	rows, err := s.Metrics.QueryMany(ctx, []string{id}, []string{telemetry.MetricHTTPRequests}, from, to)
	if err != nil {
		return true
	}
	var total float64
	for _, r := range rows {
		total += r.Value
	}
	if total == 0 {
		return false
	}
	bd, err := s.Breakdowns.QueryBreakdown(ctx, id, from, to)
	if err != nil {
		return true
	}
	return RealRequests(total, bd.Routes, EffectiveIgnorePaths(svc, a.IgnorePaths)) > 0
}

// networkActivity updates and returns each awake app's newest above-floor
// network moment, reading only the samples since the last check.
func (s *Scheduler) networkActivity(ctx context.Context, settings []store.AppSleep, now time.Time) map[string]time.Time {
	out := map[string]time.Time{}
	if s.Metrics == nil {
		return out
	}
	var ids []string
	var widest time.Duration
	for _, a := range settings {
		if a.Sleeping {
			continue
		}
		ids = append(ids, resourceID(a.ServiceName))
		if w := time.Duration(a.IdleMinutes) * time.Minute; w > widest {
			widest = w
		}
	}
	if len(ids) == 0 {
		return out
	}
	s.mu.Lock()
	from := now.Add(-widest)
	for _, a := range settings {
		if t := s.tr[a.ServiceName]; t != nil && !t.netChecked.IsZero() && t.netChecked.Add(-time.Minute).After(from) {
			from = t.netChecked.Add(-time.Minute)
		}
	}
	s.mu.Unlock()
	rows, err := s.Metrics.QueryMany(ctx, ids, []string{"network_rx_bytes", "network_tx_bytes"}, from, now)
	if err != nil {
		s.Logger.Warn("appsleep: read network metrics failed", slog.String("error", err.Error()))
		return out
	}
	rx, tx := map[string][]telemetry.Sample{}, map[string][]telemetry.Sample{}
	for _, r := range rows {
		if r.Metric == "network_rx_bytes" {
			rx[r.ResourceID] = append(rx[r.ResourceID], r)
		} else {
			tx[r.ResourceID] = append(tx[r.ResourceID], r)
		}
	}
	floor := s.cfg().NetworkFloorBytesPerMin
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range settings {
		id := resourceID(a.ServiceName)
		t := s.track(a.ServiceName)
		if at := NetworkActivity(MergeNetwork(rx[id], tx[id]), floor, 5*time.Minute); at.After(t.lastNet) {
			t.lastNet = at
		}
		t.netChecked = now
		out[a.ServiceName] = t.lastNet
	}
	return out
}

func (s *Scheduler) sleep(ctx context.Context, name string, idleMinutes int, now time.Time) error {
	if err := Sleep(ctx, s.Sleep, s.Apps, s.Nudger, name, now); err != nil {
		return err
	}
	Record(ctx, s.Events, s.Logger, store.SleepEvent{ServiceName: name, Kind: store.SleepEventSlept, At: now, Reason: store.SleepReasonIdleTimeout, IdleMinutes: idleMinutes, Actor: "system"})
	s.Logger.Info("appsleep: app idle, stopped", slog.String("app", name))
	return nil
}

// Sleep stops name now and marks it asleep, rolling the flag back if the stop fails.
func Sleep(ctx context.Context, sleep interface {
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
}, apps Apps, nudger Nudger, name string, now time.Time) error {
	if err := sleep.SetAppSleeping(ctx, name, true, now); err != nil {
		return err
	}
	if err := apps.UpdateServiceSuspended(ctx, name, true); err != nil {
		_ = sleep.SetAppSleeping(ctx, name, false, now)
		return fmt.Errorf("stop idle app: %w", err)
	}
	if nudger != nil {
		nudger.Nudge()
	}
	return nil
}

// Run ticks on interval until ctx is done.
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				s.Logger.Warn("appsleep: tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

// Wake starts a sleeping app and resets its idle clock. It reports whether the
// app was actually asleep, so repeated wake requests are harmless.
func Wake(ctx context.Context, sleep interface {
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
	GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
}, apps Apps, nudger Nudger, name string, now time.Time) (bool, error) {
	a, err := sleep.GetAppSleep(ctx, name)
	if err != nil {
		return false, err
	}
	if !a.Sleeping {
		return false, nil
	}
	if err := apps.UpdateServiceSuspended(ctx, name, false); err != nil {
		return false, fmt.Errorf("start sleeping app: %w", err)
	}
	if err := sleep.SetAppSleeping(ctx, name, false, now); err != nil {
		return false, err
	}
	if nudger != nil {
		nudger.Nudge()
	}
	return true, nil
}

func resourceID(app string) string { return "service:" + app }
