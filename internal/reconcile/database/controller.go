// Package database implements TASKS.md 1.8's managed database controller:
// the reconcile.Controller that converges a store-backed
// store.DesiredDatabase to a running, volume-backed container, the same
// architectural pattern internal/reconcile/application already
// establishes (level-triggered, deterministic naming, a narrow store
// interface for testability), applied to a database instead of a built
// application image.
//
// Redis and Postgres diverge deliberately:
//
// Redis can run safely with no password configured. That is not the
// recommended production posture, but it is not an active vulnerability
// the way unauthenticated Postgres is, so this controller reconciles
// Redis for real: create, mount its volume, start, report Ready.
//
// Postgres cannot run safely without credentials, and nothing supplies
// them yet: database auth needs the same envelope-encrypted secret
// storage CLAUDE.md 4.10 specifies (TASKS.md 1.7), which doesn't exist.
// Rather than start Postgres in trust-auth mode (no password) as a
// "temporary" workaround, Reconcile refuses to start it at all and
// reports a loud, explicit condition explaining why, mirroring how
// internal/deploy.requireNoUnresolvedEnv fails loudly rather than
// silently deploying a container missing values it needs. Controller
// carries an optional postgresCredentials field, always nil today, so
// Postgres support activates by populating it once 1.7 lands rather than
// by restructuring this controller.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Data paths inside the container each engine's image expects its
// volume mounted at.
const (
	redisDataPath    = "/data"
	postgresDataPath = "/var/lib/postgresql/data"
)

const defaultStopTimeout = 10 * time.Second

// Store is the narrow surface this controller needs from
// internal/store, so tests can fake it without a real database. *store.DB
// satisfies this.
type Store interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
}

// PostgresCredentials is what Postgres reconciliation needs once
// TASKS.md 1.7 (envelope-encrypted secrets) lands: a username and
// password to inject as POSTGRES_USER/POSTGRES_PASSWORD. Deliberately a
// plain struct, not wired to any secret store: Controller takes an
// optional, currently-always-nil *PostgresCredentials so real Postgres
// support turns on by populating it, not by rewriting this controller.
type PostgresCredentials struct {
	Username string
	Password string
}

// Controller converges one named database's desired state (read fresh
// from Store on every Reconcile, never cached) to a running,
// volume-backed container.
type Controller struct {
	dbName        string
	store         Store
	runtime       docker.Runtime
	postgresCreds *PostgresCredentials
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithPostgresCredentials supplies the credentials Postgres reconciliation
// needs. Until TASKS.md 1.7 exists nothing calls this, so every Postgres
// database reports the credentials-blocked condition instead of starting
// an unauthenticated container.
func WithPostgresCredentials(creds *PostgresCredentials) Option {
	return func(c *Controller) { c.postgresCreds = creds }
}

// New builds a Controller for dbName.
func New(dbName string, dbStore Store, runtime docker.Runtime, opts ...Option) *Controller {
	ctrl := &Controller{
		dbName:  dbName,
		store:   dbStore,
		runtime: runtime,
	}
	for _, opt := range opts {
		opt(ctrl)
	}
	return ctrl
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "database/" + c.dbName }

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	desired, err := c.store.GetDesiredDatabase(ctx, c.dbName)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		// Not a failure: a controller can be registered for a database
		// before its first save has ever written desired state.
		return unknownResult("NoDesiredState"), nil
	}
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("database/%s: get desired database: %w", c.dbName, err)
	}

	switch desired.Engine {
	case store.EngineRedis:
		return c.reconcileEngine(ctx, desired, nil, redisDataPath)

	case store.EnginePostgres:
		if c.postgresCreds == nil {
			// Deliberately not an error: this is a known, permanent,
			// documented block until TASKS.md 1.7 lands, not a transient
			// failure that should retry-and-log-error forever. The
			// condition itself is the loud explanation.
			return credentialsBlockedResult(), nil
		}
		env := map[string]string{
			"POSTGRES_USER":     c.postgresCreds.Username,
			"POSTGRES_PASSWORD": c.postgresCreds.Password,
		}
		return c.reconcileEngine(ctx, desired, env, postgresDataPath)

	default:
		err := fmt.Errorf("unrecognized engine %q", desired.Engine)
		return notReady("UnknownEngine", err), fmt.Errorf("database/%s: %w", c.dbName, err)
	}
}

// reconcileEngine is the real convergence logic, shared by Redis today
// and by Postgres once credentials are supplied. It ensures the
// database's data volume exists, then ensures the right container exists
// and is running.
func (c *Controller) reconcileEngine(ctx context.Context, desired *store.DesiredDatabase, env map[string]string, dataPath string) (reconcile.Result, error) {
	image := desired.Engine + ":" + versionOrDefault(desired.Version)
	target := containerName(c.dbName)
	volName := dataVolumeName(c.dbName)

	if err := c.runtime.EnsureVolume(ctx, volName); err != nil {
		return notReady("VolumeFailed", err), fmt.Errorf("database/%s: ensure volume %q: %w", c.dbName, volName, err)
	}

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return notReady("InspectFailed", err), fmt.Errorf("database/%s: inspect %q: %w", c.dbName, target, err)
	}

	spec := docker.ContainerSpec{
		Name:    target,
		Image:   image,
		Env:     env,
		Volumes: []docker.VolumeMount{{Name: volName, ContainerPath: dataPath}},
	}

	justDeployed := false
	switch {
	case state == nil:
		if err := c.createAndStart(ctx, spec); err != nil {
			return notReady("CreateFailed", err), fmt.Errorf("database/%s: %w", c.dbName, err)
		}
		justDeployed = true

	case state.Image != image:
		// Sequential replace, never concurrent: see containerName's doc
		// comment for why a database controller cannot use
		// application.Controller's create-alongside-then-remove-old
		// blue-green pattern.
		if err := c.replaceContainer(ctx, state, spec); err != nil {
			return notReady("ReplaceFailed", err), fmt.Errorf("database/%s: %w", c.dbName, err)
		}
		justDeployed = true

	case !state.Running:
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return notReady("StartFailed", err), fmt.Errorf("database/%s: restart %q: %w", c.dbName, target, err)
		}
		justDeployed = true
	}

	if justDeployed {
		state, err = c.runtime.InspectByName(ctx, target)
		if err != nil {
			return notReady("InspectFailed", err), fmt.Errorf("database/%s: re-inspect %q after start: %w", c.dbName, target, err)
		}
		if state == nil || !state.Running {
			return notReady("VanishedAfterStart", nil), fmt.Errorf("database/%s: %q not running immediately after starting it", c.dbName, target)
		}
		return ready("Deployed"), nil
	}

	return ready("AlreadyRunning"), nil
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

// replaceContainer stops and removes an existing container whose image
// no longer matches desired state, then creates and starts the
// replacement. Strictly sequential (stop and remove complete before
// create begins) so the data volume is never mounted by two containers
// at once.
func (c *Controller) replaceContainer(ctx context.Context, old *docker.ContainerState, spec docker.ContainerSpec) error {
	if old.Running {
		if err := c.runtime.Stop(ctx, old.ID, defaultStopTimeout); err != nil {
			return fmt.Errorf("stop %q before replace: %w", old.Name, err)
		}
	}
	if err := c.runtime.Remove(ctx, old.ID, true); err != nil {
		return fmt.Errorf("remove %q before replace: %w", old.Name, err)
	}
	return c.createAndStart(ctx, spec)
}

func versionOrDefault(v string) string {
	if v == "" {
		return "latest"
	}
	return v
}

// containerName derives this database's container name from the
// database name alone, deliberately not from a hash of image/version the
// way application.containerName derives an application container's name
// from its image.
//
// application.Controller runs an old and a new image side by side during
// a blue-green cutover because an application container is stateless:
// two of them existing briefly is harmless. A database container is not
// stateless. It holds exclusive read-write ownership of a data volume,
// and two engine instances mounting that same volume concurrently risks
// real, unrecoverable data corruption, not just a wasted container. So
// this controller keeps exactly one container name per database and,
// when the desired image changes (an engine version bump), replaces it
// in place: stop, remove, then create, strictly sequential, never two
// containers holding the volume at once. See replaceContainer.
func containerName(dbName string) string {
	return "db-" + dbName
}

// dataVolumeName is the named Docker volume backing dbName's data,
// stable across container replacements (engine version bumps) so an
// upgrade doesn't start the new version against an empty volume.
func dataVolumeName(dbName string) string {
	return "db-" + dbName + "-data"
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

// credentialsBlockedResult is the condition every Postgres database
// reports until TASKS.md 1.7 (envelope-encrypted secrets) lands: no
// credentials source exists, so Reconcile never attempts to start a
// Postgres container, unauthenticated or otherwise. See the package doc
// comment for the full reasoning.
func credentialsBlockedResult() reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type:   "Ready",
		Status: reconcile.ConditionFalse,
		Reason: "CredentialsNotYetSupported",
		Message: "postgres reconciliation is blocked on TASKS.md 1.7 (envelope-encrypted secrets); " +
			"no credentials source exists yet, so no container is started rather than running postgres unauthenticated",
	}}}
}
