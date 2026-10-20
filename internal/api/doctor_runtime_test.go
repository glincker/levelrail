package api

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type fakeRuntimeReporter struct {
	info docker.RuntimeInfo
}

func (f fakeRuntimeReporter) Ping(context.Context) error { return nil }
func (f fakeRuntimeReporter) Runtime() docker.RuntimeInfo {
	return f.info
}

func TestDoctorCheckContainerRuntime_NoDockerClientFallsBackToEnvDetection(t *testing.T) {
	rt := &Router{}
	got := rt.doctorCheckContainerRuntime(context.Background())
	if got.Code != "container_runtime" {
		t.Fatalf("Code = %q, want container_runtime", got.Code)
	}
	if got.Status != doctorStatusOK && got.Status != doctorStatusWarn {
		t.Errorf("Status = %v, want ok or warn", got.Status)
	}
}

func TestDoctorCheckContainerRuntime(t *testing.T) {
	tests := []struct {
		name       string
		pinger     DockerPinger
		wantStatus string
		wantSubstr string
	}{
		{
			name:       "rootful docker is ok",
			pinger:     fakeRuntimeReporter{info: docker.RuntimeInfo{Kind: docker.RuntimeKindDocker, Rootless: false, Host: "unix:///var/run/docker.sock", Source: "default"}},
			wantStatus: doctorStatusOK,
			wantSubstr: "docker (rootful)",
		},
		{
			name:       "rootless docker warns",
			pinger:     fakeRuntimeReporter{info: docker.RuntimeInfo{Kind: docker.RuntimeKindDocker, Rootless: true, Host: "unix:///run/user/1000/docker.sock", Source: "DOCKER_HOST"}},
			wantStatus: doctorStatusWarn,
			wantSubstr: "best-effort",
		},
		{
			name:       "podman warns",
			pinger:     fakeRuntimeReporter{info: docker.RuntimeInfo{Kind: docker.RuntimeKindPodman, Rootless: false, Host: "unix:///run/podman/podman.sock", Source: "DOCKER_HOST"}},
			wantStatus: doctorStatusWarn,
			wantSubstr: "podman (rootful)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &Router{dockerPinger: tt.pinger}
			got := rt.doctorCheckContainerRuntime(context.Background())
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", got.Status, tt.wantStatus)
			}
			if !strings.Contains(got.Message, tt.wantSubstr) {
				t.Errorf("Message = %q, want substring %q", got.Message, tt.wantSubstr)
			}
		})
	}
}
