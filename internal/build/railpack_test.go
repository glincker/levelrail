package build

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	bkclient "github.com/moby/buildkit/client"
)

func TestGenerateRailpackPlan(t *testing.T) {
	tests := []struct {
		name         string
		req          RailpackRequest
		wantErr      error // checked with errors.Is when set
		wantProvider string
		wantUnsup    string // checked against UnsupportedProviderError.Provider when non-empty test expects that type
		wantErrAny   bool
	}{
		{
			name:         "node fixture detected as node",
			req:          RailpackRequest{SourceDir: "testdata/railpack-node", Tag: "levelrail-railpack:node"},
			wantProvider: "node",
		},
		{
			name:         "go fixture detected as golang",
			req:          RailpackRequest{SourceDir: "testdata/railpack-go", Tag: "levelrail-railpack:go"},
			wantProvider: "golang",
		},
		{
			name:      "unsupported: python detected but out of scope",
			req:       RailpackRequest{SourceDir: "testdata/railpack-unsupported", Tag: "levelrail-railpack:unsupported"},
			wantUnsup: "python",
		},
		{
			name:    "missing source dir fails before touching the filesystem",
			req:     RailpackRequest{Tag: "levelrail-railpack:x"},
			wantErr: ErrRailpackSourceDirRequired,
		},
		{
			name:    "missing tag fails before touching the filesystem",
			req:     RailpackRequest{SourceDir: "testdata/railpack-node"},
			wantErr: ErrRailpackTagRequired,
		},
		{
			name:       "nonexistent source dir",
			req:        RailpackRequest{SourceDir: "testdata/does-not-exist", Tag: "levelrail-railpack:x"},
			wantErrAny: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateRailpackPlan(tt.req)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("generateRailpackPlan() err = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if tt.name == "unsupported: python detected but out of scope" {
				var unsupported *UnsupportedProviderError
				if !errors.As(err, &unsupported) {
					t.Fatalf("generateRailpackPlan() err = %v, want *UnsupportedProviderError", err)
				}
				if unsupported.Provider != tt.wantUnsup {
					t.Errorf("UnsupportedProviderError.Provider = %q, want %q", unsupported.Provider, tt.wantUnsup)
				}
				if unsupported.Error() == "" {
					t.Error("UnsupportedProviderError.Error() = \"\", want a non-empty message")
				}
				return
			}

			if tt.wantErrAny {
				if err == nil {
					t.Fatal("generateRailpackPlan() err = nil, want a non-nil error")
				}
				return
			}

			if err != nil {
				t.Fatalf("generateRailpackPlan() unexpected error: %v", err)
			}
			if result == nil || result.Plan == nil {
				t.Fatal("generateRailpackPlan() result or result.Plan is nil, want a real plan")
			}
			if len(result.DetectedProviders) != 1 || result.DetectedProviders[0] != tt.wantProvider {
				t.Errorf("DetectedProviders = %v, want [%q]", result.DetectedProviders, tt.wantProvider)
			}
		})
	}
}

func TestNewRailpackSolveOpt(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		tag  string
	}{
		{name: "node plan", dir: "testdata/railpack-node", tag: "levelrail-railpack:node"},
		{name: "go plan", dir: "testdata/railpack-go", tag: "levelrail-railpack:go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateRailpackPlan(RailpackRequest{SourceDir: tt.dir, Tag: tt.tag})
			if err != nil {
				t.Fatalf("generateRailpackPlan() unexpected error: %v", err)
			}

			out := discardWriteCloser{io.Discard}
			def, opt, err := newRailpackSolveOpt(context.Background(), result.Plan, RailpackRequest{SourceDir: tt.dir, Tag: tt.tag}, out)
			if err != nil {
				t.Fatalf("newRailpackSolveOpt() unexpected error: %v", err)
			}
			if def == nil {
				t.Fatal("newRailpackSolveOpt() Definition is nil")
			}

			if len(opt.Exports) != 1 || opt.Exports[0].Type != bkclient.ExporterDocker {
				t.Fatalf("Exports = %+v, want exactly one entry of type %q", opt.Exports, bkclient.ExporterDocker)
			}
			if opt.Exports[0].Attrs["name"] != tt.tag {
				t.Errorf("Exports[0].Attrs[name] = %q, want %q", opt.Exports[0].Attrs["name"], tt.tag)
			}
			rawConfig := opt.Exports[0].Attrs["containerimage.config"]
			if rawConfig == "" {
				t.Fatal("Exports[0].Attrs[containerimage.config] is empty, want the marshaled image config Railpack computed")
			}
			var parsed map[string]any
			if err := json.Unmarshal([]byte(rawConfig), &parsed); err != nil {
				t.Errorf("containerimage.config is not valid JSON: %v", err)
			}
			if _, ok := opt.LocalMounts["context"]; !ok {
				t.Error("LocalMounts missing \"context\" mount")
			}
			// Unlike newSolveOpt's dockerfile.v0 path, a Railpack solve
			// never sets a Frontend: BuildKit is driven entirely by def.
			if opt.Frontend != "" {
				t.Errorf("Frontend = %q, want empty: a Railpack solve is Definition-based, not frontend-based", opt.Frontend)
			}
		})
	}
}

// liveRailpackClient is liveClient's Railpack-specific twin: same
// skip-cleanly-if-unreachable behavior, reused rather than duplicated
// where liveClient itself already does everything needed.
func liveRailpackClient(t *testing.T) (*dockerclient.Client, *Client) {
	t.Helper()
	return liveClient(t)
}

// TestClient_BuildRailpack_Live_Node is this integration's real proof for
// the Node.js provider: build testdata/railpack-node through Railpack's
// own detection and BuildKit's Go client, with no railpack or docker CLI
// involved, and confirm the resulting image really exists in the local
// Docker image store, the same load-bearing assertion
// TestClient_Build_Live makes for the Dockerfile path.
func TestClient_BuildRailpack_Live_Node(t *testing.T) {
	docker, bk := liveRailpackClient(t)

	tag := "levelrail-railpack-spike:node-test"
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = docker.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	progress := &collectProgress{}
	res, err := bk.BuildRailpack(ctx, RailpackRequest{
		SourceDir: "testdata/railpack-node",
		Tag:       tag,
	}, progress.fn())
	if err != nil {
		t.Fatalf("BuildRailpack() error = %v", err)
	}
	if res.Tag != tag {
		t.Errorf("Result.Tag = %q, want %q", res.Tag, tag)
	}
	if progress.count() == 0 {
		t.Error("expected at least one ProgressEvent from a real build, got none")
	}

	inspect, _, err := docker.ImageInspectWithRaw(ctx, tag)
	if err != nil {
		t.Fatalf("image %q not found after BuildRailpack(): %v", tag, err)
	}
	if inspect.ID == "" {
		t.Error("inspected image has an empty ID")
	}
}

// TestClient_BuildRailpack_Live_Go is TestClient_BuildRailpack_Live_Node's
// twin for the golang provider.
func TestClient_BuildRailpack_Live_Go(t *testing.T) {
	docker, bk := liveRailpackClient(t)

	tag := "levelrail-railpack-spike:go-test"
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = docker.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	progress := &collectProgress{}
	res, err := bk.BuildRailpack(ctx, RailpackRequest{
		SourceDir: "testdata/railpack-go",
		Tag:       tag,
	}, progress.fn())
	if err != nil {
		t.Fatalf("BuildRailpack() error = %v", err)
	}
	if res.Tag != tag {
		t.Errorf("Result.Tag = %q, want %q", res.Tag, tag)
	}

	inspect, _, err := docker.ImageInspectWithRaw(ctx, tag)
	if err != nil {
		t.Fatalf("image %q not found after BuildRailpack(): %v", tag, err)
	}
	if inspect.ID == "" {
		t.Error("inspected image has an empty ID")
	}
}

// TestClient_BuildRailpack_Live_UnsupportedProvider exercises the
// provider allow-list through the live client without needing a full
// build to run, distinguishing "buildkit unreachable" (skip) from
// "provider rejected before any solve" (a real assertion), mirroring
// TestClient_Build_Live_InvalidRequest's shape for the Dockerfile path.
func TestClient_BuildRailpack_Live_UnsupportedProvider(t *testing.T) {
	_, bk := liveRailpackClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := bk.BuildRailpack(ctx, RailpackRequest{
		SourceDir: "testdata/railpack-unsupported",
		Tag:       "levelrail-railpack-spike:unsupported-test",
	}, nil)
	var unsupported *UnsupportedProviderError
	if !errors.As(err, &unsupported) {
		t.Fatalf("BuildRailpack() err = %v, want *UnsupportedProviderError", err)
	}
}
