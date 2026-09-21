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

const containerHashLen = 8

// RestartTracker watches Docker's event stream and records each "start"
// event as a restart for whichever service owns that container,
// keyed the same "service:<name>" way every other telemetry identifier
// in this codebase is.
//
// Counts "start" events, not "die" events: a container dying once is
// an ordinary crash, what makes it a crashloop is the reconciler
// repeatedly restarting the same container afterward (containers never
// carry Docker's own restart policy, the reconciler
// is the sole authority on bringing a dead container back, so every
// restart is visible as this process re-issuing Start on the same
// container ID). The very first start observed for a given container
// name is never counted as a restart of anything: a fresh deploy
// produces a brand-new container name (internal/reconcile/application's
// deterministic per-image naming), so its first start is ordinary
// startup, not a restart, without needing to special-case "was this a
// deploy or a crash" any other way.
type RestartTracker struct {
	mu       sync.Mutex
	restarts map[string][]time.Time // resourceID -> restart timestamps, oldest first
	seen     map[string]bool        // container names whose first start has already been observed
}

// NewRestartTracker builds an empty RestartTracker.
func NewRestartTracker() *RestartTracker {
	return &RestartTracker{
		restarts: make(map[string][]time.Time),
		seen:     make(map[string]bool),
	}
}

// Observe records one "start" event for containerName, owned by
// resourceID, at time at. Safe for concurrent use.
func (t *RestartTracker) Observe(resourceID, containerName string, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.seen[containerName] {
		t.seen[containerName] = true
		return
	}
	t.restarts[resourceID] = append(t.restarts[resourceID], at)
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

// Prune drops restart records older than olderThan, so this map doesn't
// grow without bound over a long-running process. Call periodically
// (Run does this once per resync), not per-evaluation.
func (t *RestartTracker) Prune(olderThan time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
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

// Run consumes source's event stream until ctx is done, resolving each
// "start" event's container name against a service list refreshed
// every resync tick (services are created/removed over time; a
// container belonging to a brand-new service must resolve correctly
// without a process restart, the same level-triggered "re-derive, don't
// cache forever" principle every reconcile.Controller already follows).
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

// maxRestartRetention bounds how long RestartTracker keeps a restart
// timestamp regardless of any single rule's own RestartWindow, so a
// rule reconfigured to a much longer window later doesn't silently miss
// restarts this process already discarded. Generous on purpose: this is
// an in-memory, not persisted, retention (a process restart loses
// restart history entirely, a real, accepted limitation), so keeping
// a day of history costs little.
const maxRestartRetention = 24 * time.Hour

// resolveResourceID reports whether containerName is exactly one of
// services' own containers (serviceName + "-" + an 8-hex-char image
// hash), the same exact-match check
// internal/reconcile/application.ownsContainer already established and
// for the identical reason: a bare prefix match would let a service
// named "web" match a container actually belonging to "web-worker".
func resolveResourceID(services []store.DesiredService, containerName string) (resourceID string, ok bool) {
	for _, svc := range services {
		suffix, hasPrefix := strings.CutPrefix(containerName, svc.Name+"-")
		if !hasPrefix || len(suffix) != containerHashLen {
			continue
		}
		if !isHexString(suffix) {
			continue
		}
		return "service:" + svc.Name, true
	}
	return "", false
}

func isHexString(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// EvaluateCrashloop runs one KindCrashloop rule against tracker and
// returns its updated evaluation state, the crashloop equivalent of
// EvaluateThreshold, sharing the same pending/firing debounce logic via
// advanceState. Passes forDuration=0: a crashloop rule fires the
// instant its restart count crosses the threshold, with no additional
// debounce layered on top, because RestartWindow itself already is the
// debounce (requiring N restarts within a real time window is already
// "sustained," unlike a single noisy metric sample).
func EvaluateCrashloop(tracker *RestartTracker, r Rule, now time.Time) Rule {
	count := tracker.CountSince(r.ResourceID, now.Add(-r.RestartWindow))

	next := r
	next.LastEvaluatedAt = &now
	v := float64(count)
	next.LastValue = &v

	return advanceState(next, r, count >= r.RestartCountThreshold, 0, now)
}

// AutoRollbackTracker records, per resourceID, the image MaybeAutoRollback
// most recently rolled a service's desired state back to. A KindCrashloop
// rule's own Firing bool is *not* enough of a debounce on its own: once a
// rollback fixes one bad deploy, the rule can stay continuously Firing
// (never re-transitioning through becameFiring) simply because the prior
// episode's restarts are still inside RestartWindow, even after a second,
// genuinely different bad image starts crash-looping. This tracker gives
// MaybeAutoRollback a second, independent signal for that case: if the
// service's current desired image no longer matches the image it was last
// rolled back to, something changed the desired state since (a new
// deploy), so it is safe, and necessary, to roll back again. If the
// current image still matches, nothing has changed since the last
// rollback, the ongoing Firing state is still the same incident, and
// MaybeAutoRollback must not fire again.
//
// In-memory only, the same shape as RestartTracker above: a control-plane
// restart already zeroes RestartTracker's own counts, which independently
// forces every crashloop rule back through a fresh pending-to-firing
// transition before it can fire at all, so losing this map's entries on
// restart never produces a wrong rollback decision, at worst a rule
// revisits becameFiring once more before this fast path resumes tracking
// it.
type AutoRollbackTracker struct {
	mu     sync.Mutex
	target map[string]string // resourceID -> image last auto-rolled-back to
}

// NewAutoRollbackTracker builds an empty AutoRollbackTracker.
func NewAutoRollbackTracker() *AutoRollbackTracker {
	return &AutoRollbackTracker{target: make(map[string]string)}
}

// alreadyHandled reports whether resourceID was already auto-rolled-back
// to exactly currentImage, meaning nothing has changed since and
// MaybeAutoRollback must not fire again. A nil tracker (never wired)
// always reports false, the same "absence degrades, never errors" shape
// MaybeAutoRollback's other optional dependencies already follow.
func (t *AutoRollbackTracker) alreadyHandled(resourceID, currentImage string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.target[resourceID]
	return ok && last == currentImage
}

// armed reports whether resourceID has ever had an auto-rollback recorded.
// Engine.Tick uses this as a cheap in-memory check before deciding whether
// a still-firing rule is even worth a store round trip to re-examine: a
// service that has never auto-rolled-back has nothing new to detect here.
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

// AutoRollbackStore is the narrow store surface MaybeAutoRollback needs:
// read a service's current desired state (to check the opt-in flag and
// current image) and its deploy history (to find a prior known-good
// tag), plus deploy.ImageDeployStore's write surface to actually trigger
// the rollback through the same path a manual one uses. *store.DB
// satisfies this structurally.
type AutoRollbackStore interface {
	deploy.ImageDeployStore
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
}

// MaybeAutoRollback checks whether resourceID's app has opted into
// automatic crashloop rollback (store.DesiredService.
// AutoRollbackOnCrashloop) and, if so, triggers a deploy back to the
// most recent different successful image via deploy.TriggerImageDeploy,
// the identical path a manual "Rollback to this build" action already
// uses (see that function's own doc comment). Called both on a
// KindCrashloop rule's pending-to-firing transition (Engine.Tick's
// becameFiring) and on every tick a rule stays continuously firing
// (Engine.Tick's stillFiring, gated on tracker.armed so it only costs a
// store round trip for a service that has already auto-rolled-back at
// least once): tracker's alreadyHandled check below is what still
// prevents firing twice for the same incident in both cases, a rule
// staying Firing is no longer, by itself, proof that nothing new has
// happened, since a second bad deploy inside the same RestartWindow never
// produces a fresh transition.
//
// Failures and "nothing to do" cases are logged, never returned: a
// crashloop rule's own notification already fired regardless of what
// happens here, and an automatic safety net that panics or blocks
// evaluation of the next rule would be worse than one that occasionally
// leaves a crashloop to alert-only.
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
