package build

import (
	"errors"
	"io"
	"testing"

	bkclient "github.com/moby/buildkit/client"
)

// discardWriteCloser adapts io.Discard to io.WriteCloser so newSolveOpt
// has something to hand back from its Output func without needing a real
// pipe for these pure construction tests.
type discardWriteCloser struct{ io.Writer }

func (discardWriteCloser) Close() error { return nil }

func TestNewSolveOpt(t *testing.T) {
	tests := []struct {
		name       string
		req        Request
		cacheDir   string
		wantErr    error // checked with errors.Is when set
		wantErrAny bool  // checked with a plain non-nil check when wantErr is unset
	}{
		{
			name: "plain dockerfile build",
			req:  Request{ContextDir: "testdata", Tag: "levelrail-spike:latest"},
		},
		{
			name: "with target and build args and no-cache",
			req: Request{
				ContextDir: "testdata",
				Tag:        "levelrail-spike:latest",
				Target:     "final",
				BuildArgs:  map[string]string{"FOO": "bar"},
				NoCache:    true,
			},
		},
		{
			name:    "invalid request fails before touching the filesystem",
			req:     Request{Tag: "levelrail-spike:latest"}, // no ContextDir
			wantErr: ErrContextDirRequired,
		},
		{
			name:       "nonexistent context dir",
			req:        Request{ContextDir: "testdata/does-not-exist", Tag: "levelrail-spike:latest"},
			wantErrAny: true,
		},
		{
			name:     "cache dir set: import and export both wired",
			req:      Request{ContextDir: "testdata", Tag: "levelrail-spike:latest"},
			cacheDir: "/tmp/levelrail-build-cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := discardWriteCloser{io.Discard}
			opt, err := newSolveOpt(tt.req, tt.cacheDir, out)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("newSolveOpt() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.wantErrAny {
				if err == nil {
					t.Fatalf("newSolveOpt() err = nil, want a non-nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("newSolveOpt() unexpected error: %v", err)
			}

			if opt.Frontend != "dockerfile.v0" {
				t.Errorf("Frontend = %q, want %q", opt.Frontend, "dockerfile.v0")
			}
			if opt.FrontendAttrs["filename"] != "Dockerfile" {
				t.Errorf("FrontendAttrs[filename] = %q, want %q", opt.FrontendAttrs["filename"], "Dockerfile")
			}
			if len(opt.Exports) != 1 || opt.Exports[0].Type != bkclient.ExporterDocker {
				t.Fatalf("Exports = %+v, want exactly one entry of type %q", opt.Exports, bkclient.ExporterDocker)
			}
			if opt.Exports[0].Attrs["name"] != tt.req.Tag {
				t.Errorf("Exports[0].Attrs[name] = %q, want %q", opt.Exports[0].Attrs["name"], tt.req.Tag)
			}
			if _, ok := opt.LocalMounts["context"]; !ok {
				t.Error("LocalMounts missing \"context\" mount")
			}
			if _, ok := opt.LocalMounts["dockerfile"]; !ok {
				t.Error("LocalMounts missing \"dockerfile\" mount")
			}

			if tt.req.Target != "" && opt.FrontendAttrs["target"] != tt.req.Target {
				t.Errorf("FrontendAttrs[target] = %q, want %q", opt.FrontendAttrs["target"], tt.req.Target)
			}
			if tt.req.NoCache {
				if _, ok := opt.FrontendAttrs["no-cache"]; !ok {
					t.Error("FrontendAttrs missing \"no-cache\"")
				}
			}
			for k, v := range tt.req.BuildArgs {
				if got := opt.FrontendAttrs["build-arg:"+k]; got != v {
					t.Errorf("FrontendAttrs[build-arg:%s] = %q, want %q", k, got, v)
				}
			}

			if tt.cacheDir == "" {
				if len(opt.CacheImports) != 0 || len(opt.CacheExports) != 0 {
					t.Errorf("expected no cache import/export when cacheDir is empty, got imports=%+v exports=%+v", opt.CacheImports, opt.CacheExports)
				}
				return
			}
			if len(opt.CacheImports) != 1 || opt.CacheImports[0].Type != "local" || opt.CacheImports[0].Attrs["src"] != tt.cacheDir {
				t.Errorf("CacheImports = %+v, want one local entry with src=%q", opt.CacheImports, tt.cacheDir)
			}
			if len(opt.CacheExports) != 1 || opt.CacheExports[0].Type != "local" || opt.CacheExports[0].Attrs["dest"] != tt.cacheDir {
				t.Errorf("CacheExports = %+v, want one local entry with dest=%q", opt.CacheExports, tt.cacheDir)
			}
		})
	}
}
