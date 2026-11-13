package iac

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCompatFixtures keeps documents written for earlier releases parsing and
// planning cleanly; a failure means a contract break, not a stale fixture.
func TestCompatFixtures(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "compat", "*"))
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, dir := range dirs {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) { runCompat(t, dir) })
	}
	if ran == 0 {
		t.Fatal("no compat fixtures")
	}
}

func compatPlan(t *testing.T, dir string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "docs.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{}
	if raw, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "options.json"))); err == nil {
		var g goldenOptions
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("options.json: %v", err)
		}
		opts = Options{Source: g.Source, Prune: g.Prune, Secrets: g.Secrets}
	}
	f := newFake()
	if raw, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "live.json"))); err == nil {
		if err := loadLive(f, raw); err != nil {
			t.Fatalf("live.json: %v", err)
		}
	}
	docs, issues := ParseDocuments([]Source{{Name: "docs.yaml", Data: src}})
	if len(issues) > 0 {
		t.Fatalf("parse issues: %v", issues)
	}
	res, issues := Build(docs, opts)
	if len(issues) > 0 {
		t.Fatalf("build issues: %v", issues)
	}
	st, err := Load(context.Background(), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanFor(st, res, opts)
	if plan.Summary.Error > 0 {
		t.Fatalf("plan has %d error items", plan.Summary.Error)
	}
	normalizePlan(&plan)
	out, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func runCompat(t *testing.T, dir string) {
	SetPlanHashKey([]byte("compat-key"))
	if a, b := compatPlan(t, dir), compatPlan(t, dir); string(a) != string(b) {
		t.Fatal("plan output is not stable across runs")
	}
}
