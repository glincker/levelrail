package alerting

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EventSource is the narrow Docker surface RestartTracker needs.
// *docker.Client satisfies this structurally (it already implements
// Runtime, which embeds Events).
type EventSource interface {
	Events(ctx context.Context) (<-chan docker.Event, <-chan error)
}

// ServiceLister is the narrow store surface RestartTracker needs to
// resolve a container name to the service that owns it.
// *store.DB satisfies this structurally.
type ServiceLister interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
}

// RestartRecorder is the narrow store surface RestartTracker needs, defined here so internal/telemetry never imports alerting.
type RestartRecorder interface {
	RecordContainerRestart(ctx context.Context, serviceName string, at time.Time) error
}

const containerHashLen = 8

// RestartTracker counts each container start after a container's first as a restart, since a crashloop is repeated restarts.
type RestartTracker struct {
	mu       sync.Mutex
	restarts map[string][]time.Time // resourceID -> restart timestamps, oldest first
	byName   map[string][]time.Time // container name -> restart timestamps, oldest first
	seen     map[string]bool        // container names whose first start has already been observed
	recorder RestartRecorder        // optional: persists each restart as a metric sample, nil in every existing test in this package
	logger   *slog.Logger
}

// NewRestartTracker returns a tracker that keeps only in-memory counts until WithRecorder is called.
func NewRestartTracker() *RestartTracker {
	return &RestartTracker{
		restarts: make(map[string][]time.Time),
		byName:   make(map[string][]time.Time),
		seen:     make(map[string]bool),
	}
}

// WithRecorder persists each counted restart, and a failed write is logged without stopping Observe.
func (t *RestartTracker) WithRecorder(recorder RestartRecorder, logger *slog.Logger) *RestartTracker {
	if logger == nil {
		logger = slog.Default()
	}
	t.recorder = recorder
	t.logger = logger
	return t
}

// Observe records one start event for containerName, owned by resourceID, and is safe for concurrent use.
func (t *RestartTracker) Observe(resourceID, containerName string, at time.Time) {
	t.mu.Lock()
	if !t.seen[containerName] {
		t.seen[containerName] = true
		t.mu.Unlock()
		return
	}
	t.restarts[resourceID] = append(t.restarts[resourceID], at)
	t.byName[containerName] = append(t.byName[containerName], at)
	recorder := t.recorder
	logger := t.logger
	t.mu.Unlock()

	if recorder == nil {
		return
	}
	serviceName, ok := strings.CutPrefix(resourceID, "service:")
	if !ok || serviceName == "" {
		return
	}
	if err := recorder.RecordContainerRestart(context.Background(), serviceName, at); err != nil {
		logger.Error("alerting: crashloop: record restart metric failed",
			slog.String("error", err.Error()), slog.String("resource_id", resourceID))
	}
}

// CountSince returns how many restarts resourceID has had strictly
// after since. Safe for concurrent use.
func (t *RestartTracker) CountSince(resourceID string, since time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	count := 0
	for _, ts := range t.restarts[resourceID] {
		if ts.After(since) {
			count++
		}
	}
	return count
}

// CountContainersSince is CountSince for specific containers, so a release's
// watch is not charged for restarts of another release of the same service.
func (t *RestartTracker) CountContainersSince(names []string, since time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	count := 0
	for _, name := range names {
		for _, ts := range t.byName[name] {
			if ts.After(since) {
				count++
			}
		}
	}
	return count
}

// Prune drops restart records older than olderThan, so this map doesn't
// grow without bound over a long-running process. Call periodically
// (Run does this once per resync), not per-evaluation.
func (t *RestartTracker) Prune(olderThan time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for name, times := range t.byName {
		kept := times[:0]
		for _, ts := range times {
			if ts.After(olderThan) {
				kept = append(kept, ts)
			}
		}
		if len(kept) == 0 {
			delete(t.byName, name)
		} else {
			t.byName[name] = kept
		}
	}
	for id, times := range t.restarts {
		kept := times[:0]
		for _, ts := range times {
			if ts.After(olderThan) {
				kept = append(kept, ts)
			}
		}
		if len(kept) == 0 {
			delete(t.restarts, id)
		} else {
			t.restarts[id] = kept
		}
	}
}

// Run consumes source's events until ctx is done, re-resolving services on every resync so new services work without a restart.
func (t *RestartTracker) Run(ctx context.Context, source EventSource, services ServiceLister, resync time.Duration, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	events, errs := source.Events(ctx)

	var known []store.DesiredService
	refresh := func() {
		svcs, err := services.ListDesiredServices(ctx)
		if err != nil {
			logger.Error("alerting: crashloop: refresh service list failed", slog.String("error", err.Error()))
			return
		}
		known = svcs
	}
	refresh()

	ticker := time.NewTicker(resync)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			refresh()
			t.Prune(time.Now().Add(-maxRestartRetention))
		case err, ok := <-errs:
			if ok && err != nil {
				logger.Error("alerting: crashloop: event stream error", slog.String("error", err.Error()))
			}
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if ev.Action != docker.EventStart {
				continue
			}
			resourceID, ok := resolveResourceID(known, ev.ContainerName)
			if !ok {
				continue // not a container this tracker's known services own (someone else's container, or a service deleted since the last refresh)
			}
			t.Observe(resourceID, ev.ContainerName, ev.Time)
		}
	}
}

// Kept generously because restart history is in-memory only and is lost on process restart anyway.
const maxRestartRetention = 24 * time.Hour

// Matches exactly, since a prefix match would make service web claim web-worker's containers.
func resolveResourceID(services []store.DesiredService, containerName string) (resourceID string, ok bool) {
	for _, svc := range services {
		suffix, hasPrefix := strings.CutPrefix(containerName, svc.Name+"-")
		if !hasPrefix {
			continue
		}
		// Replicas 1 and up carry an extra "-rN" after the hash.
		hash, replica, hasReplica := strings.Cut(suffix, "-r")
		if len(hash) != containerHashLen || !isHexString(hash) {
			continue
		}
		if hasReplica && !isDecimal(replica) {
			continue
		}
		return "service:" + svc.Name, true
	}
	return "", false
}

func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHexString(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// EvaluateCrashloop evaluates one crashloop rule with no extra debounce, since RestartWindow already requires sustained restarts.
func EvaluateCrashloop(tracker *RestartTracker, r Rule, now time.Time) Rule {
	count := tracker.CountSince(r.ResourceID, now.Add(-r.RestartWindow))

	next := r
	next.LastEvaluatedAt = &now
	v := float64(count)
	next.LastValue = &v

	return advanceState(next, r, count >= r.RestartCountThreshold, 0, now)
}

// AutoRollbackTracker records the image each service was last auto-rolled back to, so a new bad deploy can trigger another rollback while the rule still fires.
type AutoRollbackTracker struct {
	mu     sync.Mutex
	target map[string]string // resourceID -> image last auto-rolled-back to
}

// NewAutoRollbackTracker builds an empty AutoRollbackTracker.
func NewAutoRollbackTracker() *AutoRollbackTracker {
	return &AutoRollbackTracker{target: make(map[string]string)}
}

// Reports whether resourceID was already rolled back to currentImage; a nil tracker always reports false.
func (t *AutoRollbackTracker) alreadyHandled(resourceID, currentImage string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.target[resourceID]
	return ok && last == currentImage
}

// Lets Engine.Tick skip a store round trip for services that never auto-rolled back.
func (t *AutoRollbackTracker) armed(resourceID string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.target[resourceID]
	return ok
}

// record stores image as the one resourceID was most recently
// auto-rolled-back to. Safe for concurrent use.
func (t *AutoRollbackTracker) record(resourceID, image string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.target[resourceID] = image
}

// AutoRollbackStore is the narrow store surface MaybeAutoRollback needs, which *store.DB satisfies.
type AutoRollbackStore interface {
	deploy.ImageDeployStore
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
}

// MaybeAutoRollback rolls resourceID back to its previous good image when opted in, and logs rather than returns failures.
func MaybeAutoRollback(ctx context.Context, st AutoRollbackStore, nudger deploy.ReconcileNudger, tracker *AutoRollbackTracker, resourceID string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	name, ok := strings.CutPrefix(resourceID, "service:")
	if !ok || name == "" {
		return
	}

	svc, err := st.GetDesiredService(ctx, name)
	if err != nil {
		logger.Error("alerting: crashloop auto-rollback: load app failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	if !svc.AutoRollbackOnCrashloop {
		return
	}
	if tracker.alreadyHandled(resourceID, svc.Image) {
		return
	}

	attempts, err := st.ListDeployAttempts(ctx, name)
	if err != nil {
		logger.Error("alerting: crashloop auto-rollback: list deploy attempts failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	image, ok := deploy.PreviousKnownGoodImage(attempts, svc.Image)
	if !ok {
		logger.Warn("alerting: crashloop auto-rollback: no older known-good image to fall back to, leaving crashloop to alert only", slog.String("name", name), slog.String("image", svc.Image))
		return
	}

	if _, err := deploy.TriggerImageDeploy(ctx, st, nudger, *svc, image, store.DeployAttemptSourceAutoRollback, logger); err != nil {
		logger.Error("alerting: crashloop auto-rollback: trigger deploy failed", slog.String("name", name), slog.String("image", image), slog.String("error", err.Error()))
		return
	}
	tracker.record(resourceID, image)
	logger.Warn("alerting: crashloop auto-rollback: rolled back to prior image", slog.String("name", name), slog.String("from_image", svc.Image), slog.String("to_image", image))
}
