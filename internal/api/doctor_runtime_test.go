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

type fakeIdentifyingReporter struct {
	fakeRuntimeReporter
	id docker.EngineIdentity
}

func (f fakeIdentifyingReporter) EngineIdentity(context.Context) (docker.EngineIdentity, error) {
	return f.id, nil
}

func TestDoctorCheckContainerRuntime_KindsAndSupport(t *testing.T) {
	prev := runtimeGOOS
	runtimeGOOS = "linux"
	t.Cleanup(func() { runtimeGOOS = prev })
	sock := func(k docker.RuntimeKind, rootless bool, host string) docker.RuntimeInfo {
		return docker.RuntimeInfo{Kind: k, Rootless: rootless, Host: host, Source: "detected"}
	}
	tests := []struct {
		name      string
		pinger    DockerPinger
		wantKind  string
		wantLevel string
		wantWarn  bool
		wantText  string
	}{
		{"rootful docker", fakeRuntimeReporter{info: sock(docker.RuntimeKindDocker, false, "unix:///var/run/docker.sock")}, "docker", "supported", false, ""},
		{"colima", fakeRuntimeReporter{info: sock(docker.RuntimeKindColima, false, "unix:///h/.colima/default/docker.sock")}, "colima", "best-effort", true, "APP_CONTAINER_RUNTIME_SOCKET"},
		{"podman notes builds", fakeRuntimeReporter{info: sock(docker.RuntimeKindPodman, false, "unix:///run/podman/podman.sock")}, "podman", "best-effort", true, "source builds"},
		{"unknown is unverified", fakeRuntimeReporter{info: sock(docker.RuntimeKindUnknown, false, "tcp://10.0.0.2:2375")}, "unknown", "unverified", true, "not verified"},
		{
			"engine answer says podman behind docker.sock",
			fakeIdentifyingReporter{fakeRuntimeReporter{info: sock(docker.RuntimeKindDocker, false, "unix:///var/run/docker.sock")}, docker.EngineIdentity{Components: []string{"Podman Engine"}}},
			"podman", "best-effort", true, "enable podman.socket",
		},
		{
			"engine answer says orbstack behind docker.sock",
			fakeIdentifyingReporter{fakeRuntimeReporter{info: sock(docker.RuntimeKindDocker, false, "unix:///var/run/docker.sock")}, docker.EngineIdentity{OperatingSystem: "OrbStack"}},
			"orbstack", "best-effort", true, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&Router{dockerPinger: tt.pinger}).doctorCheckContainerRuntime(context.Background())
			if got.EngineKind != tt.wantKind || got.SupportLevel != tt.wantLevel {
				t.Errorf("kind/level = %s/%s, want %s/%s", got.EngineKind, got.SupportLevel, tt.wantKind, tt.wantLevel)
			}
			if (got.Status == doctorStatusWarn) != tt.wantWarn || got.Status == doctorStatusFail {
				t.Errorf("Status = %v, wantWarn %v", got.Status, tt.wantWarn)
			}
			if !strings.Contains(got.Message, tt.wantText) {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantText)
			}
			if tt.wantWarn && got.DocsPath != "/container-runtimes" {
				t.Errorf("DocsPath = %q", got.DocsPath)
			}
		})
	}
}

func TestDoctorCheckContainerRuntime(t *testing.T) {
	prev := runtimeGOOS
	runtimeGOOS = "linux"
	t.Cleanup(func() { runtimeGOOS = prev })
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

func TestRelabelEngineUsesPodmanName(t *testing.T) {
	prev := runtimeGOOS
	runtimeGOOS = "linux"
	t.Cleanup(func() { runtimeGOOS = prev })
	check := func(context.Context) doctorCheckResource { return doctorCheckResource{Name: "Docker daemon"} }
	podman := &Router{dockerPinger: fakeRuntimeReporter{info: docker.RuntimeInfo{Kind: docker.RuntimeKindPodman}}}
	if got := podman.relabelEngine(check)(context.Background()).Name; got != "Podman daemon" {
		t.Errorf("podman name = %q", got)
	}
	dock := &Router{dockerPinger: fakeRuntimeReporter{info: docker.RuntimeInfo{Kind: docker.RuntimeKindDocker}}}
	if got := dock.relabelEngine(check)(context.Background()).Name; got != "Docker daemon" {
		t.Errorf("docker name = %q", got)
	}
}
