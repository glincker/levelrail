package stressseed

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedOnce(t *testing.T, cfg Config) Manifest {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m, err := Seed(ctx, db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var apps, domains int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM desired_services").Scan(&apps); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM service_domains").Scan(&domains); err != nil {
		t.Fatal(err)
	}
	if apps != cfg.Apps || domains != cfg.Apps*cfg.DomainsPerApp {
		t.Fatalf("got %d apps and %d domains", apps, domains)
	}
	return m
}

func TestSeedIsDeterministic(t *testing.T) {
	cfg := Config{Apps: 12, Databases: 3, Nodes: 2, Deploys: 40, EnvsPerApp: 3, DomainsPerApp: 2, Seed: 9,
		Anchor: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), NodeIDs: []string{"a", "b"}}
	a, b := seedOnce(t, cfg), seedOnce(t, cfg)
	if a.Digest != b.Digest || len(a.Apps) != 12 || a.Deploys != 40 {
		t.Fatalf("same seed must give the same rows: %q vs %q", a.Digest, b.Digest)
	}
	cfg.Seed = 10
	if c := seedOnce(t, cfg); c.Digest == a.Digest {
		t.Fatal("a different seed must change the rows")
	}
}
