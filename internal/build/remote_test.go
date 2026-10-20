package build

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestNewRemoteRequest(t *testing.T) {
	tests := []struct {
		name           string
		req            Request
		cache          CacheConfig
		wantErr        error
		wantDockerfile string
		wantCache      CacheConfig
	}{
		{
			name:           "default dockerfile resolves to a context-relative name",
			req:            Request{ContextDir: filepath.Join("tmp", "checkout"), Tag: "app:sha"},
			wantDockerfile: "",
		},
		{
			name:           "explicit dockerfile is re-rooted relative to the context",
			req:            Request{ContextDir: filepath.Join("tmp", "checkout"), DockerfilePath: filepath.Join("tmp", "checkout", "docker", "Dockerfile"), Tag: "app:sha"},
			wantDockerfile: "docker/Dockerfile",
		},
		{
			name:    "dockerfile outside the context cannot be dispatched",
			req:     Request{ContextDir: filepath.Join("tmp", "checkout"), DockerfilePath: filepath.Join("tmp", "elsewhere", "Dockerfile"), Tag: "app:sha"},
			wantErr: ErrRemoteDockerfileOutsideContext,
		},
		{
			name:    "an invalid request is rejected before anything is re-rooted",
			req:     Request{Tag: "app:sha"},
			wantErr: ErrContextDirRequired,
		},
		{
			name:      "only the registry cache backend travels",
			req:       Request{ContextDir: filepath.Join("tmp", "checkout"), Tag: "app:sha"},
			cache:     CacheConfig{Dir: "/var/lib/cache", RegistryRef: "reg.example/cache:app", RegistryInsecure: true},
			wantCache: CacheConfig{RegistryRef: "reg.example/cache:app", RegistryInsecure: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRemoteRequest(tt.req, tt.cache)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewRemoteRequest() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewRemoteRequest() unexpected error: %v", err)
			}
			if got.Kind != RemoteKindDockerfile {
				t.Errorf("Kind = %q, want %q", got.Kind, RemoteKindDockerfile)
			}
			if got.DockerfilePath != tt.wantDockerfile {
				t.Errorf("DockerfilePath = %q, want %q", got.DockerfilePath, tt.wantDockerfile)
			}
			if got.Cache != tt.wantCache {
				t.Errorf("Cache = %+v, want %+v", got.Cache, tt.wantCache)
			}
			if got.ContextDir != tt.req.ContextDir || got.Tag != tt.req.Tag {
				t.Errorf("got = %+v, want ContextDir/Tag carried through from %+v", got, tt.req)
			}
		})
	}
}

func TestNewRemoteRailpackRequest(t *testing.T) {
	got, err := NewRemoteRailpackRequest(RailpackRequest{SourceDir: "src", Tag: "app:sha"}, CacheConfig{Dir: "/var/cache"})
	if err != nil {
		t.Fatalf("NewRemoteRailpackRequest() unexpected error: %v", err)
	}
	if got.Kind != RemoteKindRailpack || got.ContextDir != "src" || got.Tag != "app:sha" {
		t.Errorf("got = %+v, want a railpack request over src", got)
	}
	if got.Cache != (CacheConfig{}) {
		t.Errorf("Cache = %+v, want the dispatching side's local cache dir dropped", got.Cache)
	}

	if _, err := NewRemoteRailpackRequest(RailpackRequest{Tag: "app:sha"}, CacheConfig{}); !errors.Is(err, ErrRailpackSourceDirRequired) {
		t.Fatalf("NewRemoteRailpackRequest() with no source dir err = %v, want %v", err, ErrRailpackSourceDirRequired)
	}
}

func TestClient_SolveRemote_UnknownKind(t *testing.T) {
	// No BuildKit connection needed: the kind is rejected before any
	// solve path is entered, which is the whole point of checking it
	// first.
	var c Client
	if _, err := c.SolveRemote(t.Context(), RemoteRequest{Kind: "compose", Tag: "app:sha"}, nil, nil); err == nil {
		t.Fatal("SolveRemote() with an unrecognized kind: error = nil, want a non-nil error")
	}
}
