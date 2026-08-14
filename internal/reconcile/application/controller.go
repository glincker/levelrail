// Package application implements the declarative app spec's service
// contract and TASKS.md 1.3's application controller: the
// reconcile.Controller that converges a
// real, store-backed desired service to a running container, replacing
// nginxdemo's hardcoded desired state with the real thing.
//
// Deploy strategy: a container's name is derived deterministically from
// its image (ContainerName), so the level-triggered check "does the
// right container exist" needs no memory of past deploys beyond what's
// queryable from Docker directly. When the desired image changes,
// Reconcile creates a new, differently-named container alongside
// whatever's already running, waits for it to pass its readiness probe,
// then removes every other container for this service. Traffic
// switching itself (updating Caddy to point at the new container) is
// deliberately not this controller's job: this codebase's reconciler
// pattern is a reconcile loop per resource type, and ingress is its own
// resource type (TASKS.md 1.6, not yet wired in). This controller's
// contract with that future ingress controller is simple: whichever
// container currently exists and is running for a service is the one
// meant to receive traffic.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/probe"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ServiceStore is the narrow surface this controller needs from
// internal/store, so tests can fake it without a real database.
// *store.DB satisfies this.
type ServiceStore interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// SecretResolver is the narrow surface this controller needs from
// internal/secrets.Manager (TASKS.md 1.7), so tests can fake it without
// a real master key or database. *secrets.Manager satisfies this
// structurally. Resolve's plaintext return value is used exactly once,
// merged into a container's env map immediately before
// docker.Runtime.Create, and never persisted anywhere by this
// controller.
type SecretResolver interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// DeployRecorder is the narrow surface this controller needs to record
// TASKS.md 2.1's deploy-frequency metric. *telemetry.DB satisfies this
// structurally; not imported directly, same reasoning ServiceStore/
// SecretResolver above already establish. RecordDeploy is only ever
// called on a real deploy cutover (justDeployed below, the "Deployed"
// reason), never on a reconcile tick that finds nothing to do, so a
// series of recorded samples over time IS deploy frequency, no separate
// rate computation needed downstream.
type DeployRecorder interface {
	RecordDeploy(ctx context.Context, serviceName string, at time.Time) error
}

const defaultReadyBudget = 60 * time.Second

// Deploy strategies this controller actually implements. Values match
// internal/spec's StrategyRolling/StrategyRecreate/StrategyBlueGreen
// exactly, kept as this package's own unexported constants rather than
// importing internal/spec, the same "define locally, don't import a
// higher-level package's vocabulary" convention store.DesiredService's
// own Strategy field doc comment already establishes.
//
// strategyRolling is a real, named case Reconcile recognizes and
// reports on (notReady("StrategyNotSupported", ...)), not an
// unrecognized string that happens to fall through to a default: a
// service can already save strategy: rolling today (internal/spec's
// schema has always accepted it), so this controller must say so
// honestly rather than silently reconciling it as something else. See
// this package's own doc comment for why rolling specifically (staggered
// per-replica cutover, partial-success handling) is a larger, separate
// piece of work than generalizing the existing create-alongside-then-
// remove-old shape to N replicas (blue-green) or adding a genuinely
// simpler stop-then-start alternative (recreate).
const (
	strategyRolling   = "rolling"
	strategyRecreate  = "recreate"
	strategyBlueGreen = "blue-green"
)

// Controller converges one named service's desired state (read fresh
// from ServiceStore on every Reconcile, never cached) to a running
// container.
type Controller struct {
	serviceName    string
	store          ServiceStore
	runtime        docker.Runtime
	httpClient     *http.Client
	readyBudget    time.Duration
	secretResolver SecretResolver // nil is valid: a service with no secret-backed env vars never needs one
	deployRecorder DeployRecorder // nil is valid: deploy frequency just isn't recorded
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithHTTPClient overrides the client used for readiness probes.
// Defaults to http.DefaultClient; tests override this to point at a
// fake transport rather than making real HTTP calls to a real client.
func WithHTTPClient(c *http.Client) Option {
	return func(ctrl *Controller) { ctrl.httpClient = c }
}

// WithReadyBudget overrides how long Reconcile waits for a freshly
// started container to pass its readiness probe before giving up.
// Defaults to 60s.
func WithReadyBudget(d time.Duration) Option {
	return func(ctrl *Controller) { ctrl.readyBudget = d }
}

// WithSecretResolver enables container creation to fill in
// secret-backed env vars (store.DesiredService.SecretEnv). Without one
// configured (the default), a service that declares any secret-backed
// env var fails Reconcile loudly rather than silently starting a
// container missing a variable it needs, the same "fail loudly" choice
// internal/deploy.Pipeline makes at save time.
func WithSecretResolver(r SecretResolver) Option {
	return func(ctrl *Controller) { ctrl.secretResolver = r }
}

// WithDeployRecorder enables recording TASKS.md 2.1's deploy_count
// metric every time Reconcile actually performs a deploy cutover.
// Without one configured (the default), Reconcile behaves exactly as
// before, deploys just aren't measured.
func WithDeployRecorder(r DeployRecorder) Option {
	return func(ctrl *Controller) { ctrl.deployRecorder = r }
}

// New builds a Controller for serviceName.
func New(serviceName string, svcStore ServiceStore, runtime docker.Runtime, opts ...Option) *Controller {
	ctrl := &Controller{
		serviceName: serviceName,
		store:       svcStore,
		runtime:     runtime,
		httpClient:  http.DefaultClient,
		readyBudget: defaultReadyBudget,
	}
	for _, opt := range opts {
		opt(ctrl)
	}
	return ctrl
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "application/" + c.serviceName }

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	desired, err := c.store.GetDesiredService(ctx, c.serviceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		// Not a failure: a controller can be registered for a service
		// before its first deploy has ever written desired state.
		return unknownResult("NoDesiredState"), nil
	}
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("application/%s: get desired service: %w", c.serviceName, err)
	}

	// Defensive, not redundant: store.SaveDesiredService already
	// defaults an empty Strategy/zero Replicas before persisting, but
	// GetDesiredService's caller here is this package's own tests (and
	// any future direct store construction) constructing a
	// store.DesiredService{} literal by hand, which never goes through
	// SaveDesiredService at all. Reconcile must behave identically to
	// today's single-container-blue-green shape for every one of those,
	// the same "this field is never ambiguous regardless of who
	// constructs the struct" guarantee DesiredService's own doc comment
	// promises.
	replicas := desired.Replicas
	if replicas <= 0 {
		replicas = store.DefaultReplicas
	}
	strategy := desired.Strategy
	if strategy == "" {
		strategy = store.DefaultDeployStrategy
	}

	targets := make([]string, replicas)
	for i := range targets {
		targets[i] = replicaContainerName(c.serviceName, desired.Image, i)
	}

	switch strategy {
	case strategyBlueGreen:
		return c.reconcileBlueGreen(ctx, targets, desired)
	case strategyRecreate:
		return c.reconcileRecreate(ctx, targets, desired)
	case strategyRolling:
		// A real, named case, not a fallthrough: internal/spec's schema
		// has always accepted strategy: rolling, so a service can
		// already have this saved. Reported honestly rather than
		// silently reconciled as something else (this package's own doc
		// comment on the strategy constants explains why rolling is
		// separate, larger scope). Not an error: a permanently
		// unsupported strategy is the same "known, documented block, not
		// a transient failure that should retry-and-log-error forever"
		// shape internal/reconcile/database's credentials-blocked case
		// already establishes for an analogous known gap.
		return notReady("StrategyNotSupported", fmt.Errorf("deploy strategy %q is not yet implemented", strategy)), nil
	default:
		// Reachable only if something bypassed internal/spec's schema
		// validation (a hand-built DesiredService, or the schema's own
		// enum drifting from this list): an unrecognized strategy is
		// reported, never silently treated as blue-green, since guessing
		// wrong here means guessing wrong about whether it's safe to
		// remove an old container.
		return notReady("StrategyUnrecognized", fmt.Errorf("unrecognized deploy strategy %q", strategy)), nil
	}
}

// reconcileBlueGreen is today's original single-replica shape (this
// package's own doc comment: create new alongside old, wait for
// readiness, then remove every other container), generalized to
// targets' full replica set: every target is ensured running (and, if
// freshly started, ready) before any stale container, old-image or
// excess-replica alike, is removed. The safety property is unchanged
// from the single-replica case, just applied to a set instead of one
// container: the new set is fully proven healthy before the old set is
// torn down, so a bad deploy never has a moment where nothing healthy is
// serving.
func (c *Controller) reconcileBlueGreen(ctx context.Context, targets []string, desired *store.DesiredService) (reconcile.Result, error) {
	anyDeployed := false
	for _, target := range targets {
		deployed, err := c.ensureReplicaRunning(ctx, target, desired)
		if err != nil {
			return notReady(deployed.reason, err), fmt.Errorf("application/%s: %w", c.serviceName, err)
		}
		anyDeployed = anyDeployed || deployed.justDeployed
	}

	// Only reached once every target in this pass's replica set is
	// confirmed running (and, for any freshly started, ready): safe to
	// remove every other container for this service, old-image
	// containers and any excess replica from a replica-count decrease
	// alike.
	if err := c.removeStale(ctx, targets); err != nil {
		// The important fact, a healthy set is serving, is still true; a
		// stray old container is a real problem but a lesser one, so
		// Status stays True with a reason that says exactly what's
		// incomplete, and the error still surfaces (and this step
		// retries, safely, on the next reconcile).
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionTrue,
			Reason: "RunningStaleCleanupFailed", Message: err.Error(),
		}}}, fmt.Errorf("application/%s: cleanup stale containers: %w", c.serviceName, err)
	}

	return c.finishReconcile(ctx, anyDeployed)
}

// reconcileRecreate is the stop-everything-then-start-fresh strategy:
// simpler than blue-green (no old/new overlap to reason about, no
// container ever coexists with a different-image sibling), at the cost
// of a real downtime window between the stop and the new set becoming
// ready, exactly the tradeoff internal/spec's own strategy enum
// documents recreate as making.
//
// The no-op check at the top is load-bearing, not an optimization: a
// naive "always stop everything, then start everything" implementation
// would tear down and restart a perfectly healthy, already-converged
// replica set on every single resync tick (every reconcile.Engine pass,
// TASKS.md 1.3's own resyncInterval), which is a permanent recreate-loop
// bug, not a strategy. Reconcile must be idempotent (this codebase's own
// reconciler contract), so this only ever stops anything when the
// desired target set genuinely differs from what is currently running.
func (c *Controller) reconcileRecreate(ctx context.Context, targets []string, desired *store.DesiredService) (reconcile.Result, error) {
	allRunning := true
	for _, target := range targets {
		state, err := c.runtime.InspectByName(ctx, target)
		if err != nil {
			return notReady("InspectFailed", err), fmt.Errorf("application/%s: inspect %q: %w", c.serviceName, target, err)
		}
		if state == nil || !state.Running {
			allRunning = false
			break
		}
	}

	stale, err := c.staleContainers(ctx, targets)
	if err != nil {
		return notReady("InspectFailed", err), fmt.Errorf("application/%s: list stale containers: %w", c.serviceName, err)
	}

	if allRunning && len(stale) == 0 {
		return ready("AlreadyRunning"), nil
	}

	// Genuinely converging: stop and remove every existing container for
	// this service, old-image and current-image target alike. keep=nil
	// (not targets) is deliberate: a currently-running target is not
	// reused/left in place the way blue-green would leave it, recreate's
	// whole point is that nothing old ever coexists with anything new,
	// so the "already-running, already-current" targets staleContainers
	// excluded above still need to go before ensureReplicaRunning
	// recreates them fresh below.
	if err := c.removeStale(ctx, nil); err != nil {
		return notReady("CleanupFailed", err), fmt.Errorf("application/%s: stop existing containers: %w", c.serviceName, err)
	}

	for _, target := range targets {
		deployed, err := c.ensureReplicaRunning(ctx, target, desired)
		if err != nil {
			return notReady(deployed.reason, err), fmt.Errorf("application/%s: %w", c.serviceName, err)
		}
	}

	return c.finishReconcile(ctx, true)
}

// finishReconcile records the deploy metric (if a real cutover happened
// this pass) and returns the final Ready condition, the shared tail both
// reconcileBlueGreen and reconcileRecreate end with.
func (c *Controller) finishReconcile(ctx context.Context, justDeployed bool) (reconcile.Result, error) {
	if !justDeployed {
		return ready("AlreadyRunning"), nil
	}
	// Best-effort, secondary to the deploy itself: every target is
	// already confirmed running (and ready, and stale ones already
	// cleaned up) by this point, so a metrics-store hiccup is surfaced
	// through the returned error (which reconcile.Engine logs) without
	// downgrading Status away from True, the same "the important fact is
	// still true" shape this package's cleanup-failure paths already
	// establish.
	if c.deployRecorder != nil {
		if err := c.deployRecorder.RecordDeploy(ctx, c.serviceName, time.Now()); err != nil {
			return reconcile.Result{Conditions: []reconcile.Condition{{
				Type: "Ready", Status: reconcile.ConditionTrue,
				Reason: "DeployedMetricRecordFailed", Message: err.Error(),
			}}}, fmt.Errorf("application/%s: record deploy metric: %w", c.serviceName, err)
		}
	}
	return ready("Deployed"), nil
}

// replicaOutcome carries ensureReplicaRunning's own condition reason
// alongside its error, so a caller can build a notReady result with the
// specific step that failed (CreateFailed/StartFailed/ReadinessFailed/
// etc.) without ensureReplicaRunning needing to know how its caller
// constructs a reconcile.Result.
type replicaOutcome struct {
	justDeployed bool
	reason       string
}

// ensureReplicaRunning is the original single-container "does the right
// container exist, is it running, is it ready" sequence, unchanged in
// behavior, extracted so both reconcileBlueGreen and reconcileRecreate
// share exactly one implementation of it rather than two copies that
// could drift.
func (c *Controller) ensureReplicaRunning(ctx context.Context, target string, desired *store.DesiredService) (replicaOutcome, error) {
	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return replicaOutcome{reason: "InspectFailed"}, fmt.Errorf("inspect %q: %w", target, err)
	}

	justDeployed := false
	switch {
	case state == nil:
		if err := c.createAndStart(ctx, target, desired); err != nil {
			return replicaOutcome{reason: "CreateFailed"}, err
		}
		justDeployed = true
	case !state.Running:
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return replicaOutcome{reason: "StartFailed"}, fmt.Errorf("restart %q: %w", target, err)
		}
		justDeployed = true
	}

	if !justDeployed {
		return replicaOutcome{}, nil
	}

	// Re-inspect: Docker only reports port bindings once a container is
	// actually running, not at create time.
	state, err = c.runtime.InspectByName(ctx, target)
	if err != nil {
		return replicaOutcome{reason: "InspectFailed"}, fmt.Errorf("re-inspect %q after start: %w", target, err)
	}
	if state == nil {
		return replicaOutcome{reason: "VanishedAfterStart"}, fmt.Errorf("%q not found immediately after starting it", target)
	}
	if err := c.waitReady(ctx, state, desired); err != nil {
		return replicaOutcome{reason: "ReadinessFailed"}, err
	}
	return replicaOutcome{justDeployed: true}, nil
}

func (c *Controller) createAndStart(ctx context.Context, name string, desired *store.DesiredService) error {
	env, err := c.resolveEnv(ctx, desired)
	if err != nil {
		return fmt.Errorf("resolve env: %w", err)
	}

	spec := toContainerSpec(name, desired)
	spec.Env = env

	id, err := c.runtime.Create(ctx, spec)
	if err != nil {
		return fmt.Errorf("create %q: %w", name, err)
	}
	if err := c.runtime.Start(ctx, id); err != nil {
		return fmt.Errorf("start %q after create: %w", name, err)
	}
	return nil
}

// resolveEnv merges desired.Env's literal values with each of
// desired.SecretEnv's names, decrypted via the configured
// SecretResolver. This is the only place in this controller a secret's
// plaintext exists: it lives in this function's local env map only for
// the moment between here and docker.Runtime.Create in createAndStart,
// and is never written to the store, logged, or returned in a
// reconcile.Result.
//
// An unset optional ({ secret: true, required: false }) secret is
// silently omitted from the container's environment, matching
// spec.EnvVar.Required's documented meaning: internal/deploy.Pipeline
// already rejects a deploy outright if a required secret has no value,
// so by the time this runs, an unset secret still in desired.SecretEnv
// can only be an optional one.
//
// Precedence when the same key appears in both desired.Env and
// desired.SecretEnv: the secret-resolved value always wins. The literal
// values are copied into env first, then each resolved secret is
// written into env afterward, unconditionally, so a colliding key ends
// up holding the secret's value regardless of desired.Env's (map,
// unordered) iteration order. This is deterministic by construction,
// not an accident of that iteration order, because desired.SecretEnv is
// a slice applied strictly after the literal copy completes, one
// assignment per declared key. The precedence itself is a deliberate
// default: if a service declares both, the secret is presumably the
// more deliberate, more authoritative source, and letting it win
// prevents a leftover plain-text duplicate from silently shadowing a
// real secret.
func (c *Controller) resolveEnv(ctx context.Context, desired *store.DesiredService) (map[string]string, error) {
	if len(desired.SecretEnv) == 0 {
		return desired.Env, nil
	}
	if c.secretResolver == nil {
		return nil, fmt.Errorf("service declares %d secret-backed env var(s) but no secret resolver is configured", len(desired.SecretEnv))
	}

	env := make(map[string]string, len(desired.Env)+len(desired.SecretEnv))
	for k, v := range desired.Env {
		env[k] = v
	}
	for _, key := range desired.SecretEnv {
		exists, err := c.secretResolver.Exists(ctx, c.serviceName, key)
		if err != nil {
			return nil, fmt.Errorf("check secret %q: %w", key, err)
		}
		if !exists {
			continue
		}
		value, err := c.secretResolver.Resolve(ctx, c.serviceName, key)
		if err != nil {
			return nil, fmt.Errorf("resolve secret %q: %w", key, err)
		}
		env[key] = value
	}
	return env, nil
}

// waitReady gates a freshly (re)started container on its readiness
// probe, if the service declares one and has a port to probe at all. A
// service with no port (a worker with nothing listening) or no
// readiness config is considered ready the moment it's running: there's
// nothing more to check.
func (c *Controller) waitReady(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService) error {
	if desired.Health == nil || desired.Health.Readiness == nil {
		return nil
	}
	if desired.Port == 0 {
		return nil
	}

	addr, err := primaryAddr(state)
	if err != nil {
		return fmt.Errorf("readiness probe: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, c.readyBudget)
	defer cancel()

	cfg := probe.Config{
		Path:     desired.Health.Readiness.Path,
		Interval: desired.Health.Readiness.Interval,
		Timeout:  desired.Health.Readiness.Timeout,
	}
	if err := probe.WaitReady(probeCtx, c.httpClient, addr, cfg); err != nil {
		return fmt.Errorf("readiness probe: %w", err)
	}
	return nil
}

// removeStale finds every container for this service other than keep
// and stops and removes each. Level-triggered by construction: it
// re-derives "what's stale" from Docker's current state every call,
// never from a remembered list of past container names.
// removeStale finds every container for this service other than one of
// keep and stops and removes each. Level-triggered by construction: it
// re-derives "what's stale" from Docker's current state every call,
// never from a remembered list of past container names. keep is
// typically the full current replica target set (reconcileBlueGreen);
// an empty keep (reconcileRecreate's own use of the lower-level
// staleContainers+removeContainers pair below) is a valid way to find
// literally everything for this service.
func (c *Controller) removeStale(ctx context.Context, keep []string) error {
	stale, err := c.staleContainers(ctx, keep)
	if err != nil {
		return err
	}
	return c.removeContainers(ctx, stale)
}

// staleContainers lists every container for this service other than one
// of keep, applying the same ownsContainer boundary check removeStale
// itself relied on before this was split out: ListByPrefix is a bare
// string-prefix match, not a service boundary (internal/spec's
// service-name validation permits hyphens freely, so a service named
// "web" also prefix-matches containers belonging to a differently-named
// service like "web-worker"), so only a container whose name is exactly
// one of this service's own replicaContainerName shapes is ever
// considered, regardless of whether it's in keep.
func (c *Controller) staleContainers(ctx context.Context, keep []string) ([]docker.ContainerState, error) {
	all, err := c.runtime.ListByPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return nil, fmt.Errorf("list containers for %s: %w", c.serviceName, err)
	}

	keepSet := make(map[string]bool, len(keep))
	for _, name := range keep {
		keepSet[name] = true
	}

	var stale []docker.ContainerState
	for _, cs := range all {
		if keepSet[cs.Name] || !ownsContainer(c.serviceName, cs.Name) {
			continue
		}
		stale = append(stale, cs)
	}
	return stale, nil
}

// removeContainers stops and removes every container in cs, continuing
// past an individual failure rather than aborting the batch, and
// returning the first error encountered (if any) once every container
// has been attempted: one stuck container must not block cleanup of the
// rest, the same "one broken resource must not block others" principle
// this codebase's own dynamicSource doc comment already applies to a
// reconcile pass across services.
func (c *Controller) removeContainers(ctx context.Context, cs []docker.ContainerState) error {
	var firstErr error
	for _, container := range cs {
		if err := c.runtime.Stop(ctx, container.ID, 10*time.Second); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("stop stale container %s: %w", container.Name, err)
		}
		if err := c.runtime.Remove(ctx, container.ID, true); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("remove stale container %s: %w", container.Name, err)
		}
	}
	return firstErr
}

// primaryAddr picks the address a readiness probe should hit: the sole
// port binding a service with exactly one declared port produces.
func primaryAddr(state *docker.ContainerState) (string, error) {
	if len(state.Ports) == 0 {
		return "", fmt.Errorf("container %s has no published ports to probe", state.Name)
	}
	return "127.0.0.1:" + strconv.Itoa(state.Ports[0].HostPort), nil
}

// hashLen is the number of hex characters ContainerName keeps from the
// image's hash. Shared with ownsContainer so the two stay in lockstep:
// the length that identifies "this is one of my containers" has to
// exactly match the length ContainerName actually produces.
const hashLen = 8

// ContainerName derives a stable, unique-per-image name so the
// level-triggered check "does the right container exist" needs no
// memory beyond what Docker itself can answer. Two deploys of the same
// image produce the same name (a genuine no-op redeploy correctly finds
// nothing to do); two different images always produce different names
// (so both can exist side by side during a cutover). Exported so the
// ingress controller (internal/reconcile/ingress, TASKS.md 1.6) can
// derive the exact same name to find a service's currently active
// container, without reimplementing this hash logic a second time and
// risking the two drifting apart.
func ContainerName(serviceName, image string) string {
	sum := sha256.Sum256([]byte(image))
	return serviceName + "-" + hex.EncodeToString(sum[:])[:hashLen]
}

// replicaContainerName is ContainerName generalized to replica index.
// index 0 returns exactly what ContainerName returns, byte for byte: a
// single-replica service (every service before this field existed, and
// every service that never sets replicas going forward) must get the
// identical name it always has, both so an upgrade never orphans an
// already-running container under a new name it doesn't recognize, and
// so internal/reconcile/ingress's own call to the exported ContainerName
// (its own doc comment: "derive the exact same name to find a service's
// currently active container") keeps finding the right one without that
// package needing to know replicas exist at all. Replica index N>0 gets
// an additional "-rN" suffix; ownsContainer below recognizes both forms.
func replicaContainerName(serviceName, image string, index int) string {
	base := ContainerName(serviceName, image)
	if index == 0 {
		return base
	}
	return base + "-r" + strconv.Itoa(index)
}

// ownsContainer reports whether name is one this service's
// replicaContainerName could have produced for some image and replica
// index: serviceName + "-" + an hashLen-character hex image hash,
// optionally followed by "-rN" for replica index N>0. A plain prefix
// match isn't enough: service names may contain hyphens (internal/spec's
// nameLike permits them), so a service named "web" string-prefix-matches
// containers belonging to an entirely different, differently-owned
// service like "web-worker". Without this exact check, stale-container
// cleanup would treat another service's live container as one of "web"'s
// own stale leftovers and remove it.
func ownsContainer(serviceName, name string) bool {
	suffix, ok := strings.CutPrefix(name, serviceName+"-")
	if !ok || len(suffix) < hashLen {
		return false
	}
	hash, rest := suffix[:hashLen], suffix[hashLen:]
	for _, r := range hash {
		if !isHexDigit(r) {
			return false
		}
	}
	if rest == "" {
		return true // replica 0: no "-rN" suffix at all
	}
	replicaIndex, ok := strings.CutPrefix(rest, "-r")
	if !ok || replicaIndex == "" {
		return false
	}
	for _, r := range replicaIndex {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}

func toContainerSpec(name string, desired *store.DesiredService) docker.ContainerSpec {
	spec := docker.ContainerSpec{
		Name:  name,
		Image: desired.Image,
		Env:   desired.Env,
	}
	if desired.Port != 0 {
		spec.Ports = []docker.PortBinding{{ContainerPort: desired.Port}}
	}
	if desired.Resources != nil {
		spec.Resources = &docker.Resources{
			MemoryBytes: desired.Resources.MemoryBytes,
			NanoCPUs:    desired.Resources.NanoCPUs,
		}
	}
	return spec
}

func ready(reason string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionTrue, Reason: reason,
	}}}
}

func notReady(reason string, err error) reconcile.Result {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionFalse, Reason: reason, Message: msg,
	}}}
}

func unknownResult(reason string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionUnknown, Reason: reason,
	}}}
}
