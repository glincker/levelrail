// Command perf-fixtures seeds a scratch control plane data dir with a
// large, deterministic resource graph for UI performance measurement.
// Dev tooling only: never imported by the control plane and it refuses
// a data dir that already holds a database. The graph itself lives in
// internal/stressseed.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/stressseed"
)

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
	cfg := stressseed.Config{Apps: *apps, Databases: *dbs, Nodes: *nodes, Deploys: *deploys, Seed: *seed}
	if err := run(*dir, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "perf-fixtures:", err)
		os.Exit(1)
	}
}

func run(dir string, cfg stressseed.Config) error {
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
	m, err := stressseed.Seed(ctx, db, cfg)
	if err != nil {
		return err
	}
	fmt.Printf("seeded %d apps, %d databases, %d nodes, %d deploys into %s\n", len(m.Apps), len(m.Databases), len(m.Nodes), m.Deploys, path)
	return nil
}
