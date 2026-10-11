package resourceadvisor

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

// Engine names, matching store.Engine*; kept local so this package stays pure.
const (
	enginePostgres = "postgres"
	engineRedis    = "redis"
	engineKeyDB    = "keydb"
	engineMySQL    = "mysql"
	engineMariaDB  = "mariadb"
)

func isPgvector(r Resource) bool {
	s := strings.ToLower(r.Version + " " + r.Image)
	return strings.Contains(s, "pgvector") || strings.Contains(s, "vector")
}

// pgSize renders bytes in Postgres's own unit syntax.
func pgSize(b int64) string {
	if b >= gib && b%gib == 0 {
		return fmt.Sprintf("%dGB", b/gib)
	}
	return fmt.Sprintf("%dMB", max(1, b/mib))
}

func clamp(v, lo, hi int64) int64 { return max(lo, min(v, hi)) }

// tuningFinding is rule f: engine settings sized from the container memory.
// It only suggests; nothing here can be applied by the advisor.
func tuningFinding(r Resource, th Thresholds, now time.Time) (Finding, bool) {
	if r.Ref.Kind != ResourceDatabase {
		return Finding{}, false
	}
	mem := within(r.Memory, now.Add(-th.Window))
	if coverage(mem, now) < th.MinData {
		return Finding{}, false
	}
	s := summarize(mem)
	vector := r.Engine == enginePostgres && isPgvector(r)
	if int64(s.p95) < th.TuningMinMemory && !vector {
		return Finding{}, false
	}
	basis, basisLabel := r.MemoryLimit, "its memory limit"
	if basis <= 0 {
		basis = rightsizing.RoundMemoryUp(math.Max(s.p95*th.MemoryHeadroom, s.max))
		basisLabel = "the suggested memory limit"
	}
	f := newFinding(KindEngineTuning, SeverityInfo, r.Ref)
	f.Evidence = memoryEvidence(mem, r.MemoryLimit, th.Window)
	f.Evidence.Numbers = map[string]float64{"basis_bytes": float64(basis)}
	f.Params["basis"], f.Params["engine"] = rightsizing.FormatMemory(basis), r.Engine
	settings := map[string]string{}
	var why, how string
	switch r.Engine {
	case enginePostgres:
		maxConn := int64(50)
		switch {
		case basis >= 4*gib:
			maxConn = 200
		case basis >= gib:
			maxConn = 100
		}
		shared := clamp(basis/4, 32*mib, 8*gib)
		work := clamp((basis-shared)/(maxConn*3), mib, 64*mib)
		maint := clamp(basis/16, 16*mib, gib)
		if vector {
			maint = clamp(basis/4, 64*mib, 2*gib)
		}
		settings["shared_buffers"] = pgSize(shared)
		settings["effective_cache_size"] = pgSize(basis * 3 / 4)
		settings["work_mem"] = pgSize(work)
		settings["maintenance_work_mem"] = pgSize(maint)
		settings["max_connections"] = fmt.Sprint(maxConn)
		why = fmt.Sprintf("Sized from %s (%s): shared_buffers at a quarter, effective_cache_size at three quarters, work_mem so %d connections running a few sorts each still fit.",
			basisLabel, rightsizing.FormatMemory(basis), maxConn)
		if vector {
			why += " This is a pgvector database: HNSW and IVFFlat index builds run inside maintenance_work_mem and spill to disk, much slower, when the index does not fit, so give it more memory than a plain Postgres of the same size."
			f.Params["pgvector"] = "true"
		}
		how = "Run ALTER SYSTEM SET <name> = '<value>'; for each, then SELECT pg_reload_conf(); shared_buffers and max_connections take effect after a restart. Compare with SHOW <name>; first: the advisor cannot read the current values and assumes image defaults (shared_buffers 128MB, max_connections 100)."
	case engineRedis, engineKeyDB:
		settings["maxmemory"] = rightsizing.FormatMemory(basis * 3 / 4)
		settings["maxmemory-policy"] = "allkeys-lru (cache) or noeviction (queues, primary data)"
		why = fmt.Sprintf("Without maxmemory, Redis grows until the container limit kills it. Three quarters of %s (%s) leaves room for fork during persistence.", basisLabel, rightsizing.FormatMemory(basis))
		how = "Run CONFIG SET maxmemory <bytes> and CONFIG SET maxmemory-policy <policy>, then CONFIG REWRITE if Redis uses a config file. Pick allkeys-lru only if every key can be lost."
	case engineMySQL, engineMariaDB:
		pool := max(128*mib, (basis/2)/(128*mib)*(128*mib))
		settings["innodb_buffer_pool_size"] = pgSize(pool)
		why = fmt.Sprintf("The InnoDB buffer pool should hold the hot data set: about half of %s (%s) leaves room for connections and the OS.", basisLabel, rightsizing.FormatMemory(basis))
		how = "Run SET GLOBAL innodb_buffer_pool_size = <bytes>; (resizes online) and persist it in the server config. The image default is 128MB."
	default:
		return Finding{}, false
	}
	f.Title = fmt.Sprintf("Tune %s settings for %s", r.Engine, r.Ref.Name)
	f.Why = why
	f.Action = &Action{Type: ActionManual, Settings: settings, Instructions: how}
	return f, true
}
