package appsleep

import (
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Idle decision reasons, shown in logs and used by tests.
const (
	ReasonIdle        = "idle"
	ReasonRecentUse   = "recent_activity"
	ReasonConnections = "open_connections"
	ReasonOperation   = "operation_in_flight"
	ReasonCooldown    = "wake_cooldown"
	ReasonUnobserved  = "window_not_observed"
)

// DefaultIgnorePaths are request routes that never count as traffic: health
// checks, readiness endpoints and uptime or status page probes.
var DefaultIgnorePaths = []string{"/health", "/healthz", "/ready", "/readyz", "/livez", "/ping", "/status", "/up"}

// EffectiveIgnorePaths merges the defaults, the app's readiness path and the
// operator's list, normalized the way request routes are stored.
func EffectiveIgnorePaths(svc *store.DesiredService, custom []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		r := telemetry.NormalizeRoute(p, telemetry.RouteDepthFromEnv())
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, p := range DefaultIgnorePaths {
		add(p)
	}
	if svc != nil && svc.Health != nil {
		for _, pr := range []*store.ServiceProbe{svc.Health.Readiness, svc.Health.Liveness} {
			if pr != nil && !pr.TCP && len(pr.Exec) == 0 {
				add(pr.Path)
			}
		}
	}
	for _, p := range custom {
		add(p)
	}
	sort.Strings(out)
	return out
}

// Signals is what was seen for one app in the idle window.
type Signals struct {
	// LastRequest is the newest real (not ignored) ingress request, zero if none.
	LastRequest time.Time
	// LastNetwork is the newest moment container traffic was above the floor.
	LastNetwork time.Time
	// OpenConnections counts established connections to the app right now.
	OpenConnections int
	// OperationInFlight is true while a deploy, build or rollback runs.
	OperationInFlight bool
}

// DecideInput is everything Decide needs.
type DecideInput struct {
	Now         time.Time
	IdleMinutes int
	// Since is when the app was enabled or last woke.
	Since time.Time
	// ObservedFrom is when this process started watching; earlier time is unknown.
	ObservedFrom time.Time
	Cooldown     time.Duration
	Signals      Signals
}

// Decision is the outcome of one idle check.
type Decision struct {
	Sleep  bool
	Reason string
	// LastActivity is the newest sign of use, or the baseline when there is none.
	LastActivity time.Time
	IdleFor      time.Duration
	// WillSleepAt is when the app would sleep if nothing else happens, zero when held.
	WillSleepAt time.Time
}

// Decide says whether an app is idle for its whole window. Idle means no real
// request, no network traffic above the floor and no open connection since the
// newest of: enabling, the last wake, and when this process began watching.
func Decide(in DecideInput) Decision {
	window := time.Duration(in.IdleMinutes) * time.Minute
	last := in.Since
	if in.ObservedFrom.After(last) {
		last = in.ObservedFrom
	}
	for _, t := range []time.Time{in.Signals.LastRequest, in.Signals.LastNetwork} {
		if t.After(last) {
			last = t
		}
	}
	if last.After(in.Now) {
		last = in.Now
	}
	d := Decision{LastActivity: last, IdleFor: in.Now.Sub(last), WillSleepAt: last.Add(window)}
	if in.Now.Before(in.Since) {
		d.IdleFor, d.Reason = 0, ReasonRecentUse
		return d
	}
	switch {
	case in.Signals.OperationInFlight:
		d.Reason, d.WillSleepAt = ReasonOperation, time.Time{}
	case in.Signals.OpenConnections > 0:
		d.Reason, d.WillSleepAt = ReasonConnections, time.Time{}
	case d.IdleFor < window:
		d.Reason = ReasonRecentUse
		if in.ObservedFrom.After(in.Since) && last.Equal(in.ObservedFrom) {
			d.Reason = ReasonUnobserved
		}
	case in.Now.Sub(in.Since) < in.Cooldown:
		d.Reason, d.WillSleepAt = ReasonCooldown, in.Since.Add(in.Cooldown)
	default:
		d.Sleep, d.Reason = true, ReasonIdle
	}
	return d
}

// NetSample is one cumulative rx plus tx byte reading.
type NetSample struct {
	At    time.Time
	Bytes float64
}

// NetworkActivity returns the newest time the traffic rate between two
// consecutive samples exceeded floorPerMin bytes per minute. Counter resets
// and gaps over maxGap are skipped; samples are oldest first.
func NetworkActivity(samples []NetSample, floorPerMin int64, maxGap time.Duration) time.Time {
	var newest time.Time
	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1], samples[i]
		dt := b.At.Sub(a.At)
		delta := b.Bytes - a.Bytes
		if dt <= 0 || delta < 0 || (maxGap > 0 && dt > maxGap) {
			continue
		}
		if delta/dt.Minutes() > float64(floorPerMin) {
			newest = b.At
		}
	}
	return newest
}

// MergeNetwork sums rx and tx samples taken at the same instants into one series.
func MergeNetwork(rx, tx []telemetry.Sample) []NetSample {
	byTime := map[time.Time]float64{}
	for _, s := range rx {
		byTime[s.Timestamp] += s.Value
	}
	for _, s := range tx {
		byTime[s.Timestamp] += s.Value
	}
	out := make([]NetSample, 0, len(byTime))
	for t, v := range byTime {
		out = append(out, NetSample{At: t, Bytes: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// RealRequests subtracts the requests on ignored routes from total. A route
// breakdown that is missing (nil routes while total > 0) cannot discount
// anything, so every request counts.
func RealRequests(total float64, routes []telemetry.RouteRow, ignore []string) float64 {
	skip := map[string]bool{}
	for _, p := range ignore {
		skip[p] = true
	}
	var ignored float64
	for _, r := range routes {
		if skip[r.Route] {
			ignored += r.Requests
		}
	}
	if ignored >= total {
		return 0
	}
	return total - ignored
}
