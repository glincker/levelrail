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
// Redis for real: create, mount its volume, start, report Ready. That
// "not an active vulnerability" reasoning assumes internal-network-only
// reachability; PubliclyAccessible (migrations/0026_database_public_access.sql)
// breaks that assumption for a passwordless Redis, so the operator UI
// carries an explicit warning when enabling it rather than this
// controller silently refusing, matching the project's operator-trust
// posture elsewhere (no built-in service mesh or per-resource firewall).
//
// Postgres cannot run safely without credentials: database auth needs
// the same envelope-encrypted secret storage the secrets design specifies
// (TASKS.md 1.7, internal/secrets.Manager), which exists but is only
// wired into this controller when the control plane itself has a
// master key configured (cmd/levelrail's dynamicSource calls
// postgresCredentialsFor and passes the result via
// WithPostgresCredentials below). Without one, Reconcile refuses to
// start Postgres at all and reports a loud, explicit condition
// explaining why, mirroring how internal/deploy.requireNoUnresolvedEnv
// fails loudly rather than silently deploying a container missing
// values it needs, rather than starting it in trust-auth mode (no
// password) as a "temporary" workaround. Controller carries an
// optional postgresCredentials field, nil whenever no master key is
// configured or credential generation itself fails for this pass, so
// Postgres support activates purely by populating it, no restructuring
// of this controller needed either way.
package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Data paths inside the container each engine's image expects its
// volume mounted at.
const (
	redisDataPath      = "/data"
	postgresDataPath   = "/var/lib/postgresql/data"
	mysqlDataPath      = "/var/lib/mysql"
	mongoDataPath      = "/data/db"
	clickhouseDataPath = "/var/lib/clickhouse"
)

// Container ports each engine's image listens on, used to publish the
// right port when a database's desired state has PubliclyAccessible set
// (store.DesiredDatabase, migrations/0026_database_public_access.sql).
// This is real per-engine behavior, not data, so it lives here rather
// than in database_engines.yaml, the same "identity/display metadata
// only" boundary that registry's own doc comment already draws.
const (
	redisContainerPort      = 6379
	postgresContainerPort   = 5432
	mysqlContainerPort      = 3306
	mongoContainerPort      = 27017
	clickhouseContainerPort = 8123
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

// MySQLCredentials is what MySQL reconciliation needs: the same
// dbName-scoped username/password shape PostgresCredentials already
// establishes, injected as MYSQL_USER/MYSQL_PASSWORD/MYSQL_DATABASE
// (see reconcileMySQLEnv). Password also backs MYSQL_ROOT_PASSWORD:
// unlike Postgres' image, MySQL's own image refuses to start at all
// without a root password (or an explicit, deliberately-not-used
// empty-password opt-out), so there is no unauthenticated-but-running
// MySQL state this controller could otherwise reach.
type MySQLCredentials struct {
	Username string
	Password string
}

// MongoDBCredentials is what MongoDB reconciliation needs: the same
// dbName-scoped username/password shape PostgresCredentials/
// MySQLCredentials already establish, injected as
// MONGO_INITDB_ROOT_USERNAME/MONGO_INITDB_ROOT_PASSWORD, the official
// mongo image's own root-credential env vars. Unlike MySQL's image,
// mongo's does not refuse to start without these, so this controller
// still requires them (WithMongoDBCredentials unset blocks reconciling
// just like an unset WithPostgresCredentials/WithMySQLCredentials
// does), keeping every managed database on the same "never
// unauthenticated" posture rather than making Mongo the one exception.
type MongoDBCredentials struct {
	Username string
	Password string
}

// MariaDBCredentials is MySQLCredentials' counterpart for the official
// mariadb image. Injected as MARIADB_ROOT_PASSWORD/MARIADB_USER/
// MARIADB_PASSWORD/MARIADB_DATABASE, not the MYSQL_* names: the mariadb
// image accepts MYSQL_* only as a legacy alias ("MARIADB_* variables
// will be used in preference to MYSQL_* variables" per its own docs),
// and from MariaDB 11 the mysqldump/mysql client binaries themselves are
// gone from the image, so this controller and internal/backup's dump/
// restore commands both use the MariaDB-native names throughout, not the
// MySQL-shaped ones this image only keeps for someone else's old scripts.
type MariaDBCredentials struct {
	Username string
	Password string
}

// ClickHouseCredentials is what ClickHouse reconciliation needs,
// injected as CLICKHOUSE_USER/CLICKHOUSE_PASSWORD/CLICKHOUSE_DB. Unlike
// MySQL/MariaDB, the image doesn't refuse to start without these, but
// this controller still requires them, the same "never unauthenticated"
// posture MongoDBCredentials' own doc comment explains.
type ClickHouseCredentials struct {
	Username string
	Password string
}

// Controller converges one named database's desired state (read fresh
// from Store on every Reconcile, never cached) to a running,
// volume-backed container.
type Controller struct {
	dbName          string
	store           Store
	runtime         docker.Runtime
	postgresCreds   *PostgresCredentials
	mysqlCreds      *MySQLCredentials
	mongoCreds      *MongoDBCredentials
	mariadbCreds    *MariaDBCredentials
	clickhouseCreds *ClickHouseCredentials
	meshDNSAddr     string // empty is valid: no mesh DNS server is running, or it hasn't resolved a container-reachable address, see WithMeshDNSAddr
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

// WithMySQLCredentials supplies the credentials MySQL reconciliation
// needs, the same "populate to activate, no restructuring needed"
// shape WithPostgresCredentials already establishes. Without one, every
// MySQL database reports the credentials-blocked condition, identical
// reasoning to Postgres: no container starts without real auth.
func WithMySQLCredentials(creds *MySQLCredentials) Option {
	return func(c *Controller) { c.mysqlCreds = creds }
}

// WithMongoDBCredentials supplies the credentials MongoDB reconciliation
// needs, the same "populate to activate, no restructuring needed" shape
// WithPostgresCredentials/WithMySQLCredentials already establish.
// Without one, every MongoDB database reports the credentials-blocked
// condition, identical reasoning to Postgres and MySQL.
func WithMongoDBCredentials(creds *MongoDBCredentials) Option {
	return func(c *Controller) { c.mongoCreds = creds }
}

// WithMariaDBCredentials supplies the credentials MariaDB reconciliation
// needs, the same "populate to activate, no restructuring needed" shape
// the other WithXCredentials options already establish. Without one,
// every MariaDB database reports the credentials-blocked condition.
func WithMariaDBCredentials(creds *MariaDBCredentials) Option {
	return func(c *Controller) { c.mariadbCreds = creds }
}

// WithClickHouseCredentials supplies the credentials ClickHouse
// reconciliation needs. Without one, every ClickHouse database reports
// the credentials-blocked condition.
func WithClickHouseCredentials(creds *ClickHouseCredentials) Option {
	return func(c *Controller) { c.clickhouseCreds = creds }
}

// WithMeshDNSAddr points every container this controller creates at addr,
// a bare nameserver IP (never "ip:port": neither Docker's DNS HostConfig
// field nor a container's own resolv.conf supports a non-standard
// nameserver port, confirmed live while building this option, see
// cmd/levelrail/mesh.go's dockerNameserverPort doc comment), as an
// additional nameserver ahead of whatever Docker's own resolver would
// otherwise configure (docker.ContainerSpec.DNS). Without one configured
// (the default, empty string), Reconcile behaves exactly as before this
// field existed: no DNS override at all. addr is expected to already be a
// real, container-reachable address (cmd/levelrail/mesh.go's
// containerDNSAddr resolves Docker's own bridge gateway IP, only once the
// mesh DNS server is confirmed bound to port 53); this controller does
// not validate it.
func WithMeshDNSAddr(addr string) Option {
	return func(c *Controller) { c.meshDNSAddr = addr }
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
		return c.reconcileEngine(ctx, desired, nil, nil, redisDataPath, redisContainerPort)

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
		return c.reconcileEngine(ctx, desired, env, nil, postgresDataPath, postgresContainerPort)

	case store.EngineMySQL:
		if c.mysqlCreds == nil {
			// Same reasoning as the Postgres branch above: a known,
			// permanent, documented block, not a transient failure.
			return credentialsBlockedResult(), nil
		}
		env := map[string]string{
			// MYSQL_ROOT_PASSWORD is mandatory for this image to start
			// at all (see MySQLCredentials' own doc comment); MYSQL_USER/
			// MYSQL_PASSWORD/MYSQL_DATABASE mirror the dbName-scoped
			// user+database Postgres' own POSTGRES_USER already creates,
			// so both engines end up with an equivalent "one user, one
			// database, named after the resource" shape from this
			// controller's point of view.
			"MYSQL_ROOT_PASSWORD": c.mysqlCreds.Password,
			"MYSQL_DATABASE":      c.dbName,
			"MYSQL_USER":          c.mysqlCreds.Username,
			"MYSQL_PASSWORD":      c.mysqlCreds.Password,
		}
		return c.reconcileEngine(ctx, desired, env, nil, mysqlDataPath, mysqlContainerPort)

	case store.EngineMongoDB:
		if c.mongoCreds == nil {
			// Same reasoning as the Postgres/MySQL branches above: a
			// known, permanent, documented block, not a transient failure.
			return credentialsBlockedResult(), nil
		}
		env := map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": c.mongoCreds.Username,
			"MONGO_INITDB_ROOT_PASSWORD": c.mongoCreds.Password,
		}
		return c.reconcileEngine(ctx, desired, env, nil, mongoDataPath, mongoContainerPort)

	case store.EngineMariaDB:
		if c.mariadbCreds == nil {
			// Same reasoning as the Postgres/MySQL/MongoDB branches above:
			// a known, permanent, documented block, not a transient
			// failure.
			return credentialsBlockedResult(), nil
		}
		env := map[string]string{
			// See MariaDBCredentials' own doc comment for why these are
			// MARIADB_*, not MYSQL_*: the same one-user-one-database shape
			// the EngineMySQL case above sets up, MariaDB-native names.
			"MARIADB_ROOT_PASSWORD": c.mariadbCreds.Password,
			"MARIADB_DATABASE":      c.dbName,
			"MARIADB_USER":          c.mariadbCreds.Username,
			"MARIADB_PASSWORD":      c.mariadbCreds.Password,
		}
		// MariaDB is a MySQL-protocol-compatible fork: same data
		// directory and port as the mysql image, reused directly rather
		// than duplicating identical constants under a new name.
		return c.reconcileEngine(ctx, desired, env, nil, mysqlDataPath, mysqlContainerPort)

	case store.EngineKeyDB:
		// KeyDB is a Redis-protocol-compatible drop-in fork: same
		// passwordless-by-default posture, same /data path, same port,
		// reused directly rather than duplicating identical constants
		// under a new name. See the package doc comment for why an
		// unauthenticated Redis-family database is an accepted posture
		// here.
		return c.reconcileEngine(ctx, desired, nil, nil, redisDataPath, redisContainerPort)

	case store.EngineDragonfly:
		// dragonflyCommand pins dbfilename so restoreRedisLike's
		// /data/dump.rdb write actually gets loaded on restart; Dragonfly's
		// own default (dump-{timestamp}) never matches that path.
		return c.reconcileEngine(ctx, desired, nil, dragonflyCommand, redisDataPath, redisContainerPort)

	case store.EngineClickHouse:
		if c.clickhouseCreds == nil {
			return credentialsBlockedResult(), nil
		}
		env := map[string]string{
			"CLICKHOUSE_DB":                        c.dbName,
			"CLICKHOUSE_USER":                      c.clickhouseCreds.Username,
			"CLICKHOUSE_PASSWORD":                  c.clickhouseCreds.Password,
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": "1",
		}
		return c.reconcileEngine(ctx, desired, env, nil, clickhouseDataPath, clickhouseContainerPort)

	default:
		err := fmt.Errorf("unrecognized engine %q", desired.Engine)
		return notReady("UnknownEngine", err), fmt.Errorf("database/%s: %w", c.dbName, err)
	}
}

// reconcileEngine is the real convergence logic, shared by Redis today
// and by Postgres once credentials are supplied. It ensures the
// database's data volume exists, then ensures the right container exists
// and is running.
func (c *Controller) reconcileEngine(ctx context.Context, desired *store.DesiredDatabase, env map[string]string, command []string, dataPath string, containerPort int) (reconcile.Result, error) {
	image := dockerImageFor(desired.Engine) + ":" + versionOrDefault(desired.Version)
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
		Command: command,
		Volumes: []docker.VolumeMount{{Name: volName, ContainerPath: dataPath}},
	}
	if c.meshDNSAddr != "" {
		spec.DNS = []string{c.meshDNSAddr}
	}
	// PubliclyAccessible/PublicPort (migrations/0026_database_public_access.sql):
	// bind the engine's own container port to the host port
	// SetDatabasePublicAccess already assigned, so an operator's own
	// database GUI tool can reach it directly. Nil/empty otherwise,
	// byte-identical to every database before this field existed:
	// internal-network-only, the same as an application container that
	// declares no port.
	if desired.PubliclyAccessible && desired.PublicPort != 0 {
		spec.Ports = []docker.PortBinding{{ContainerPort: containerPort, HostPort: desired.PublicPort}}
	}
	if desired.Resources != nil {
		spec.Resources = &docker.Resources{
			MemoryBytes: desired.Resources.MemoryBytes,
			NanoCPUs:    desired.Resources.NanoCPUs,
		}
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

	case state.Running && !portsMatch(state.Ports, spec.Ports):
		// Docker bakes a container's published ports into its create-time
		// HostConfig; toggling public access or changing the assigned
		// port can't be applied to a running container in place, so this
		// takes the same sequential replace path an image change does.
		// Only compared while state.Running: a stopped container reports
		// no observed ports at all (docker.ContainerState.Ports' own doc
		// comment), so there is nothing meaningful to diff against desired
		// here; the !state.Running case below just restarts it as-is,
		// a known gap if desired ports changed while it happened to be
		// crashed (see this package's own test for the corresponding
		// live-Docker proof of the running-container path).
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

// portsMatch reports whether observed (a running container's actual
// published ports, docker.ContainerState.Ports) and desired (the
// ContainerSpec.Ports this controller would create) describe the same
// set of bindings, so reconcileEngine knows whether a public-access
// toggle or port change needs a replace. Protocol is normalized ("" and
// "tcp" are the same binding): spec.Ports built by this package never
// sets Protocol explicitly, while Docker's own observed ports always
// report a concrete one.
func portsMatch(observed, desired []docker.PortBinding) bool {
	return reflect.DeepEqual(normalizedPortSet(observed), normalizedPortSet(desired))
}

func normalizedPortSet(ports []docker.PortBinding) map[docker.PortBinding]bool {
	set := make(map[docker.PortBinding]bool, len(ports))
	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		set[docker.PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: proto}] = true
	}
	return set
}

// dockerImageMapping holds engines whose registry id (see
// database_engines.yaml) doesn't match its Docker image name; any id
// absent here is used as its own image name.
var dockerImageMapping = map[string]string{
	store.EngineMongoDB:   "mongo",
	store.EngineKeyDB:     "eqalpha/keydb",
	store.EngineDragonfly: "docker.dragonflydb.io/dragonflydb/dragonfly",
	// ClickHouse's own registry namespace, not a Docker Official Image:
	// clickhouse/clickhouse-server is what upstream publishes.
	store.EngineClickHouse: "clickhouse/clickhouse-server",
}

// dragonflyCommand overrides the image's default CMD: its entrypoint.sh
// prepends the "dragonfly" binary itself whenever the first arg starts
// with "-", so this is passed as flags only, matching Dragonfly's own
// documented invocation (--dbfilename without a "dragonfly" prefix).
var dragonflyCommand = []string{"--dbfilename", "dump"}

// dockerImageFor returns the Docker image name for engine, applying
// dockerImageMapping where the registry id and image name diverge.
func dockerImageFor(engine string) string {
	if image, ok := dockerImageMapping[engine]; ok {
		return image
	}
	return engine
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

// credentialsBlockedResult is the condition every credentialed engine
// reports when it has no usable credentials: no container starts rather
// than running unauthenticated. See the package doc comment.
func credentialsBlockedResult() reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type:   "Ready",
		Status: reconcile.ConditionFalse,
		Reason: "CredentialsNotConfigured",
		Message: "no credentials available for this database; either the control plane has no secrets master key configured, " +
			"or generating/resolving this database's own credentials failed, so no container is started rather than running it unauthenticated",
	}}}
}
