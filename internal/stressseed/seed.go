// Package stressseed fills a store with a deterministic, large resource graph
// for UI and stress measurement. Dev tooling only: never imported by the control plane.
package stressseed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Config sizes the graph. The same Seed and Anchor always produce the same rows.
type Config struct {
	Apps          int
	Databases     int
	Nodes         int
	Deploys       int
	EnvsPerApp    int
	DomainsPerApp int
	Seed          uint64
	// Anchor is the "now" that every timestamp is computed from; zero means the current time.
	Anchor time.Time
	// NodeIDs places apps round-robin on these existing nodes; empty leaves them on the local node.
	NodeIDs []string
}

// Manifest names what a seed produced. Digest is a hash of every inserted row.
type Manifest struct {
	Apps      []string
	Databases []string
	Nodes     []string
	Deploys   int
	Digest    string
}

type column struct {
	name, typ string
	required  bool
}

// Seed inserts the graph in one transaction. It refuses a store that already holds the seeded app names.
func Seed(ctx context.Context, db *store.DB, cfg Config) (Manifest, error) {
	if cfg.Anchor.IsZero() {
		cfg.Anchor = time.Now()
	}
	cfg.Anchor = cfg.Anchor.UTC()
	rng := &splitmix{state: cfg.Seed}
	ts := func(ago time.Duration) string { return cfg.Anchor.Add(-ago).Format("2006-01-02T15:04:05.000Z") }

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("stressseed: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	h := sha256.New()
	cache := map[string][]column{}
	ins := func(table string, vals map[string]any) error {
		cols, ok := cache[table]
		if !ok {
			var cerr error
			if cols, cerr = tableColumns(ctx, tx, table); cerr != nil {
				return cerr
			}
			cache[table] = cols
		}
		var names, marks []string
		var args []any
		for _, c := range cols {
			v, ok := vals[c.name]
			if !ok {
				if !c.required {
					continue
				}
				if strings.Contains(strings.ToUpper(c.typ), "INT") {
					v = 0
				} else {
					v = ""
				}
			}
			names = append(names, c.name)
			marks = append(marks, "?")
			args = append(args, v)
		}
		_, _ = fmt.Fprintf(h, "%s|%v|%v\n", table, names, args)
		//nolint:gosec // table names are compile-time constants from this file
		q := "INSERT INTO " + table + " (" + strings.Join(names, ",") + ") VALUES (" + strings.Join(marks, ",") + ")"
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("stressseed: insert %s: %w", table, err)
		}
		return nil
	}

	var m Manifest
	for i := 0; i < cfg.Nodes; i++ {
		id := fmt.Sprintf("node-%04d", i)
		status := []string{"online", "online", "online", "offline"}[rng.Intn(4)]
		if err := ins("nodes", map[string]any{
			"id": id, "name": fmt.Sprintf("perf-node-%03d", i),
			"address": fmt.Sprintf("10.0.%d.%d", i/250, i%250+1), "status": status,
			"joined_at": ts(48 * time.Hour), "last_seen_at": ts(time.Duration(rng.Intn(600)) * time.Second),
		}); err != nil {
			return Manifest{}, err
		}
		m.Nodes = append(m.Nodes, id)
	}
	for i := 0; i < cfg.Apps; i++ {
		name := fmt.Sprintf("perf-app-%04d", i)
		env := map[string]string{}
		for e := 0; e < cfg.EnvsPerApp; e++ {
			env[fmt.Sprintf("STRESS_VAR_%02d", e)] = fmt.Sprintf("value-%d-%d", i, rng.Intn(100000))
		}
		envJSON, _ := json.Marshal(env)
		domains := make([]string, 0, cfg.DomainsPerApp)
		for d := 0; d < cfg.DomainsPerApp; d++ {
			domains = append(domains, fmt.Sprintf("%s-%d.stress.invalid", name, d))
		}
		domJSON, _ := json.Marshal(domains)
		vals := map[string]any{
			"name": name, "image": fmt.Sprintf("registry.invalid/perf/app-%d:latest", i),
			"port": 3000 + i%100, "env": string(envJSON), "domains": string(domJSON),
			"updated_at": ts(time.Duration(rng.Intn(100000)) * time.Second),
		}
		if len(cfg.NodeIDs) > 0 {
			vals["node_id"] = cfg.NodeIDs[i%len(cfg.NodeIDs)]
		}
		if err := ins("desired_services", vals); err != nil {
			return Manifest{}, err
		}
		for _, d := range domains {
			if err := ins("service_domains", map[string]any{"domain": d, "service_name": name}); err != nil {
				return Manifest{}, err
			}
		}
		m.Apps = append(m.Apps, name)
	}
	for i := 0; i < cfg.Databases; i++ {
		name := fmt.Sprintf("perf-db-%04d", i)
		if err := ins("desired_databases", map[string]any{
			"name": name, "engine": []string{"postgres", "redis"}[i%2], "version": "16",
			"updated_at": ts(time.Duration(rng.Intn(100000)) * time.Second),
		}); err != nil {
			return Manifest{}, err
		}
		m.Databases = append(m.Databases, name)
	}
	for i := 0; i < cfg.Deploys && cfg.Apps > 0; i++ {
		started := time.Duration(rng.Intn(30*24*3600)+60) * time.Second
		status := []string{"succeeded", "succeeded", "succeeded", "failed"}[rng.Intn(4)]
		vals := map[string]any{
			"id": fmt.Sprintf("dpl_perf%06d", i), "service_name": fmt.Sprintf("perf-app-%04d", rng.Intn(cfg.Apps)),
			"image": "registry.invalid/perf/app:latest", "status": status,
			"started_at": ts(started), "finished_at": ts(started - 40*time.Second),
		}
		if status == "failed" {
			vals["error"] = "seeded failure"
		}
		if err := ins("deploy_attempts", vals); err != nil {
			return Manifest{}, err
		}
		m.Deploys++
	}
	if err := tx.Commit(); err != nil {
		return Manifest{}, fmt.Errorf("stressseed: commit: %w", err)
	}
	m.Digest = hex.EncodeToString(h.Sum(nil))
	sort.Strings(m.Apps)
	return m, nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) ([]column, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, fmt.Errorf("stressseed: table info %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var out []column
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("stressseed: scan table info: %w", err)
		}
		out = append(out, column{name: name, typ: typ, required: notnull == 1 && !dflt.Valid})
	}
	return out, rows.Err()
}

// splitmix is a tiny deterministic generator: fixtures only need repeatable spread, not randomness.
type splitmix struct{ state uint64 }

func (s *splitmix) Intn(n int) int {
	s.state += 0x9e3779b97f4a7c15
	z := s.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return int(z % uint64(n)) //nolint:gosec // n is always a small positive constant
}
