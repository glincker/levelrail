package alerting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// KindAppUnhealthy fires while an app container with a liveness probe is failing it past the
// threshold, and resolves once the probe passes again. ResourceID service:<app> narrows it to one app.
const KindAppUnhealthy Kind = "app_unhealthy"

// appHealthResourcePrefix scopes an app_unhealthy rule to one app.
const appHealthResourcePrefix = "service:"

// UnhealthyApp is one container the liveness probe has failed past its threshold and not yet passed again.
type UnhealthyApp struct {
	Container string
	Service   string
	Failures  int
	Since     time.Time
}

// AppHealthSource lists containers currently failing their liveness probe.
type AppHealthSource interface {
	UnhealthyApps() []UnhealthyApp
}

// AppHealthTracker holds unhealthy containers in memory, so a restart starts clear and the
// next failed probes re-establish a real problem.
type AppHealthTracker struct {
	mu   sync.Mutex
	down map[string]UnhealthyApp
}

// NewAppHealthTracker returns an empty tracker.
func NewAppHealthTracker() *AppHealthTracker {
	return &AppHealthTracker{down: map[string]UnhealthyApp{}}
}

// Record marks a container unhealthy, or clears it once its probe passes again.
func (t *AppHealthTracker) Record(container, service string, unhealthy bool, failures int, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !unhealthy {
		delete(t.down, container)
		return
	}
	if cur, ok := t.down[container]; ok {
		cur.Failures = failures
		t.down[container] = cur
		return
	}
	t.down[container] = UnhealthyApp{Container: container, Service: service, Failures: failures, Since: at}
}

// UnhealthyApps implements AppHealthSource, ordered by container name.
func (t *AppHealthTracker) UnhealthyApps() []UnhealthyApp {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]UnhealthyApp, 0, len(t.down))
	for _, d := range t.down {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Container < out[j].Container })
	return out
}

// EvaluateAppUnhealthy fires while any in-scope container is unhealthy. The probe threshold already
// confirmed the failure, so the rule's own ForDuration is the only extra delay.
func EvaluateAppUnhealthy(_ context.Context, src AppHealthSource, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now
	scope, _ := strings.CutPrefix(r.ResourceID, appHealthResourcePrefix)
	if !strings.HasPrefix(r.ResourceID, appHealthResourcePrefix) {
		scope = ""
	}
	parts := []string{}
	for _, d := range src.UnhealthyApps() {
		if scope != "" && d.Service != scope {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s failed its health check %d times", cleanAlertText(d.Container), d.Failures))
	}
	v := float64(len(parts))
	next.LastValue = &v
	next = advanceState(next, r, len(parts) > 0, r.ForDuration, now)
	if len(parts) == 0 {
		return next, "", nil
	}
	return next, "app unhealthy: " + strings.Join(parts, "; "), nil
}
