package iac

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// liveState mirrors the fake's maps. Its fields are exported so encoding/json
// can read them; the fake's own fields are unexported.
type liveState struct {
	Projects  map[string]map[string]any            `json:"projects"`
	Envs      map[string]map[string]any            `json:"envs"`
	EnvVars   map[string]map[string]any            `json:"envVars"`
	Tags      map[string]map[string]any            `json:"tags"`
	Dbs       map[string]map[string]any            `json:"dbs"`
	Apps      map[string]map[string]any            `json:"apps"`
	Secrets   map[string]map[string]bool           `json:"secrets"`
	Lbs       map[string]map[string]any            `json:"lbs"`
	Pipelines map[string]map[string]map[string]any `json:"pipelines"`
	Alerts    map[string]map[string]map[string]any `json:"alerts"`
	Jobs      map[string]map[string]map[string]any `json:"jobs"`
}

func loadLive(f *fakeCP, raw []byte) error {
	var l liveState
	if err := json.Unmarshal(raw, &l); err != nil {
		return err
	}
	for dst, src := range map[*map[string]map[string]any]map[string]map[string]any{
		&f.projects: l.Projects, &f.envs: l.Envs, &f.envVars: l.EnvVars, &f.tags: l.Tags,
		&f.dbs: l.Dbs, &f.apps: l.Apps, &f.lbs: l.Lbs,
	} {
		if src != nil {
			*dst = src
		}
	}
	if l.Secrets != nil {
		f.secrets = l.Secrets
	}
	if l.Pipelines != nil {
		f.pipelines = l.Pipelines
	}
	if l.Alerts != nil {
		f.alerts = l.Alerts
	}
	if l.Jobs != nil {
		f.jobs = l.Jobs
	}
	return nil
}

// goldenOptions is the optional options.json of a golden case.
type goldenOptions struct {
	Source  string            `json:"source"`
	Prune   bool              `json:"prune"`
	Secrets map[string]string `json:"secrets"`
	Ledger  *goldenLedger     `json:"ledger"`
}

// goldenLedger runs the case as a stack apply with this ownership view.
type goldenLedger struct {
	Stack          string            `json:"stack"`
	ScopeProject   string            `json:"scopeProject"`
	Owned          []LedgerEntry     `json:"owned"`
	Foreign        map[string]string `json:"foreign"`
	Adopt          bool              `json:"adopt"`
	AllowMassPrune bool              `json:"allowMassPrune"`
	MaxPercent     int               `json:"maxPercent"`
	MaxCount       int               `json:"maxCount"`
}

func (g *goldenLedger) context() context.Context {
	if g == nil {
		return context.Background()
	}
	l := NewLedger(g.Stack, g.Stack, g.ScopeProject, g.Owned)
	for k, v := range g.Foreign {
		l.Foreign[k] = v
	}
	l.Adopt, l.AllowMassPrune = g.Adopt, g.AllowMassPrune
	l.Guard = PruneGuard{MaxPercent: g.MaxPercent, MaxCount: g.MaxCount}
	return WithLedger(context.Background(), l)
}

// TestGoldenPlans pins the plan for each fixture under testdata/golden. The
// live state is loaded into the same fake control plane the other tests use,
// so live.json has the fake's own shape (projects, apps, tags, ...).
func TestGoldenPlans(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no golden cases")
	}
	for _, dir := range dirs {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) { runGolden(t, dir) })
	}
}

func runGolden(t *testing.T, dir string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "docs.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{}
	var gopts goldenOptions
	if raw, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "options.json"))); err == nil {
		if err := json.Unmarshal(raw, &gopts); err != nil {
			t.Fatalf("options.json: %v", err)
		}
		opts = Options{Source: gopts.Source, Prune: gopts.Prune, Secrets: gopts.Secrets}
	}

	f := newFake()
	if raw, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "live.json"))); err == nil {
		if err := loadLive(f, raw); err != nil {
			t.Fatalf("live.json: %v", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	docs, issues := ParseDocuments([]Source{{Name: "docs.yaml", Data: src}})
	if len(issues) > 0 {
		t.Fatalf("parse issues: %v", issues)
	}
	res, issues := Build(docs, opts)
	if len(issues) > 0 {
		t.Fatalf("build issues: %v", issues)
	}
	st, err := Load(gopts.Ledger.context(), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanFor(st, res, opts)
	if f.writeCount() != 0 {
		t.Fatalf("planning wrote: %v", f.writes)
	}
	normalizePlan(&plan)

	got, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	for _, v := range gopts.Secrets {
		if bytes.Contains(got, []byte(v)) {
			t.Fatalf("secret value leaked into plan output")
		}
	}

	golden := filepath.Join(dir, "plan.golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(filepath.Clean(golden))
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("plan differs from %s (run with UPDATE_GOLDEN=1 to accept)\n--- got\n%s", golden, got)
	}
}

// normalizePlan sorts the per-change lists so the golden does not depend on
// map iteration order inside the planner.
func normalizePlan(p *Plan) {
	for i := range p.Changes {
		c := &p.Changes[i]
		sort.SliceStable(c.Fields, func(a, b int) bool { return c.Fields[a].Path < c.Fields[b].Path })
		sort.SliceStable(c.Kept, func(a, b int) bool { return c.Kept[a].Path < c.Kept[b].Path })
		sort.Strings(c.Warnings)
	}
}
