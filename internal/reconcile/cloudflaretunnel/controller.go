// Package cloudflaretunnel implements the reconcile.Controller that
// converges a single, platform-wide desired state (store.
// CloudflareTunnelSettings plus a token in internal/secrets) to a
// running or absent cloudflared container, letting an operator expose
// this control plane (or, via their own Cloudflare-side routing, one of
// its apps) to the internet without opening an inbound port.
//
// Shaped after internal/reconcile/database's Controller: a single
// container, created and started through docker.Runtime like every
// other container this codebase manages, never shelled out to. Unlike a
// database, this resource has no per-name identity (there is exactly
// one tunnel connection per control plane, the same "id = 1" singleton
// shape store.GitHubAppConnection already establishes) and no volume:
// cloudflared is stateless, so a config change (enable/disable, a
// rotated token) is handled by create-or-remove, never an in-place
// replace.
package cloudflaretunnel

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// image is the official Cloudflare Tunnel daemon image, run as a
// container like every other process this codebase manages: Levelrail
// never shells out to a cloudflared binary itself.
const image = "cloudflare/cloudflared:latest"

const defaultStopTimeout = 10 * time.Second

// defaultPrefix mirrors application.defaultNetworkPrefix's own fallback:
// used only when no brand-derived prefix was configured.
const defaultPrefix = "platform"

// ControllerName identifies this controller in reconcile_status
// (store.GetConditions), the fixed name GET
// /api/v1/settings/cloudflare-tunnel reads status from. A fixed string,
// not a per-resource name like databaseControllerName produces, because
// this resource is a platform-wide singleton.
const ControllerName = "cloudflare-tunnel"

// tunnelEnvKey is the env var name cloudflared's own image reads a
// tunnel token from when no --token argument is passed on its command
// line. Env, not a command-line argument, deliberately: a command-line
// --token would appear in this container's inspected Config.Cmd
// (visible to anything with Docker socket access), where an env var
// (still visible via inspect, but not additionally duplicated into
// process listings/shell history the way a CLI arg pattern invites)
// matches how every other credential this codebase injects into a
// container (POSTGRES_PASSWORD, MYSQL_ROOT_PASSWORD) already flows.
const tunnelEnvKey = "TUNNEL_TOKEN"

// Store is the narrow surface this controller needs from internal/store,
// so tests can fake it without a real database. *store.DB satisfies
// this.
type Store interface {
	GetCloudflareTunnelSettings(ctx context.Context) (store.CloudflareTunnelSettings, error)
}

// TokenResolver is the narrow surface this controller needs from
// internal/secrets.Manager, mirroring application.SecretResolver's exact
// shape. *secrets.Manager satisfies this structurally.
type TokenResolver interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Controller converges the desired Cloudflare Tunnel config to a
// running or absent cloudflared container.
type Controller struct {
	store   Store
	tokens  TokenResolver
	runtime docker.Runtime
	prefix  string
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithContainerPrefix sets the prefix ContainerName derives the
// cloudflared container's name from (typically brand.Brand.ShortName),
// the same "prefix, not a hardcoded name" shape
// application.WithNetworkPrefix establishes. Without one configured,
// ContainerName falls back to defaultPrefix.
func WithContainerPrefix(prefix string) Option {
	return func(c *Controller) { c.prefix = prefix }
}

// New builds a Controller. tokens may be nil (no secrets master key
// configured): Reconcile then treats every enabled tunnel as if it has
// no token, the same "known, permanent, documented block" shape
// database.Controller's credentialsBlockedResult already establishes
// for Postgres/MySQL/MongoDB without WithPostgresCredentials/etc.
func New(st Store, tokens TokenResolver, runtime docker.Runtime, opts ...Option) *Controller {
	c := &Controller{store: st, tokens: tokens, runtime: runtime}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return ControllerName }

// ContainerName derives the cloudflared container's deterministic name
// from prefix (typically brand.Brand.ShortName), mirroring
// application.NetworkName's own "prefix + fixed suffix" shape. This
// container is a platform-wide singleton, not per-resource, so unlike
// application.ContainerName/database's containerName it needs no
// resource name folded in.
func ContainerName(prefix string) string {
	if prefix == "" {
		prefix = defaultPrefix
	}
	return prefix + "-cloudflared"
}

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	settings, err := c.store.GetCloudflareTunnelSettings(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("cloudflare-tunnel: get settings: %w", err)
	}

	target := ContainerName(c.prefix)

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return notReady("InspectFailed", err), fmt.Errorf("cloudflare-tunnel: inspect %q: %w", target, err)
	}

	hasToken, err := c.tokenExists(ctx)
	if err != nil {
		return notReady("TokenCheckFailed", err), fmt.Errorf("cloudflare-tunnel: check token: %w", err)
	}

	if !settings.Enabled || !hasToken {
		if state == nil {
			return absentResult(settings, hasToken), nil
		}
		if err := c.remove(ctx, state); err != nil {
			return notReady("RemoveFailed", err), fmt.Errorf("cloudflare-tunnel: remove %q: %w", target, err)
		}
		return absentResult(settings, hasToken), nil
	}

	token, err := c.tokens.Resolve(ctx, store.CloudflareTunnelSecretsKey(), store.CloudflareTunnelTokenEnvKey)
	if err != nil {
		return notReady("TokenResolveFailed", err), fmt.Errorf("cloudflare-tunnel: resolve token: %w", err)
	}

	spec := docker.ContainerSpec{
		Name:    target,
		Image:   image,
		Env:     map[string]string{tunnelEnvKey: token},
		Command: []string{"tunnel", "--no-autoupdate", "run"},
	}

	justDeployed := false
	switch {
	case state == nil:
		if err := c.createAndStart(ctx, spec); err != nil {
			return notReady("CreateFailed", err), fmt.Errorf("cloudflare-tunnel: %w", err)
		}
		justDeployed = true

	case state.Image != image:
		if err := c.replace(ctx, state, spec); err != nil {
			return notReady("ReplaceFailed", err), fmt.Errorf("cloudflare-tunnel: %w", err)
		}
		justDeployed = true

	case !state.Running:
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return notReady("StartFailed", err), fmt.Errorf("cloudflare-tunnel: restart %q: %w", target, err)
		}
		justDeployed = true
	}

	if justDeployed {
		state, err = c.runtime.InspectByName(ctx, target)
		if err != nil {
			return notReady("InspectFailed", err), fmt.Errorf("cloudflare-tunnel: re-inspect %q after start: %w", target, err)
		}
		if state == nil || !state.Running {
			return notReady("VanishedAfterStart", nil), fmt.Errorf("cloudflare-tunnel: %q not running immediately after starting it", target)
		}
		return ready("Connected"), nil
	}

	return ready("Connected"), nil
}

// tokenExists reports whether a token is available to resolve, treating
// a nil TokenResolver (no secrets master key configured) the same as
// "no token set": the caller cannot tell those two states apart from
// Exists' bool alone, and doesn't need to, since both mean the same
// thing here, no container can be started.
func (c *Controller) tokenExists(ctx context.Context) (bool, error) {
	if c.tokens == nil {
		return false, nil
	}
	return c.tokens.Exists(ctx, store.CloudflareTunnelSecretsKey(), store.CloudflareTunnelTokenEnvKey)
}

func (c *Controller) createAndStart(ctx context.Context, spec docker.ContainerSpec) error {
	id, err := c.runtime.Create(ctx, spec)
	if err != nil {
		return fmt.Errorf("create %q: %w", spec.Name, err)
	}
	if err := c.runtime.Start(ctx, id); err != nil {
		return fmt.Errorf("start %q after create: %w", spec.Name, err)
	}
	return nil
}

// replace stops and removes an existing container whose image no longer
// matches desired state, then creates and starts the replacement.
// Sequential, mirroring database.Controller's replaceContainer, though
// this container is stateless (no volume) so the ordering here is about
// avoiding two cloudflared instances racing to register the same tunnel
// connection, not data corruption.
func (c *Controller) replace(ctx context.Context, old *docker.ContainerState, spec docker.ContainerSpec) error {
	if err := c.remove(ctx, old); err != nil {
		return err
	}
	return c.createAndStart(ctx, spec)
}

func (c *Controller) remove(ctx context.Context, old *docker.ContainerState) error {
	if old.Running {
		if err := c.runtime.Stop(ctx, old.ID, defaultStopTimeout); err != nil {
			return fmt.Errorf("stop %q: %w", old.Name, err)
		}
	}
	if err := c.runtime.Remove(ctx, old.ID, true); err != nil {
		return fmt.Errorf("remove %q: %w", old.Name, err)
	}
	return nil
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

// absentResult reports the condition for "no container should exist",
// distinguishing the operator's own choice (disabled or never
// configured, ConditionUnknown: not a failure) from an enabled tunnel
// this control plane cannot start yet for lack of a token
// (ConditionFalse: a real, actionable gap, the same "credentials
// missing is a loud False, not a silent Unknown" reasoning
// database.Controller's credentialsBlockedResult already establishes).
func absentResult(settings store.CloudflareTunnelSettings, hasToken bool) reconcile.Result {
	if settings.Enabled && !hasToken {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionFalse, Reason: "TokenNotConfigured",
			Message: "cloudflare tunnel is enabled but no token has been set",
		}}}
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "Disabled",
	}}}
}
