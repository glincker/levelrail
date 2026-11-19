package deploy

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type settingsStaticStore struct {
	fakeStaticSiteStore
	set store.StaticSiteSettings
}

func (s *settingsStaticStore) GetStaticSiteSettings(context.Context, string) (store.StaticSiteSettings, store.StaticUIRules, error) {
	return s.set, store.StaticUIRules{}, nil
}

func TestDeployStaticSettingsOverrideBuild(t *testing.T) {
	st := &settingsStaticStore{set: store.StaticSiteSettings{BuildCommand: "make site", OutputDir: "public"}}
	builder, ex := &fakeBuilder{}, &fakeExtractor{files: map[string]string{"index.html": "x"}}
	root, src := t.TempDir(), t.TempDir()
	p := New(builder, &fakeServiceStore{}, WithStaticSiteStore(st), WithStaticRootDir(root), WithStaticExtractor(ex))
	if _, err := p.Deploy(context.Background(), Request{ServiceName: "docs", Service: builtService(), SourceDir: src, CommitSHA: "abc1234"}, nil); err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if builder.calls != 1 || !strings.Contains(builder.lastReq.DockerfilePath, "Dockerfile") {
		t.Fatalf("builder calls = %d", builder.calls)
	}
}

func TestApplyStaticSiteSettings(t *testing.T) {
	tests := []struct {
		name    string
		set     store.StaticSiteSettings
		build   spec.Build
		want    spec.Build
		wantErr bool
	}{
		{"output dir picks an upload subfolder", store.StaticSiteSettings{OutputDir: "site"}, spec.Build{Type: spec.BuildStatic}, spec.Build{Type: spec.BuildStatic, Path: "site"}, false},
		{"output dir replaces the detected publish", store.StaticSiteSettings{OutputDir: "out"}, spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "dist"}, spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "out"}, false},
		{"command replaces the detected one", store.StaticSiteSettings{BuildCommand: "pnpm build"}, spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "dist"}, spec.Build{Type: spec.BuildStatic, Command: "pnpm build", Publish: "dist"}, false},
		{"command reuses the served path as publish", store.StaticSiteSettings{BuildCommand: "make"}, spec.Build{Type: spec.BuildStatic, Path: "dist"}, spec.Build{Type: spec.BuildStatic, Command: "make", Publish: "dist"}, false},
		{"no site setting keeps the git source build config", store.StaticSiteSettings{}, spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "build"}, spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "build"}, false},
		{"site setting wins over the git source build config", store.StaticSiteSettings{BuildCommand: "make", OutputDir: "out"}, spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "build"}, spec.Build{Type: spec.BuildStatic, Command: "make", Publish: "out"}, false},
		{"command without any output dir fails", store.StaticSiteSettings{BuildCommand: "make"}, spec.Build{Type: spec.BuildStatic}, spec.Build{}, true},
		{"escaping output dir fails", store.StaticSiteSettings{OutputDir: "../x"}, spec.Build{Type: spec.BuildStatic}, spec.Build{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&settingsStaticStore{set: tc.set}), WithStaticRootDir(filepath.Join(t.TempDir(), "r")))
			req := Request{ServiceName: "docs", Service: spec.Service{Build: tc.build}}
			err := p.applyStaticSiteSettings(context.Background(), &req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && (req.Service.Build.Command != tc.want.Command || req.Service.Build.Publish != tc.want.Publish || req.Service.Build.Path != tc.want.Path) {
				t.Fatalf("build = %+v, want %+v", req.Service.Build, tc.want)
			}
		})
	}
}

func TestDeployStaticSettingsOutputDirSelectsUploadSubfolder(t *testing.T) {
	st := &settingsStaticStore{set: store.StaticSiteSettings{OutputDir: "site"}}
	root, src := t.TempDir(), t.TempDir()
	writeTree(t, src, map[string]string{"site/index.html": "<h1>in</h1>", "other/x.html": "no"})
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(st), WithStaticRootDir(root))
	dir, err := p.Deploy(context.Background(), Request{ServiceName: "docs", Service: spec.Service{Build: spec.Build{Type: spec.BuildStatic}}, SourceDir: src, CommitSHA: "abc1234"}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if got := readTree(t, dir); got["index.html"] != "<h1>in</h1>" || got["other/x.html"] != "" {
		t.Fatalf("published tree = %v", got)
	}
}
