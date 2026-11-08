package stressprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const valid = `
name: t
seed: 7
apps: 10
nodes: 2
duration: 30s
thresholds:
  p95_ms: 100
  p99_ms: 200
  max_rss_mb: 500
  max_goroutines: 5000
  max_sqlite_mb: 100
  max_reconcile_pass_ms: 5000
  max_error_rate_pct: 1
`

func TestParseValid(t *testing.T) {
	p, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if p.Apps != 10 || p.Duration != 30*time.Second || p.Thresholds.P99Ms != 200 {
		t.Fatalf("unexpected profile: %+v", p)
	}
	s, c, l := p.PhaseSplit()
	if s+c+l != p.Duration {
		t.Fatalf("phases %v %v %v do not sum to %v", s, c, l, p.Duration)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"unknown key":      strings.Replace(valid, "apps: 10", "apps: 10\nappz: 1", 1),
		"zero apps":        strings.Replace(valid, "apps: 10", "apps: 0", 1),
		"short duration":   strings.Replace(valid, "30s", "1s", 1),
		"missing p95":      strings.Replace(valid, "p95_ms: 100", "p95_ms: 0", 1),
		"p99 below p95":    strings.Replace(valid, "p99_ms: 200", "p99_ms: 50", 1),
		"negative clients": strings.Replace(valid, "nodes: 2", "nodes: 2\nlist_clients: -1", 1),
		"bad error rate":   strings.Replace(valid, "max_error_rate_pct: 1", "max_error_rate_pct: 101", 1),
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoadUsesEnvFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvProfile, path)
	if _, err := Load(""); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvProfile, "")
	if _, err := Load(""); err == nil {
		t.Fatal("expected an error with no path")
	}
}

func TestShippedProfilesValid(t *testing.T) {
	files, err := filepath.Glob("../../stress/profiles/*.yaml")
	if err != nil || len(files) < 3 {
		t.Fatalf("expected the shipped profiles, got %v (%v)", files, err)
	}
	for _, f := range files {
		if _, err := Load(f); err != nil {
			t.Errorf("%v", err)
		}
	}
}
