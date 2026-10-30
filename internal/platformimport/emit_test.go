package platformimport

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/platformimport/fakeheroku"
	"github.com/GLINCKER/levelrail/internal/platformimport/fakerender"
	"github.com/GLINCKER/levelrail/internal/platformimport/fakevercel"
)

var updateEmit = os.Getenv("PLATFORMIMPORT_UPDATE") != ""

func emitCases(t *testing.T) map[string]*Discovery {
	t.Helper()
	cases := map[string]*Discovery{}
	for name, h := range map[string]struct {
		handler func() *httptest.Server
		build   func(url string) (Source, error)
	}{
		"heroku": {func() *httptest.Server { return httptest.NewServer(fakeheroku.New().Handler()) },
			func(u string) (Source, error) { return NewHeroku(u, fakeheroku.Token, testOpts) }},
		"vercel": {func() *httptest.Server { return httptest.NewServer(fakevercel.New().Handler()) },
			func(u string) (Source, error) { return NewVercel(u, fakevercel.Token, testOpts) }},
		"render": {func() *httptest.Server { return httptest.NewServer(fakerender.New().Handler()) },
			func(u string) (Source, error) { return NewRender(u, fakerender.Token, testOpts) }},
	} {
		srv := h.handler()
		t.Cleanup(srv.Close)
		src, err := h.build(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		d, err := src.Discover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		cases[name] = d
	}
	return cases
}

func emitFor(t *testing.T, d *Discovery) []EmitFile {
	t.Helper()
	in := BuildPlan(d, PlanOptions{Collision: CollisionSuffix, DBVersions: map[string]string{"postgres": "16", "redis": "7.4"}}).EmitInput()
	files, err := EmitStack(in, EmitOptions{Stack: string(d.Platform) + "-import", Project: string(d.Platform) + "-import", Environment: "production", ImageRepo: "registry.example.com/acme"})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// validateEmitted runs the same loader and schema checks apply uses.
func validateEmitted(t *testing.T, files []EmitFile) []*iac.Resource {
	t.Helper()
	var srcs []iac.Source
	for _, f := range files {
		if !strings.HasSuffix(f.Name, ".yaml") || f.Name == EmitStackFile {
			continue
		}
		srcs = append(srcs, iac.Source{Name: f.Name, Data: f.Content})
	}
	docs, issues := iac.ParseDocuments(srcs)
	if len(issues) > 0 {
		t.Fatalf("parse issues: %v", issues)
	}
	res, issues := iac.Build(docs, iac.Options{})
	if len(issues) > 0 {
		t.Fatalf("emitted documents do not validate: %v", issues)
	}
	return res
}

func TestEmitStackGolden(t *testing.T) {
	for name, d := range emitCases(t) {
		t.Run(name, func(t *testing.T) {
			files := emitFor(t, d)
			validateEmitted(t, files)
			dir := filepath.Join("testdata", "emit", name)
			if updateEmit {
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range files {
				p := filepath.Join(dir, f.Name)
				if updateEmit {
					if err := os.WriteFile(p, f.Content, 0o600); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(p) //nolint:gosec // fixture path from testdata
				if err != nil {
					t.Fatalf("%v (set PLATFORMIMPORT_UPDATE=1 to write goldens)", err)
				}
				if !bytes.Equal(want, f.Content) {
					t.Errorf("%s differs from golden:\n%s", f.Name, f.Content)
				}
			}
		})
	}
}

func TestEmitStackDeterministic(t *testing.T) {
	for name, d := range emitCases(t) {
		a, b := emitFor(t, d), emitFor(t, d)
		for i := range a {
			if !bytes.Equal(a[i].Content, b[i].Content) {
				t.Errorf("%s: %s differs between runs", name, a[i].Name)
			}
		}
	}
}

func TestEmitNeverWritesSecretValues(t *testing.T) {
	for name, d := range emitCases(t) {
		var values []string
		for _, a := range d.Apps {
			for _, e := range a.Env {
				if e.Secret && len(e.Value) >= 6 {
					values = append(values, e.Value)
				}
			}
		}
		for _, f := range emitFor(t, d) {
			for _, v := range values {
				if strings.Contains(string(f.Content), v) {
					t.Errorf("%s: %s contains a secret value", name, f.Name)
				}
			}
		}
	}
}

func TestEmitJobsAndAmbiguousConnections(t *testing.T) {
	p := &Plan{
		Apps: []AppPlan{{Name: "web", Image: "ghcr.io/acme/web:1", Port: 3000, Replicas: 1,
			Secrets: map[string]string{"DATABASE_URL": "x", "REDIS_URL": "y"},
			Crons: []CronPlan{{Name: "nightly", Schedule: "0 3 * * *", Command: "node nightly.js"},
				{Name: "leaky", Schedule: "0 4 * * *", Command: "curl -H 'Authorization: Bearer abcdefghijklmnop1234' https://x"}}}},
		Databases: []DatabasePlan{{Name: "a", Engine: "postgres"}, {Name: "b", Engine: "postgres"}, {Name: "cache", Engine: "redis"}},
	}
	files, err := EmitStack(p.EmitInput(), EmitOptions{Stack: "s", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	validateEmitted(t, files)
	all := map[string]string{}
	for _, f := range files {
		all[f.Name] = string(f.Content)
	}
	apps := all[EmitAppsFile]
	for _, want := range []string{"DATABASE_URL: {secretRef: DATABASE_URL}", "REDIS_URL: {from: {database: cache, field: url}}", "name: nightly", "command: [sh, -c, node nightly.js]"} {
		if !strings.Contains(apps, want) {
			t.Errorf("apps.yaml lacks %q:\n%s", want, apps)
		}
	}
	if strings.Contains(apps, "leaky") || strings.Contains(apps, "abcdefghijklmnop1234") || !strings.Contains(all[EmitNotesFile], "leaky") {
		t.Errorf("a job whose command looks like a credential must stay a note:\n%s\n%s", apps, all[EmitNotesFile])
	}
}

func TestEmitNeedsImageRepoForBuiltApps(t *testing.T) {
	d := emitCases(t)["heroku"]
	in := BuildPlan(d, PlanOptions{}).EmitInput()
	_, err := EmitStack(in, EmitOptions{Stack: "s", Project: "p"})
	if err == nil || !strings.Contains(err.Error(), "--image-repo") {
		t.Fatalf("want an --image-repo error, got %v", err)
	}
}
