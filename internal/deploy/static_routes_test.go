package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func deployRouted(t *testing.T, p *Pipeline, svc spec.Service, files map[string]string, sha string) (string, []string, error) {
	t.Helper()
	src := t.TempDir()
	writeTree(t, src, files)
	var logs []string
	dir, err := p.Deploy(context.Background(), Request{ServiceName: "docs", Service: svc, SourceDir: src, CommitSHA: sha}, func(e build.ProgressEvent) {
		if e.Log != "" {
			logs = append(logs, e.Log)
		}
	})
	return dir, logs, err
}

func routedService() spec.Service {
	s := staticService("docs.example.com")
	s.Build.Path = ""
	return s
}

func TestPipeline_DeployStatic_CompilesRoutingRulesBesideTree(t *testing.T) {
	root := t.TempDir()
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{}), WithStaticRootDir(root))
	svc := routedService()
	svc.Routes = []spec.Route{{From: "/spec", To: "/", Status: 302}}
	svc.Static = &spec.StaticOptions{SPA: true}

	dir, logs, err := deployRouted(t, p, svc, map[string]string{
		"index.html": "x",
		"_redirects": "/old /new 301\nbroken line here now\n",
		"_headers":   "/*\n  X-A: b\n",
	}, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_redirects")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("_redirects must not stay in the served tree")
	}
	if _, err := os.Stat(filepath.Join(dir, "_headers")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("_headers must not stay in the served tree")
	}
	rs, err := staticroutes.LoadSidecar(dir)
	if err != nil || rs == nil {
		t.Fatalf("sidecar: %v %v", rs, err)
	}
	if len(rs.Rules) != 2 || rs.Rules[0].Source != "app.yaml routes[0]" || rs.Rules[1].Source != "_redirects:1" || len(rs.Headers) != 1 || !rs.Options.SPA {
		t.Fatalf("ruleset = %+v", rs)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "_redirects:2") || !strings.Contains(joined, "line skipped") || !strings.Contains(joined, "2 rules, 1 header blocks, 1 problems") {
		t.Fatalf("deploy log should report the problem and the summary:\n%s", joined)
	}
}

func TestPipeline_DeployStatic_NoRoutingFilesStillWritesSidecar(t *testing.T) {
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{}), WithStaticRootDir(t.TempDir()))
	dir, _, err := deployRouted(t, p, routedService(), map[string]string{"index.html": "x"}, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if rs, err := staticroutes.LoadSidecar(dir); err != nil || rs == nil || len(rs.Rules) != 0 {
		t.Fatalf("every static deploy gets a ruleset so 404.html works: %v %v", rs, err)
	}
}

func TestPipeline_DeployStatic_RuleCapFailsDeployBeforeCommit(t *testing.T) {
	store := &fakeStaticSiteStore{}
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(store), WithStaticRootDir(t.TempDir()),
		WithStaticRouteLimits(staticroutes.Limits{MaxRules: 1, MaxHeaderBlocks: 5, MaxFileBytes: 1000, MaxValueLen: 100}))
	_, _, err := deployRouted(t, p, routedService(), map[string]string{"index.html": "x", "_redirects": "/a /b\n/c /d\n"}, "c1")
	if err == nil || !strings.Contains(err.Error(), "over the cap") {
		t.Fatalf("err = %v", err)
	}
	if store.saveCalls != 0 {
		t.Fatal("a failed rules compile must not publish the site")
	}
}

func TestPipeline_DeployStatic_FailedPublishKeepsPreviousRulesServing(t *testing.T) {
	root := t.TempDir()
	store := &fakeStaticSiteStore{}
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(store), WithStaticRootDir(root))

	oldDir, _, err := deployRouted(t, p, routedService(), map[string]string{"index.html": "v1", "_redirects": "/a /one\n"}, "c1")
	if err != nil {
		t.Fatal(err)
	}
	store.saveErr = errors.New("db down")
	if _, _, err := deployRouted(t, p, routedService(), map[string]string{"index.html": "v2", "_redirects": "/a /two\n"}, "c2"); err == nil {
		t.Fatal("want the save failure to fail the deploy")
	}
	if store.saved.RootDir == oldDir {
		t.Fatal("the failed deploy must not have been recorded as the served site")
	}
	rs, err := staticroutes.LoadSidecar(oldDir)
	if err != nil || rs == nil || rs.Rules[0].To != "/one" {
		t.Fatalf("the live deploy's rules must be untouched: %+v %v", rs, err)
	}
	if got := readTree(t, oldDir)["index.html"]; got != "v1" {
		t.Fatalf("the live tree changed: %q", got)
	}
}

func TestPipeline_DeployStatic_RollbackTargetKeepsItsOwnRules(t *testing.T) {
	root := t.TempDir()
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{}), WithStaticRootDir(root))
	d1, _, err := deployRouted(t, p, routedService(), map[string]string{"index.html": "v1", "_redirects": "/a /one\n"}, "c1")
	if err != nil {
		t.Fatal(err)
	}
	d2, _, err := deployRouted(t, p, routedService(), map[string]string{"index.html": "v2", "_redirects": "/a /two\n"}, "c2")
	if err != nil {
		t.Fatal(err)
	}
	r1, _ := staticroutes.LoadSidecar(d1)
	r2, _ := staticroutes.LoadSidecar(d2)
	if r1.Rules[0].To != "/one" || r2.Rules[0].To != "/two" {
		t.Fatalf("each deploy must keep its own rules so a rollback restores them: %v %v", r1.Rules[0].To, r2.Rules[0].To)
	}
}

func TestPipeline_DeployStatic_OversizedRulesFileRejected(t *testing.T) {
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{}), WithStaticRootDir(t.TempDir()),
		WithStaticRouteLimits(staticroutes.Limits{MaxRules: 10, MaxHeaderBlocks: 5, MaxFileBytes: 20, MaxValueLen: 100}))
	_, _, err := deployRouted(t, p, routedService(), map[string]string{"index.html": "x", "_headers": strings.Repeat("/a\n  X-A: b\n", 10)}, "c1")
	if err == nil || !strings.Contains(err.Error(), "byte cap") {
		t.Fatalf("err = %v", err)
	}
}
