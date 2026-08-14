package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStaticSiteStore is a hand-written fake for StaticSiteStore, the
// same pattern fakeServiceStore already establishes in deploy_test.go.
type fakeStaticSiteStore struct {
	saveErr   error
	saved     store.StaticSite
	saveCalls int
}

func (f *fakeStaticSiteStore) SaveStaticSite(_ context.Context, site store.StaticSite) error {
	f.saveCalls++
	f.saved = site
	return f.saveErr
}

func staticService(domains ...string) spec.Service {
	return spec.Service{
		Build:   spec.Build{Type: spec.BuildStatic, Path: "dist"},
		Domains: domains,
	}
}

// writeTree creates files (path -> content) under root, creating parent
// directories as needed.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", full, err)
		}
	}
}

// readTree reads every regular file under root into a map, relative
// paths as keys, so tests can assert the copied tree matches the source
// tree exactly, no more, no less.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir over a t.TempDir() this test created, not user input
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", root, err)
	}
	return out
}

func TestPipeline_DeployStatic_NoStaticSiteStoreConfigured_Errors(t *testing.T) {
	builder := &fakeBuilder{}
	p := New(builder, &fakeServiceStore{}, WithStaticRootDir(t.TempDir()))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"dist/index.html": "<h1>hi</h1>"})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want an error: no StaticSiteStore configured")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0: static deploys never build an image", builder.calls)
	}
}

func TestPipeline_DeployStatic_NoStaticRootDirConfigured_Errors(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"dist/index.html": "<h1>hi</h1>"})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want an error: no static root dir configured")
	}
	if staticStore.saveCalls != 0 {
		t.Errorf("SaveStaticSite called %d times, want 0", staticStore.saveCalls)
	}
}

func TestPipeline_DeployStatic_Success(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	rootDir := t.TempDir()
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(rootDir))

	sourceDir := t.TempDir()
	files := map[string]string{
		"dist/index.html":       "<h1>hi</h1>",
		"dist/assets/style.css": "body { color: red; }",
	}
	writeTree(t, sourceDir, files)
	// A file outside dist/ (Build.Path) must never be copied: only the
	// declared static root, matching how a Dockerfile build only ever
	// reads what its own build context declares.
	writeTree(t, sourceDir, map[string]string{"README.md": "not part of the site"})

	tag, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com", "www.docs.example.com"),
		SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	wantDest := filepath.Join(rootDir, "docs", "abc1234")
	if tag != wantDest {
		t.Errorf("tag = %q, want %q (the directory now being served)", tag, wantDest)
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0: static deploys never build an image", builder.calls)
	}

	got := readTree(t, wantDest)
	want := map[string]string{
		"index.html":       "<h1>hi</h1>",
		"assets/style.css": "body { color: red; }",
	}
	if len(got) != len(want) {
		t.Fatalf("copied tree = %v, want %v", got, want)
	}
	for rel, content := range want {
		if got[rel] != content {
			t.Errorf("copied file %q = %q, want %q", rel, got[rel], content)
		}
	}

	if staticStore.saveCalls != 1 {
		t.Fatalf("SaveStaticSite called %d times, want 1", staticStore.saveCalls)
	}
	if staticStore.saved.Name != "docs" {
		t.Errorf("saved.Name = %q, want docs", staticStore.saved.Name)
	}
	if staticStore.saved.RootDir != wantDest {
		t.Errorf("saved.RootDir = %q, want %q", staticStore.saved.RootDir, wantDest)
	}
	wantDomains := []string{"docs.example.com", "www.docs.example.com"}
	if len(staticStore.saved.Domains) != len(wantDomains) {
		t.Fatalf("saved.Domains = %v, want %v", staticStore.saved.Domains, wantDomains)
	}
	for i, d := range wantDomains {
		if staticStore.saved.Domains[i] != d {
			t.Errorf("saved.Domains[%d] = %q, want %q", i, staticStore.saved.Domains[i], d)
		}
	}
}

func TestPipeline_DeployStatic_NoBuildPath_ServesSourceDirRoot(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	rootDir := t.TempDir()
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(rootDir))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"index.html": "<h1>root</h1>"})

	svc := staticService("docs.example.com")
	svc.Build.Path = "" // no build.path: serve the checkout root as-is

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: svc, SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	wantDest := filepath.Join(rootDir, "docs", "abc1234")
	got := readTree(t, wantDest)
	if got["index.html"] != "<h1>root</h1>" {
		t.Errorf("copied tree = %v, want index.html at the root", got)
	}
}

func TestPipeline_DeployStatic_SourceDirMissing_Errors(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(t.TempDir()))

	sourceDir := t.TempDir() // exists, but has no "dist" subdirectory

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want an error: build.path does not exist in the checkout")
	}
	if staticStore.saveCalls != 0 {
		t.Errorf("SaveStaticSite called %d times, want 0: a missing source dir must never reach the store", staticStore.saveCalls)
	}
}

func TestPipeline_DeployStatic_SourcePathIsAFile_Errors(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(t.TempDir()))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"dist": "this is a file, not a directory"})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want an error: build.path resolves to a file, not a directory")
	}
}

func TestPipeline_DeployStatic_SaveFailure_Propagates(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{saveErr: errors.New("disk full")}
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(t.TempDir()))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"dist/index.html": "<h1>hi</h1>"})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the save error to propagate")
	}
}

func TestPipeline_DeployStatic_Redeploy_OverwritesStaleFiles(t *testing.T) {
	// Same service, same commit SHA (a re-triggered build of the exact
	// same commit) must not leave a file behind that the newer source
	// tree no longer has: copyStaticDir must clear the destination
	// first, not merge on top of whatever's already there.
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	rootDir := t.TempDir()
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(rootDir))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{
		"dist/index.html": "v1",
		"dist/stale.html": "will be removed before the second deploy",
	})
	req := Request{ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234"}
	if _, err := p.Deploy(context.Background(), req, nil); err != nil {
		t.Fatalf("first Deploy() error = %v", err)
	}

	if err := os.Remove(filepath.Join(sourceDir, "dist", "stale.html")); err != nil {
		t.Fatalf("os.Remove() error = %v", err)
	}
	writeTree(t, sourceDir, map[string]string{"dist/index.html": "v2"})

	if _, err := p.Deploy(context.Background(), req, nil); err != nil {
		t.Fatalf("second Deploy() error = %v", err)
	}

	wantDest := filepath.Join(rootDir, "docs", "abc1234")
	got := readTree(t, wantDest)
	if _, stillThere := got["stale.html"]; stillThere {
		t.Errorf("copied tree = %v, want stale.html removed on redeploy", got)
	}
	if got["index.html"] != "v2" {
		t.Errorf("index.html = %q, want v2 (the redeploy's content)", got["index.html"])
	}
}

func TestPipeline_DeployStatic_RecordsBuildDuration(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	recorder := &fakeBuildMetricsRecorder{}
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(t.TempDir()), WithBuildMetricsRecorder(recorder))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"dist/index.html": "<h1>hi</h1>"})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("RecordBuildDuration called %d times, want 1", recorder.calls)
	}
	if recorder.lastServiceName != "docs" {
		t.Errorf("recorded service = %q, want docs", recorder.lastServiceName)
	}
	if recorder.lastDuration < 0 {
		t.Errorf("recorded duration = %v, want >= 0", recorder.lastDuration)
	}
}

// TestPipeline_DeployStatic_ProgressCallback_NeverCalled documents that
// static deploys have no build progress to forward: progress, if given,
// is simply never invoked, since there is no build.ProgressEvent-emitting
// step in this path at all.
func TestPipeline_DeployStatic_ProgressCallback_NeverCalled(t *testing.T) {
	builder := &fakeBuilder{}
	staticStore := &fakeStaticSiteStore{}
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(staticStore), WithStaticRootDir(t.TempDir()))

	sourceDir := t.TempDir()
	writeTree(t, sourceDir, map[string]string{"dist/index.html": "<h1>hi</h1>"})

	called := false
	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "docs", Service: staticService("docs.example.com"), SourceDir: sourceDir, CommitSHA: "abc1234",
	}, func(build.ProgressEvent) { called = true })
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if called {
		t.Error("progress callback was called, want it never invoked for a static deploy")
	}
}
