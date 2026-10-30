package alerting

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// KindMonitorDown is platform-wide: it fires while any uptime monitor holds a confirmed down state (fed by the monitor notifier).
const KindMonitorDown Kind = "monitor_down"

// MonitorDown is one monitor the state machine confirmed down and has not yet confirmed recovered.
type MonitorDown struct {
	MonitorID string
	Name      string
	Reason    string
	Since     time.Time
}

// MonitorDownSource lists monitors currently confirmed down.
type MonitorDownSource interface {
	MonitorsDown() []MonitorDown
}

// MonitorDownTracker holds confirmed-down monitors in memory, so a restart starts clear
// and the monitor engine re-confirms real outages on its own.
type MonitorDownTracker struct {
	mu   sync.Mutex
	down map[string]MonitorDown
}

// NewMonitorDownTracker returns an empty tracker.
func NewMonitorDownTracker() *MonitorDownTracker {
	return &MonitorDownTracker{down: map[string]MonitorDown{}}
}

// Record marks a monitor down, or clears it on confirmed recovery.
func (t *MonitorDownTracker) Record(id, name, reason string, down bool, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !down {
		delete(t.down, id)
		return
	}
	if _, already := t.down[id]; already {
		return
	}
	t.down[id] = MonitorDown{MonitorID: id, Name: name, Reason: reason, Since: at}
}

// Forget drops a deleted monitor so it cannot keep the rule firing.
func (t *MonitorDownTracker) Forget(id string) {
	t.mu.Lock()
	delete(t.down, id)
	t.mu.Unlock()
}

// MonitorsDown implements MonitorDownSource, ordered by name.
func (t *MonitorDownTracker) MonitorsDown() []MonitorDown {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]MonitorDown, 0, len(t.down))
	for _, d := range t.down {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// EvaluateMonitorDown fires while any monitor is confirmed down. Confirmation already happened
// in the monitor state machine, so the rule's own ForDuration is the only extra delay.
func EvaluateMonitorDown(_ context.Context, src MonitorDownSource, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	downs := src.MonitorsDown()
	v := float64(len(downs))
	next.LastValue = &v
	next = advanceState(next, r, len(downs) > 0, r.ForDuration, now)
	if len(downs) == 0 {
		return next, "", nil
	}
	parts := make([]string, 0, len(downs))
	for _, d := range downs {
		s := d.Name
		if d.Reason != "" {
			s += ": " + d.Reason
		}
		parts = append(parts, s)
	}
	return next, "monitor down: " + strings.Join(parts, "; "), nil
}
