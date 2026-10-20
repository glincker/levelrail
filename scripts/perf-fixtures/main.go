// Command perf-fixtures seeds a scratch control plane data dir with a
// large, deterministic resource graph for UI performance measurement.
// Dev tooling only: never imported by the control plane and it refuses
// a data dir that already holds a database.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type column struct {
	name, typ string
	required  bool
}

func main() {
	dir := flag.String("data-dir", "", "scratch APP_DATA_DIR to seed (must not hold a database yet)")
	apps := flag.Int("apps", 1000, "apps to create")
	dbs := flag.Int("databases", 200, "databases to create")
	nodes := flag.Int("nodes", 50, "nodes to create")
	deploys := flag.Int("deploys", 20000, "deploy attempts to create")
	seed := flag.Uint64("seed", 1, "random seed")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "perf-fixtures: -data-dir is required")
		os.Exit(2)
	}
	if err := run(*dir, *apps, *dbs, *nodes, *deploys, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "perf-fixtures:", err)
		os.Exit(1)
	}
}

func run(dir string, apps, dbs, nodes, deploys int, seed uint64) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dir, "levelrail.db")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists, refusing to seed a used data dir", path)
	}
	ctx := context.Background()
	db, err := store.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = db.Close() }()

	rng := &splitmix{state: seed}
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format("2006-01-02T15:04:05.000Z") }

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	cache := map[string][]column{}
	ins := func(table string, vals map[string]any) error {
		cols, ok := cache[table]
		if !ok {
			if cols, err = tableColumns(ctx, tx, table); err != nil {
				return err
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
		//nolint:gosec // table names are compile-time constants from this file
		q := "INSERT INTO " + table + " (" + strings.Join(names, ",") + ") VALUES (" + strings.Join(marks, ",") + ")"
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("seed %s: %w", table, err)
		}
		return nil
	}

	for i := 0; i < nodes; i++ {
		status := []string{"online", "online", "online", "offline"}[rng.Intn(4)]
		if err := ins("nodes", map[string]any{
			"id": fmt.Sprintf("node-%04d", i), "name": fmt.Sprintf("perf-node-%03d", i),
			"address": fmt.Sprintf("10.0.%d.%d", i/250, i%250+1), "status": status,
			"joined_at": ts(48 * time.Hour), "last_seen_at": ts(time.Duration(rng.Intn(600)) * time.Second),
		}); err != nil {
			return err
		}
	}
	for i := 0; i < apps; i++ {
		if err := ins("desired_services", map[string]any{
			"name": fmt.Sprintf("perf-app-%04d", i), "image": fmt.Sprintf("registry.invalid/perf/app-%d:latest", i),
			"port": 3000 + i%100, "updated_at": ts(time.Duration(rng.Intn(100000)) * time.Second),
		}); err != nil {
			return err
		}
	}
	for i := 0; i < dbs; i++ {
		if err := ins("desired_databases", map[string]any{
			"name": fmt.Sprintf("perf-db-%04d", i), "engine": []string{"postgres", "redis"}[i%2], "version": "16",
			"updated_at": ts(time.Duration(rng.Intn(100000)) * time.Second),
		}); err != nil {
			return err
		}
	}
	for i := 0; i < deploys; i++ {
		started := time.Duration(rng.Intn(30*24*3600)+60) * time.Second
		status := []string{"succeeded", "succeeded", "succeeded", "failed"}[rng.Intn(4)]
		vals := map[string]any{
			"id": fmt.Sprintf("dpl_perf%06d", i), "service_name": fmt.Sprintf("perf-app-%04d", rng.Intn(apps)),
			"image": "registry.invalid/perf/app:latest", "status": status,
			"started_at": ts(started), "finished_at": ts(started - 40*time.Second),
		}
		if status == "failed" {
			vals["error"] = "seeded failure"
		}
		if err := ins("deploy_attempts", vals); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("seeded %d apps, %d databases, %d nodes, %d deploys into %s\n", apps, dbs, nodes, deploys, path)
	return nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) ([]column, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []column
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
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
