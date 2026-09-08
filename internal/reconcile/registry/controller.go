// Package registry implements the reconcile.Controller that converges a
// single, platform-wide desired state (store.RegistrySettings plus a
// generated password in internal/secrets) to a running or absent
// registry:2 container: Levelrail's own built-in image registry, so a
// multi-node deployment gets a BuildKit cache/distribution backend
// (internal/build's WithCacheRegistry) without an operator first signing
// up for an external one.
//
// Shaped after internal/reconcile/cloudflaretunnel's Controller: a
// single, unnamed, platform-wide container, created and started through
// docker.Runtime like every other container this codebase manages, never
// shelled out to. Unlike cloudflared, this container is stateful (a named
// volume for image blobs, mirroring internal/reconcile/database's own
// volume handling) and needs its own login credential: the registry
// image's htpasswd auth is written from an env-injected bcrypt hash at
// container start (see htpasswdBootScript), rather than fronting an
// unauthenticated registry with the ingress layer alone, so the published
// port is never usable without the generated credential even if reached
// directly, bypassing the embedded Caddy ingress.
package registry

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// image is the official Docker Distribution registry image, run as a
// container like every other process this codebase manages.
const image = "registry:2"

// dataPath is where the registry image itself stores image blobs, its
// own documented default.
const dataPath = "/var/lib/registry"

// containerPort is the port the registry image listens on inside the
// container, its own documented default.
const containerPort = 5000

// HostPort is the fixed host port this controller publishes containerPort
// to. Fixed, not Docker-assigned (HostPort: 0), because both the ingress
// controller's reverse-proxy dial (WithRegistryDial) and the build-cache
// wiring (cmd/levelrail's buildCacheOptions) need a stable target that
// doesn't change across reconciles, the same reasoning
// cloudflaretunnel's stateless container gets away without: that
// container is never dialed by anything else in this codebase.
const HostPort = 5540

const defaultStopTimeout = 10 * time.Second

// defaultPrefix mirrors cloudflaretunnel.defaultPrefix's own fallback:
// used only when no brand-derived prefix was configured.
const defaultPrefix = "platform"

// ControllerName identifies this controller in reconcile_status
// (store.GetConditions), the fixed name GET /api/v1/settings/registry
// reads status from. A fixed string, not a per-resource name, because
// this resource is a platform-wide singleton.
const ControllerName = "builtin-registry"

// htpasswdUser/htpasswdHashEnv are the env var names htpasswdBootScript
// reads to materialize /auth/htpasswd before handing off to the real
// registry entrypoint. Distinct from REGISTRY_AUTH_HTPASSWD_* (the
// registry image's own env vars, set directly in Env below): these two
// only exist to get the username/hash into the container long enough for
// the boot script to write them to a file.
const (
	htpasswdUserEnv = "LEVELRAIL_REGISTRY_HTPASSWD_USER" //nolint:gosec // env var name, not a credential value
	htpasswdHashEnv = "LEVELRAIL_REGISTRY_HTPASSWD_HASH" //nolint:gosec // env var name, not a credential value
)

// htpasswdBootScript writes /auth/htpasswd from the injected env vars
// above, then execs the image's real entrypoint. The registry image
// itself only ever reads an htpasswd *path* (REGISTRY_AUTH_HTPASSWD_PATH,
// set in Env below); it has no way to accept htpasswd content directly,
// so this is what actually produces the file that path points at.
const htpasswdBootScript = `set -e
mkdir -p /auth
printf '%s:%s\n' "$` + htpasswdUserEnv + `" "$` + htpasswdHashEnv + `" > /auth/htpasswd
exec /entrypoint.sh /etc/docker/registry/config.yml
`

// Store is the narrow surface this controller needs from internal/store,
// so tests can fake it without a real database. *store.DB satisfies this.
type Store interface {
	GetRegistrySettings(ctx context.Context) (store.RegistrySettings, error)
}

// CredentialResolver is the narrow surface this controller needs from
// internal/secrets.Manager, structurally identical to
// cloudflaretunnel.TokenResolver but named separately since the two
// resolve unrelated credentials under different serviceName namespaces.
type CredentialResolver interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Controller converges the desired built-in registry config to a running
// or absent registry:2 container.
type Controller struct {
	store   Store
	creds   CredentialResolver
	runtime docker.Runtime
	prefix  string
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithContainerPrefix sets the prefix ContainerName/VolumeName derive
// names from (typically brand.Brand.ShortName), the same "prefix, not a
// hardcoded name" shape cloudflaretunnel.WithContainerPrefix establishes.
// Without one configured, names fall back to defaultPrefix.
func WithContainerPrefix(prefix string) Option {
	return func(c *Controller) { c.prefix = prefix }
}

// New builds a Controller. creds may be nil (no secrets master key
// configured): Reconcile then treats an enabled registry as if it has no
// credentials yet, the same "known, permanent, documented block" shape
// database.Controller's credentialsBlockedResult already establishes.
func New(st Store, creds CredentialResolver, runtime docker.Runtime, opts ...Option) *Controller {
	c := &Controller{store: st, creds: creds, runtime: runtime}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return ControllerName }

// ContainerName derives the registry container's deterministic name from
// prefix, mirroring cloudflaretunnel.ContainerName's exact shape.
func ContainerName(prefix string) string {
	if prefix == "" {
		prefix = defaultPrefix
	}
	return prefix + "-registry"
}

// VolumeName derives the registry's data volume name from prefix, the
// same "prefix + fixed suffix" shape ContainerName uses.
func VolumeName(prefix string) string {
	if prefix == "" {
		prefix = defaultPrefix
	}
	return prefix + "-registry-data"
}

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	settings, err := c.store.GetRegistrySettings(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("registry: get settings: %w", err)
	}

	target := ContainerName(c.prefix)

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return notReady("InspectFailed", err), fmt.Errorf("registry: inspect %q: %w", target, err)
	}

	hasCreds, err := c.credentialsExist(ctx)
	if err != nil {
		return notReady("CredentialCheckFailed", err), fmt.Errorf("registry: check credentials: %w", err)
	}

	if !settings.Enabled || !hasCreds {
		if state == nil {
			return absentResult(settings, hasCreds), nil
		}
		if err := c.remove(ctx, state); err != nil {
			return notReady("RemoveFailed", err), fmt.Errorf("registry: remove %q: %w", target, err)
		}
		return absentResult(settings, hasCreds), nil
	}

	password, err := c.creds.Resolve(ctx, store.RegistrySettingsSecretsKey(), store.RegistryPasswordEnvKey)
	if err != nil {
		return notReady("CredentialResolveFailed", err), fmt.Errorf("registry: resolve password: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return notReady("HashFailed", err), fmt.Errorf("registry: hash password: %w", err)
	}

	volName := VolumeName(c.prefix)
	if err := c.runtime.EnsureVolume(ctx, volName); err != nil {
		return notReady("VolumeFailed", err), fmt.Errorf("registry: ensure volume %q: %w", volName, err)
	}

	spec := docker.ContainerSpec{
		Name:  target,
		Image: image,
		Env: map[string]string{ //nolint:gosec // a bcrypt hash and a generated password, not a hardcoded credential
			"REGISTRY_STORAGE_DELETE_ENABLED": "true",
			"REGISTRY_AUTH":                   "htpasswd",
			"REGISTRY_AUTH_HTPASSWD_REALM":    "Levelrail Registry",
			"REGISTRY_AUTH_HTPASSWD_PATH":     "/auth/htpasswd",
			htpasswdUserEnv:                   settings.Username,
			htpasswdHashEnv:                   string(hash),
		},
		Entrypoint: []string{"sh", "-c"},
		Command:    []string{htpasswdBootScript},
		Volumes:    []docker.VolumeMount{{Name: volName, ContainerPath: dataPath}},
		Ports:      []docker.PortBinding{{ContainerPort: containerPort, HostPort: HostPort}},
	}

	justDeployed := false
	switch {
	case state == nil:
		if err := c.createAndStart(ctx, spec); err != nil {
			return notReady("CreateFailed", err), fmt.Errorf("registry: %w", err)
		}
		justDeployed = true

	case state.Image != image:
		if err := c.replace(ctx, state, spec); err != nil {
			return notReady("ReplaceFailed", err), fmt.Errorf("registry: %w", err)
		}
		justDeployed = true

	case !state.Running:
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return notReady("StartFailed", err), fmt.Errorf("registry: restart %q: %w", target, err)
		}
		justDeployed = true
	}

	if justDeployed {
		state, err = c.runtime.InspectByName(ctx, target)
		if err != nil {
			return notReady("InspectFailed", err), fmt.Errorf("registry: re-inspect %q after start: %w", target, err)
		}
		if state == nil || !state.Running {
			return notReady("VanishedAfterStart", nil), fmt.Errorf("registry: %q not running immediately after starting it", target)
		}
		return ready("Running"), nil
	}

	return ready("Running"), nil
}

// credentialsExist reports whether a password is available to resolve,
// treating a nil CredentialResolver (no secrets master key configured)
// the same as "no password set": the caller cannot tell those two states
// apart from Exists' bool alone, and doesn't need to, since both mean the
// same thing here, no container can be started.
func (c *Controller) credentialsExist(ctx context.Context) (bool, error) {
	if c.creds == nil {
		return false, nil
	}
	return c.creds.Exists(ctx, store.RegistrySettingsSecretsKey(), store.RegistryPasswordEnvKey)
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
// Sequential, mirroring database.Controller's replaceContainer, so the
// data volume is never mounted by two containers at once.
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
// configured, ConditionUnknown: not a failure) from an enabled registry
// this control plane cannot start yet for lack of credentials
// (ConditionFalse: a real, actionable gap), the same split
// cloudflaretunnel.absentResult already establishes for its own resource.
func absentResult(settings store.RegistrySettings, hasCreds bool) reconcile.Result {
	if settings.Enabled && !hasCreds {
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionFalse, Reason: "CredentialsNotConfigured",
			Message: "the built-in registry is enabled but no credentials have been generated",
		}}}
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionUnknown, Reason: "Disabled",
	}}}
}
