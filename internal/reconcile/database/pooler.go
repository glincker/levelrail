package database

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env knobs for the managed Postgres pooler. The image falls back to a pinned
// default; operators override it with a tag or digest they have vetted.
const (
	EnvPoolerImage           = "APP_PG_POOLER_IMAGE"
	EnvPoolerDefaultPoolSize = "APP_PG_POOLER_DEFAULT_POOL_SIZE"
	EnvPoolerMaxClientConn   = "APP_PG_POOLER_MAX_CLIENT_CONN"
)

// DefaultPoolerImage is used when EnvPoolerImage is unset. It is pinned to an
// exact version and speaks the env contract poolerEnv writes.
const DefaultPoolerImage = "edoburu/pgbouncer:v1.26.1-p0"

// PoolerPort is where the pooler listens. It shares the database's network
// namespace, so it must not collide with Postgres' own 5432.
const PoolerPort = 6432

// PoolerConditionType is reported next to Ready so a broken pooler never
// marks the database itself as down.
const PoolerConditionType = "PoolerReady"

const (
	poolerTargetLabelKey  = "db.pooler-target"
	poolerConfigLabelKey  = "db.pooler-config"
	poolerAuthRevLabelKey = "db.pooler-authrev"
)

// PoolerContainerName is the pooler child container for dbName.
func PoolerContainerName(dbName string) string { return containerName(dbName) + "-pooler" }

// PoolerImage is the env override, else the pinned default.
func PoolerImage() string {
	if v := strings.TrimSpace(os.Getenv(EnvPoolerImage)); v != "" {
		return v
	}
	return DefaultPoolerImage
}

// PoolerActive reports whether apps should be handed the pooled address.
func PoolerActive(d *store.DesiredDatabase) bool {
	return d != nil && d.PoolerEnabled && d.Engine == store.EnginePostgres && !d.Suspended
}

// positiveEnv returns the env value when it is a positive integer, so a typo
// falls back to the pooler's own default instead of a broken config.
func positiveEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return v
	}
	return ""
}

// poolerEnv configures a pgbouncer-style image through its documented env
// contract. Transaction pooling is the whole point of the feature.
func poolerEnv(user, password string, poolSize, maxClientConn int) map[string]string {
	env := map[string]string{
		"DB_HOST":            "127.0.0.1",
		"DB_PORT":            strconv.Itoa(postgresContainerPort),
		"DB_USER":            user,
		"DB_PASSWORD":        password,
		"LISTEN_PORT":        strconv.Itoa(PoolerPort),
		"POOL_MODE":          "transaction",
		"AUTH_TYPE":          "scram-sha-256",
		"STATS_USERS":        user,
		"SERVER_TLS_SSLMODE": "prefer",
	}
	// A per-database value wins over the control plane env default.
	if poolSize > 0 {
		env["DEFAULT_POOL_SIZE"] = strconv.Itoa(poolSize)
	} else if v := positiveEnv(EnvPoolerDefaultPoolSize); v != "" {
		env["DEFAULT_POOL_SIZE"] = v
	}
	if maxClientConn > 0 {
		env["MAX_CLIENT_CONN"] = strconv.Itoa(maxClientConn)
	} else if v := positiveEnv(EnvPoolerMaxClientConn); v != "" {
		env["MAX_CLIENT_CONN"] = v
	}
	return env
}

// poolerConfigRevision hashes every setting except the password, so a label
// never carries anything derived from a credential. STATS_USERS is left out so
// adding it does not recreate poolers that already run.
func poolerConfigRevision(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		if k != "DB_PASSWORD" && k != "STATS_USERS" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s\n", k, env[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// poolerCredRevision is a short salted digest, so a rotated password replaces
// the pooler (its env is fixed at create) without the label exposing it.
func poolerCredRevision(dbName, password string) string {
	sum := sha256.Sum256([]byte("pooler-cred:" + dbName + ":" + password))
	return hex.EncodeToString(sum[:])[:16]
}

func poolerCondition(status reconcile.ConditionStatus, reason, msg string) reconcile.Condition {
	return reconcile.Condition{Type: PoolerConditionType, Status: status, Reason: reason, Message: msg}
}

// withPooler converges the pooler child after the database pass and appends
// its condition. Disabled or non-Postgres databases just get it removed.
func (c *Controller) withPooler(ctx context.Context, desired *store.DesiredDatabase, res reconcile.Result) reconcile.Result {
	if !PoolerActive(desired) {
		if err := c.removePooler(ctx); err != nil {
			res.Conditions = append(res.Conditions, poolerCondition(reconcile.ConditionFalse, "PoolerRemoveFailed", err.Error()))
		}
		return res
	}
	res.Conditions = append(res.Conditions, c.reconcilePooler(ctx, desired))
	return res
}

func (c *Controller) reconcilePooler(ctx context.Context, desired *store.DesiredDatabase) reconcile.Condition {
	image := PoolerImage()
	if image == "" {
		return poolerCondition(reconcile.ConditionFalse, "PoolerImageNotConfigured", "set "+EnvPoolerImage+" to a pinned pooler image tag or digest")
	}
	if c.postgresCreds == nil {
		return poolerCondition(reconcile.ConditionFalse, "CredentialsNotConfigured", "the database has no credentials yet")
	}
	db, err := c.runtime.InspectByName(ctx, containerName(c.dbName))
	if err != nil {
		return poolerCondition(reconcile.ConditionUnknown, "InspectFailed", err.Error())
	}
	if db == nil || !db.Running {
		return poolerCondition(reconcile.ConditionFalse, "DatabaseNotRunning", "the pooler starts once the database container runs")
	}

	env := poolerEnv(c.postgresCreds.Username, c.postgresCreds.Password, desired.PoolerPoolSize, desired.PoolerMaxClientConn)
	spec := docker.ContainerSpec{
		Name:  PoolerContainerName(c.dbName),
		Image: image,
		Env:   env,
		// Joining the database's namespace keeps the pooler on the same host
		// and IP apps already resolve, only the port changes.
		NetworkMode: "container:" + db.ID,
		Labels: map[string]string{
			poolerTargetLabelKey:  db.ID,
			poolerConfigLabelKey:  poolerConfigRevision(env),
			poolerAuthRevLabelKey: poolerCredRevision(c.dbName, c.postgresCreds.Password),
		},
	}
	if c.prov != nil {
		spec.OnPull = c.prov.Pull
	}

	state, err := c.runtime.InspectByName(ctx, spec.Name)
	if err != nil {
		return poolerCondition(reconcile.ConditionUnknown, "InspectFailed", err.Error())
	}
	if state != nil && poolerStale(state, spec) {
		if err := c.removeContainer(ctx, state); err != nil {
			return poolerCondition(reconcile.ConditionFalse, "PoolerReplaceFailed", err.Error())
		}
		state = nil
	}
	switch {
	case state == nil:
		id, err := c.runtime.Create(ctx, spec)
		if err != nil {
			return poolerCondition(reconcile.ConditionFalse, "PoolerCreateFailed", fmt.Sprintf("create %q: %v", spec.Name, err))
		}
		if err := c.runtime.Start(ctx, id); err != nil {
			return poolerCondition(reconcile.ConditionFalse, "PoolerStartFailed", fmt.Sprintf("start %q: %v", spec.Name, err))
		}
	case !state.Running:
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return poolerCondition(reconcile.ConditionFalse, "PoolerStartFailed", fmt.Sprintf("start %q: %v", spec.Name, err))
		}
	}
	return poolerCondition(reconcile.ConditionTrue, "PoolerRunning", "")
}

// poolerStale is true when the database container was replaced (its old
// namespace is gone), the image changed, the pool settings changed, or the password rotated.
func poolerStale(state *docker.ContainerState, spec docker.ContainerSpec) bool {
	if !docker.SameImageRef(state.Image, spec.Image) {
		return true
	}
	if state.LabelsUnknown {
		return false
	}
	return state.Labels[poolerTargetLabelKey] != spec.Labels[poolerTargetLabelKey] ||
		state.Labels[poolerConfigLabelKey] != spec.Labels[poolerConfigLabelKey] ||
		state.Labels[poolerAuthRevLabelKey] != spec.Labels[poolerAuthRevLabelKey]
}

func (c *Controller) removeContainer(ctx context.Context, state *docker.ContainerState) error {
	if state.Running {
		if err := c.runtime.Stop(ctx, state.ID, defaultStopTimeout); err != nil {
			return fmt.Errorf("stop %q: %w", state.Name, err)
		}
	}
	if err := c.runtime.Remove(ctx, state.ID, true); err != nil {
		return fmt.Errorf("remove %q: %w", state.Name, err)
	}
	return nil
}

// removePooler is idempotent: no pooler container means nothing to do.
func (c *Controller) removePooler(ctx context.Context) error {
	state, err := c.runtime.InspectByName(ctx, PoolerContainerName(c.dbName))
	if err != nil {
		return fmt.Errorf("database/%s: inspect pooler: %w", c.dbName, err)
	}
	if state == nil {
		return nil
	}
	if err := c.removeContainer(ctx, state); err != nil {
		return fmt.Errorf("database/%s: %w", c.dbName, err)
	}
	return nil
}

// PoolSaturation is the client queue pgbouncer reports across the pools of one database.
type PoolSaturation struct {
	ClientsWaiting int
	MaxWaitSeconds float64
}

// ParseShowPools reads psql -A output of pgbouncer SHOW POOLS (header row, then one row per pool, separated by |).
// Columns are found by header name because their order differs between pgbouncer releases.
func ParseShowPools(out string) (PoolSaturation, error) {
	var sat PoolSaturation
	var cols map[string]int
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "(") {
			continue
		}
		fields := strings.Split(line, "|")
		if cols == nil {
			cols = make(map[string]int, len(fields))
			for i, f := range fields {
				cols[strings.TrimSpace(f)] = i
			}
			if _, ok := cols["cl_waiting"]; !ok {
				return PoolSaturation{}, fmt.Errorf("pooler: SHOW POOLS output has no cl_waiting column")
			}
			continue
		}
		if i, ok := cols["database"]; ok && i < len(fields) && strings.TrimSpace(fields[i]) == "pgbouncer" {
			continue
		}
		if n, err := strconv.Atoi(cell(fields, cols["cl_waiting"])); err == nil {
			sat.ClientsWaiting += n
		}
		if i, ok := cols["maxwait"]; ok {
			w, _ := strconv.ParseFloat(cell(fields, i), 64)
			if i2, ok := cols["maxwait_us"]; ok {
				us, _ := strconv.ParseFloat(cell(fields, i2), 64)
				w += us / 1e6
			}
			sat.MaxWaitSeconds = max(sat.MaxWaitSeconds, w)
		}
	}
	if cols == nil {
		return PoolSaturation{}, fmt.Errorf("pooler: SHOW POOLS returned no rows")
	}
	return sat, nil
}

func cell(fields []string, i int) string {
	if i < 0 || i >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[i])
}
