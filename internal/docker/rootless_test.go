package docker

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestDetectRuntimeSocket_DesktopPaths(t *testing.T) {
	home := "/home/dev"
	tests := []struct {
		name     string
		existing string
		want     RuntimeKind
	}{
		{"docker desktop", home + "/.docker/run/docker.sock", RuntimeKindDockerDesktop},
		{"colima", home + "/.colima/default/docker.sock", RuntimeKindColima},
		{"orbstack", home + "/.orbstack/run/docker.sock", RuntimeKindOrbStack},
		{"rancher desktop", home + "/.rd/docker.sock", RuntimeKindRancherDesktop},
		{"podman rootful", "/run/podman/podman.sock", RuntimeKindPodman},
		{"podman rootless via xdg", "/run/xdg/podman/podman.sock", RuntimeKindPodman},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"HOME": home, "XDG_RUNTIME_DIR": "/run/xdg"}
			got := detectRuntimeSocket(lookupFrom(env), func(p string) bool { return p == tt.existing })
			if got.Kind != tt.want || got.Source != "detected" || got.Host != "unix://"+tt.existing {
				t.Errorf("got %+v, want kind %s", got, tt.want)
			}
		})
	}
}

func TestClassifySocket_Kinds(t *testing.T) {
	tests := map[string]RuntimeKind{
		"unix:///var/run/docker.sock":             RuntimeKindDocker,
		"tcp://10.0.0.2:2375":                     RuntimeKindUnknown,
		"unix:///tmp/custom.sock":                 RuntimeKindUnknown,
		"unix:///run/user/1000/docker.sock":       RuntimeKindDocker,
		"unix:///Users/a/.docker/run/docker.sock": RuntimeKindDockerDesktop,
	}
	for host, want := range tests {
		if got := classifySocket(host, "x"); got.Kind != want {
			t.Errorf("%s: kind %s, want %s", host, got.Kind, want)
		}
	}
	if classifySocket("unix:///Users/a/.docker/run/docker.sock", "x").Rootless {
		t.Error("docker desktop must not be rootless")
	}
}

func TestRefineKind(t *testing.T) {
	tests := []struct {
		name string
		path RuntimeKind
		id   EngineIdentity
		want RuntimeKind
	}{
		{"podman component", RuntimeKindDocker, EngineIdentity{Components: []string{"Podman Engine"}}, RuntimeKindPodman},
		{"podman beats desktop path", RuntimeKindColima, EngineIdentity{PlatformName: "Podman Engine"}, RuntimeKindPodman},
		{"docker desktop os", RuntimeKindDocker, EngineIdentity{OperatingSystem: "Docker Desktop"}, RuntimeKindDockerDesktop},
		{"orbstack os", RuntimeKindUnknown, EngineIdentity{OperatingSystem: "OrbStack"}, RuntimeKindOrbStack},
		{"specific path kept", RuntimeKindColima, EngineIdentity{PlatformName: "Docker Engine - Community"}, RuntimeKindColima},
		{"unknown path, docker answer", RuntimeKindUnknown, EngineIdentity{PlatformName: "Docker Engine - Community"}, RuntimeKindDocker},
		{"no signal keeps path", RuntimeKindUnknown, EngineIdentity{}, RuntimeKindUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RefineKind(tt.path, tt.id); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSupportFor(t *testing.T) {
	tests := []struct {
		info RuntimeInfo
		goos string
		want SupportLevel
	}{
		{RuntimeInfo{Kind: RuntimeKindDocker}, "linux", SupportSupported},
		{RuntimeInfo{Kind: RuntimeKindDocker, Rootless: true}, "linux", SupportBestEffort},
		{RuntimeInfo{Kind: RuntimeKindDocker}, "darwin", SupportBestEffort},
		{RuntimeInfo{Kind: RuntimeKindPodman}, "linux", SupportBestEffort},
		{RuntimeInfo{Kind: RuntimeKindOrbStack}, "darwin", SupportBestEffort},
		{RuntimeInfo{Kind: RuntimeKindUnknown}, "linux", SupportUnverified},
	}
	for _, tt := range tests {
		got, reason := SupportFor(tt.info, tt.goos)
		if got != tt.want || reason == "" {
			t.Errorf("%+v on %s: %s %q, want %s", tt.info, tt.goos, got, reason, tt.want)
		}
	}
}

func TestDetectRuntimeSocket_LimaOnlyOnDarwin(t *testing.T) {
	home := t.TempDir()
	sock := filepath.Join(home, ".lima", "default", "sock", "docker.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	env := lookupFrom(map[string]string{"HOME": home})
	got := detectRuntimeSocketOS(env, socketExists, "darwin")
	if got.Kind != RuntimeKindLima || got.Host != "unix://"+sock {
		t.Errorf("darwin got %+v, want lima socket", got)
	}
	if got := detectRuntimeSocketOS(env, socketExists, "linux"); got.Kind == RuntimeKindLima {
		t.Errorf("linux must ignore lima, got %+v", got)
	}
}

func TestDetectRuntimeSocket_DockerHostWinsOnDarwin(t *testing.T) {
	home := t.TempDir()
	env := lookupFrom(map[string]string{"HOME": home, "DOCKER_HOST": "unix:///custom.sock"})
	got := detectRuntimeSocketOS(env, func(string) bool { return true }, "darwin")
	if got.Source != "DOCKER_HOST" || got.Host != "unix:///custom.sock" {
		t.Errorf("got %+v", got)
	}
}

func TestConnectHost(t *testing.T) {
	detected := RuntimeInfo{Source: "detected", Host: "unix:///h/.orbstack/run/docker.sock"}
	if got := ConnectHost(detected, "darwin"); got != detected.Host {
		t.Errorf("darwin detected = %q", got)
	}
	if got := ConnectHost(detected, "linux"); got != "" {
		t.Errorf("linux detected must stay on the SDK default, got %q", got)
	}
	if got := ConnectHost(RuntimeInfo{Source: "DOCKER_HOST", Host: "tcp://x"}, "darwin"); got != "" {
		t.Errorf("DOCKER_HOST is read by the SDK, got %q", got)
	}
	if got := ConnectHost(RuntimeInfo{Source: envRuntimeSocket, Host: "unix:///p"}, "linux"); got != "unix:///p" {
		t.Errorf("pinned socket = %q", got)
	}
}

func TestDetectRuntimeSocketSkipsDeadSocket(t *testing.T) {
	home := t.TempDir()
	dead := filepath.Join(home, ".docker", "run", "docker.sock")
	if err := os.MkdirAll(filepath.Dir(dead), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dead, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) (string, bool) {
		if k == "HOME" {
			return home, true
		}
		return "", false
	}
	probe := func(os string) RuntimeInfo {
		// Only paths under the temp home count, so a real daemon on this host cannot leak in.
		inHome := func(p string) bool { return strings.HasPrefix(p, home) }
		exists := func(p string) bool { return inHome(p) && socketExists(p) }
		dial := func(p string) bool { return inHome(p) && dialableSocket(p) }
		return detectRuntimeSocketProbe(env, exists, dial, os)
	}

	// Only a dead candidate: keep the first existing one.
	if got := probe("darwin"); got.Host != "unix://"+dead || got.Source != "detected" {
		t.Fatalf("none live fallback = %+v", got)
	}

	live := filepath.Join(home, ".lima", "default", "sock", "docker.sock")
	if err := os.MkdirAll(filepath.Dir(live), 0o750); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", live)
	if err != nil {
		t.Skipf("unix listener unavailable: %v", err)
	}
	defer func() { _ = ln.Close() }()

	got := probe("darwin")
	if got.Host != "unix://"+live || got.Kind != RuntimeKindLima {
		t.Fatalf("live candidate not chosen: %+v", got)
	}
}
