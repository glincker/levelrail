package docker

import (
	"fmt"
	"os"
	"testing"
)

func lookupFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestDetectRuntimeSocket(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		existing   map[string]bool
		wantKind   RuntimeKind
		wantRoot   bool
		wantSource string
		wantHost   string
	}{
		{
			name:       "explicit override wins over everything",
			env:        map[string]string{envRuntimeSocket: "unix:///run/user/1000/podman/podman.sock", "DOCKER_HOST": "unix:///var/run/docker.sock"},
			wantKind:   RuntimeKindPodman,
			wantRoot:   true,
			wantSource: envRuntimeSocket,
			wantHost:   "unix:///run/user/1000/podman/podman.sock",
		},
		{
			name:       "DOCKER_HOST rootful docker",
			env:        map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"},
			wantKind:   RuntimeKindDocker,
			wantRoot:   false,
			wantSource: "DOCKER_HOST",
		},
		{
			name:       "DOCKER_HOST rootless docker",
			env:        map[string]string{"DOCKER_HOST": "unix:///run/user/1000/docker.sock"},
			wantKind:   RuntimeKindDocker,
			wantRoot:   true,
			wantSource: "DOCKER_HOST",
		},
		{
			name:       "DOCKER_HOST podman rootful",
			env:        map[string]string{"DOCKER_HOST": "unix:///run/podman/podman.sock"},
			wantKind:   RuntimeKindPodman,
			wantRoot:   false,
			wantSource: "DOCKER_HOST",
		},
		{
			name:       "detects rootless docker socket on disk",
			env:        map[string]string{},
			existing:   map[string]bool{fmt.Sprintf("/run/user/%d/docker.sock", os.Getuid()): true},
			wantKind:   RuntimeKindDocker,
			wantRoot:   true,
			wantSource: "detected",
		},
		{
			name:       "falls back to rootful default",
			env:        map[string]string{},
			wantKind:   RuntimeKindDocker,
			wantRoot:   false,
			wantSource: "default",
			wantHost:   "unix:///var/run/docker.sock",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "detects rootless docker socket on disk" && os.Getuid() == 0 {
				t.Skip("candidateSockets only probes /run/user/<uid> for a non-root uid")
			}
			exists := func(path string) bool { return tt.existing[path] }
			got := detectRuntimeSocket(lookupFrom(tt.env), exists)
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", got.Kind, tt.wantKind)
			}
			if got.Rootless != tt.wantRoot {
				t.Errorf("Rootless = %v, want %v", got.Rootless, tt.wantRoot)
			}
			if got.Source != tt.wantSource {
				t.Errorf("Source = %v, want %v", got.Source, tt.wantSource)
			}
			if tt.wantHost != "" && got.Host != tt.wantHost {
				t.Errorf("Host = %v, want %v", got.Host, tt.wantHost)
			}
		})
	}
}

func TestDetectRuntimeSocket_NilLookupEnv(t *testing.T) {
	got := detectRuntimeSocket(nil, func(string) bool { return false })
	if got.Source != "default" {
		t.Errorf("Source = %v, want default", got.Source)
	}
}
