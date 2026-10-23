package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

type fakeExtractor struct {
	files map[string]string
	err   error
	calls int
	tag   string
	src   string
}

func (f *fakeExtractor) ExtractDir(_ context.Context, tag, srcPath, destDir string) error {
	f.calls++
	f.tag, f.src = tag, srcPath
	if f.err != nil {
		return f.err
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return err
	}
	for rel, body := range f.files {
		p := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func builtService() spec.Service {
	return spec.Service{
		Build:   spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "dist", Args: map[string]string{"VITE_API": "x"}},
		Domains: []string{"docs.example.com"},
	}
}

func newBuiltPipeline(builder *fakeBuilder, st *fakeStaticSiteStore, ex *fakeExtractor, root string) *Pipeline {
	return New(builder, &fakeServiceStore{}, WithStaticSiteStore(st), WithStaticRootDir(root), WithStaticExtractor(ex))
}

func TestDeployStaticBuilt_Success(t *testing.T) {
	builder, st := &fakeBuilder{}, &fakeStaticSiteStore{}
	ex := &fakeExtractor{files: map[string]string{"index.html": "<h1>built</h1>", "assets/a.js": "1"}}
	root := t.TempDir()
	src := t.TempDir()
	writeTree(t, src, map[string]string{"package.json": "{}", "package-lock.json": "{}"})

	dir, err := newBuiltPipeline(builder, st, ex, root).Deploy(context.Background(), Request{
		ServiceName: "docs", Service: builtService(), SourceDir: src, CommitSHA: "abc1234",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if want := filepath.Join(root, "docs", "abc1234"); dir != want || st.saved.RootDir != want {
		t.Fatalf("dir = %q saved = %q, want %q", dir, st.saved.RootDir, want)
	}
	if got := readTree(t, dir); got["index.html"] != "<h1>built</h1>" || got["assets/a.js"] != "1" {
		t.Fatalf("published tree = %v", got)
	}
	if builder.calls != 1 || builder.lastReq.BuildArgs["VITE_API"] != "x" || ex.src != "/out" {
		t.Fatalf("builder calls = %d args = %v extract src = %q", builder.calls, builder.lastReq.BuildArgs, ex.src)
	}
	if _, err := os.Stat(filepath.Join(root, staticStagingDir)); err == nil {
		entries, _ := os.ReadDir(filepath.Join(root, staticStagingDir))
		if len(entries) != 0 {
			t.Fatalf("staging not cleaned: %v", entries)
		}
	}
}

func TestDeployStaticBuilt_Failures(t *testing.T) {
	tests := []struct {
		name      string
		builder   *fakeBuilder
		ex        *fakeExtractor
		wantSaves int
		wantErr   string
	}{
		{"build fails", &fakeBuilder{err: errors.New("exit 1")}, &fakeExtractor{files: map[string]string{"i.html": "x"}}, 0, "static build"},
		{"half succeeded: extract fails", &fakeBuilder{}, &fakeExtractor{err: errors.New("no such path")}, 0, "collect build output"},
		{"half succeeded: empty output", &fakeBuilder{}, &fakeExtractor{files: map[string]string{}}, 0, "no files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStaticSiteStore{}
			root := t.TempDir()
			prev := filepath.Join(root, "docs", "old")
			writeTree(t, prev, map[string]string{"index.html": "live"})

			_, err := newBuiltPipeline(tt.builder, st, tt.ex, root).Deploy(context.Background(), Request{
				ServiceName: "docs", Service: builtService(), SourceDir: t.TempDir(), CommitSHA: "new",
			}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if st.saveCalls != tt.wantSaves {
				t.Fatalf("saves = %d", st.saveCalls)
			}
			if got := readTree(t, prev); got["index.html"] != "live" {
				t.Fatalf("previous build changed: %v", got)
			}
			if _, err := os.Stat(filepath.Join(root, "docs", "new")); err == nil {
				t.Fatal("failed deploy left a published directory")
			}
			if entries, _ := os.ReadDir(filepath.Join(root, staticStagingDir)); len(entries) != 0 {
				t.Fatalf("staging not cleaned: %v", entries)
			}
		})
	}
}

func TestDeployStaticBuilt_SaveFailureKeepsPreviousRoute(t *testing.T) {
	st := &fakeStaticSiteStore{saveErr: errors.New("db down")}
	ex := &fakeExtractor{files: map[string]string{"index.html": "x"}}
	_, err := newBuiltPipeline(&fakeBuilder{}, st, ex, t.TempDir()).Deploy(context.Background(), Request{
		ServiceName: "docs", Service: builtService(), SourceDir: t.TempDir(), CommitSHA: "c1",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "save static site") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeployStaticBuilt_RequiresExtractor(t *testing.T) {
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{}), WithStaticRootDir(t.TempDir()))
	_, err := p.Deploy(context.Background(), Request{ServiceName: "docs", Service: builtService(), SourceDir: t.TempDir(), CommitSHA: "c"}, nil)
	if err == nil || !strings.Contains(err.Error(), "WithStaticExtractor") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeployStaticBuilt_RejectsInvalidSpec(t *testing.T) {
	svc := builtService()
	svc.Build.Publish = "../escape"
	ex := &fakeExtractor{}
	_, err := newBuiltPipeline(&fakeBuilder{}, &fakeStaticSiteStore{}, ex, t.TempDir()).Deploy(context.Background(), Request{
		ServiceName: "docs", Service: svc, SourceDir: t.TempDir(), CommitSHA: "c",
	}, nil)
	if err == nil || ex.calls != 0 {
		t.Fatalf("err = %v extract calls = %d", err, ex.calls)
	}
}

func TestStaticDockerfile(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
		deny  []string
	}{
		{"npm lockfile", map[string]string{"package.json": "{}", "package-lock.json": "{}"}, []string{"RUN npm ci", `COPY ["package.json","package-lock.json","./"]`, "RUN npm run build", `COPY --from=build ["/src/dist","/out/"]`, "ARG VITE_API"}, nil},
		{"pnpm lockfile", map[string]string{"package.json": "{}", "pnpm-lock.yaml": "x"}, []string{"pnpm install --frozen-lockfile"}, []string{"npm ci"}},
		{"yarn lockfile", map[string]string{"package.json": "{}", "yarn.lock": "x"}, []string{"yarn install --frozen-lockfile"}, nil},
		{"no lockfile", map[string]string{"package.json": "{}"}, []string{"RUN npm install"}, nil},
		{"no package.json", map[string]string{"build.sh": "x"}, []string{"COPY . ."}, []string{"npm "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir, tt.files)
			b := spec.Build{Type: spec.BuildStatic, Command: "npm run build", Publish: "dist/", Args: map[string]string{"VITE_API": "x"}}
			got, err := staticDockerfile(dir, "node:22", b)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if tt.name != "no package.json" && !strings.Contains(got, w) && !strings.HasPrefix(w, "ARG") {
					t.Errorf("missing %q in\n%s", w, got)
				}
			}
			for _, d := range tt.deny {
				if strings.Contains(strings.ReplaceAll(got, "npm run build", ""), d) {
					t.Errorf("unexpected %q in\n%s", d, got)
				}
			}
		})
	}
}
