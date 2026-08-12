// Package application implements CLAUDE.md 4.9 and TASKS.md 1.3's
// application controller: the reconcile.Controller that converges a
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
// deliberately not this controller's job: CLAUDE.md 4.2 makes "a
// reconcile loop per resource type" the pattern, and ingress is its own
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

const defaultReadyBudget = 60 * time.Second

// Controller converges one named service's desired state (read fresh
// from ServiceStore on every Reconcile, never cached) to a running
// container.
type Controller struct {
	serviceName string
	store       ServiceStore
	runtime     docker.Runtime
	httpClient  *http.Client
	readyBudget time.Duration
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

	target := ContainerName(c.serviceName, desired.Image)

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return notReady("InspectFailed", err), fmt.Errorf("application/%s: inspect %q: %w", c.serviceName, target, err)
	}

	justDeployed := false
	switch {
	case state == nil:
		if err := c.createAndStart(ctx, target, desired); err != nil {
			return notReady("CreateFailed", err), fmt.Errorf("application/%s: %w", c.serviceName, err)
		}
		justDeployed = true
	case !state.Running:
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return notReady("StartFailed", err), fmt.Errorf("application/%s: restart %q: %w", c.serviceName, target, err)
		}
		justDeployed = true
	}

	if justDeployed {
		// Re-inspect: Docker only reports port bindings once a container
		// is actually running, not at create time.
		state, err = c.runtime.InspectByName(ctx, target)
		if err != nil {
			return notReady("InspectFailed", err), fmt.Errorf("application/%s: re-inspect %q after start: %w", c.serviceName, target, err)
		}
		if state == nil {
			return notReady("VanishedAfterStart", nil), fmt.Errorf("application/%s: %q not found immediately after starting it", c.serviceName, target)
		}

		if err := c.waitReady(ctx, state, desired); err != nil {
			return notReady("ReadinessFailed", err), fmt.Errorf("application/%s: %w", c.serviceName, err)
		}
	}

	// Only reached once the current desired container is confirmed
	// running (and, if just deployed, ready): safe to remove every
	// other container for this service.
	if err := c.removeStale(ctx, target); err != nil {
		// The important fact, a healthy container is serving, is still
		// true; a stray old container is a real problem but a lesser
		// one, so Status stays True with a reason that says exactly
		// what's incomplete, and the error still surfaces (and this
		// step retries, safely, on the next reconcile).
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionTrue,
			Reason: "RunningStaleCleanupFailed", Message: err.Error(),
		}}}, fmt.Errorf("application/%s: cleanup stale containers: %w", c.serviceName, err)
	}

	if justDeployed {
		return ready("Deployed"), nil
	}
	return ready("AlreadyRunning"), nil
}

func (c *Controller) createAndStart(ctx context.Context, name string, desired *store.DesiredService) error {
	id, err := c.runtime.Create(ctx, toContainerSpec(name, desired))
	if err != nil {
		return fmt.Errorf("create %q: %w", name, err)
	}
	if err := c.runtime.Start(ctx, id); err != nil {
		return fmt.Errorf("start %q after create: %w", name, err)
	}
	return nil
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
func (c *Controller) removeStale(ctx context.Context, keep string) error {
	all, err := c.runtime.ListByPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return fmt.Errorf("list containers for %s: %w", c.serviceName, err)
	}

	var firstErr error
	for _, cs := range all {
		if cs.Name == keep {
			continue
		}
		if !ownsContainer(c.serviceName, cs.Name) {
			// ListByPrefix is a bare string-prefix match, not a service
			// boundary: internal/spec's service-name validation permits
			// hyphens freely, so a service named "web" also
			// prefix-matches containers belonging to a differently-named
			// service like "web-worker". Only ever touch a container
			// whose name is exactly this service's own containerName
			// shape; anything else found by the prefix scan belongs to
			// someone else and must be left alone.
			continue
		}
		if err := c.runtime.Stop(ctx, cs.ID, 10*time.Second); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("stop stale container %s: %w", cs.Name, err)
		}
		if err := c.runtime.Remove(ctx, cs.ID, true); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("remove stale container %s: %w", cs.Name, err)
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

// ownsContainer reports whether name is exactly one this service's
// containerName produces: serviceName + "-" + an hashLen-character hex
// image hash, nothing more and nothing less. A plain prefix match isn't
// enough: service names may contain hyphens (internal/spec's nameLike
// permits them), so a service named "web" string-prefix-matches
// containers belonging to an entirely different, differently-owned
// service like "web-worker". Without this exact check, removeStale would
// treat another service's live container as one of "web"'s own stale
// leftovers and remove it.
func ownsContainer(serviceName, name string) bool {
	suffix, ok := strings.CutPrefix(name, serviceName+"-")
	if !ok || len(suffix) != hashLen {
		return false
	}
	for _, r := range suffix {
		if !isHexDigit(r) {
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
