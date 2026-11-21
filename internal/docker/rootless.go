package docker

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// socketDialTimeout bounds the liveness probe of one candidate socket; a
// stale socket file refuses at once, so this only caps a wedged listener.
const socketDialTimeout = 1500 * time.Millisecond

// RuntimeKind is the container engine DetectRuntimeSocket believes it is
// talking to, decided from the socket path and, when available, refined
// from the engine's own /version and /info answer (see RefineKind).
type RuntimeKind string

// RuntimeKind values DetectRuntimeSocket can return.
const (
	RuntimeKindDocker         RuntimeKind = "docker"
	RuntimeKindPodman         RuntimeKind = "podman"
	RuntimeKindDockerDesktop  RuntimeKind = "docker-desktop"
	RuntimeKindColima         RuntimeKind = "colima"
	RuntimeKindLima           RuntimeKind = "lima"
	RuntimeKindOrbStack       RuntimeKind = "orbstack"
	RuntimeKindRancherDesktop RuntimeKind = "rancher-desktop"
	RuntimeKindUnknown        RuntimeKind = "unknown"
)

// envRuntimeSocket overrides which Docker Engine API host this client
// connects to, ahead of the standard DOCKER_HOST the Docker SDK's
// FromEnv already reads. Useful when an operator wants the control plane
// pinned to a specific rootless or Podman socket without exporting
// DOCKER_HOST into the whole process environment.
const envRuntimeSocket = "APP_CONTAINER_RUNTIME_SOCKET"

// RuntimeInfo is DetectRuntimeSocket's result: which engine and privilege
// mode this control plane is likely talking to, and which setting decided
// the socket.
type RuntimeInfo struct {
	Kind     RuntimeKind
	Rootless bool
	// Host is the docker host URL (unix://... or tcp://...) this was
	// classified from.
	Host string
	// Source is the env var name that supplied Host, or "detected" for a
	// well-known socket path found on disk, or "default" for the
	// standard rootful fallback.
	Source string
}

// DetectRuntimeSocket resolves the Docker Engine API host NewClient would
// connect to, in the same precedence order NewClient itself applies:
// APP_CONTAINER_RUNTIME_SOCKET, then DOCKER_HOST, then a well-known
// rootless or Podman socket path, then the standard rootful default. It
// classifies engine (Docker vs Podman) and privilege mode (rootless vs
// rootful) from that path alone, a heuristic based on well-documented
// conventions (XDG_RUNTIME_DIR, /run/user/<uid>, Podman's socket name):
// it never dials the daemon, so a live Docker client's own ServerVersion
// is the only verified corroborating signal available.
func DetectRuntimeSocket(lookupEnv func(string) (string, bool)) RuntimeInfo {
	return detectRuntimeSocketProbe(lookupEnv, socketExists, dialableSocket, runtime.GOOS)
}

func detectRuntimeSocket(lookupEnv func(string) (string, bool), exists func(string) bool) RuntimeInfo {
	return detectRuntimeSocketOS(lookupEnv, exists, runtime.GOOS)
}

// detectRuntimeSocketOS takes goos so the macOS-only candidates are testable on any host.
func detectRuntimeSocketOS(lookupEnv func(string) (string, bool), exists func(string) bool, goos string) RuntimeInfo {
	return detectRuntimeSocketProbe(lookupEnv, exists, exists, goos)
}

// detectRuntimeSocketProbe prefers the first candidate that exists and answers
// a dial, so a stale socket file left by a quit desktop app does not hide a live one.
func detectRuntimeSocketProbe(lookupEnv func(string) (string, bool), exists, live func(string) bool, goos string) RuntimeInfo {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if v, ok := lookupEnv(envRuntimeSocket); ok && strings.TrimSpace(v) != "" {
		return classifySocket(strings.TrimSpace(v), envRuntimeSocket)
	}
	if v, ok := lookupEnv("DOCKER_HOST"); ok && strings.TrimSpace(v) != "" {
		return classifySocket(strings.TrimSpace(v), "DOCKER_HOST")
	}
	var dead []string
	for _, path := range candidateSockets(lookupEnv, goos) {
		if !exists(path) {
			continue
		}
		if live(path) {
			return classifySocket("unix://"+path, "detected")
		}
		dead = append(dead, path)
	}
	if len(dead) > 0 {
		slog.Warn("docker: runtime sockets found but not reachable", slog.String("sockets", strings.Join(dead, ",")))
		return classifySocket("unix://"+dead[0], "detected")
	}
	return classifySocket("unix:///var/run/docker.sock", "default")
}

// candidateSockets lists rootless Docker and Podman socket paths in the
// order upstream tooling (dockerd-rootless-setuptool, podman machine)
// actually creates them, checked before the rootful default.
func candidateSockets(lookupEnv func(string) (string, bool), goos string) []string {
	var out []string
	if xdg, ok := lookupEnv("XDG_RUNTIME_DIR"); ok && strings.TrimSpace(xdg) != "" {
		xdg = strings.TrimSpace(xdg)
		out = append(out, filepath.Join(xdg, "docker.sock"), filepath.Join(xdg, "podman", "podman.sock"))
	}
	if uid := os.Getuid(); uid > 0 {
		out = append(out,
			fmt.Sprintf("/run/user/%d/docker.sock", uid),
			fmt.Sprintf("/run/user/%d/podman/podman.sock", uid),
		)
	}
	if home, ok := lookupEnv("HOME"); ok && strings.TrimSpace(home) != "" {
		home = strings.TrimSpace(home)
		out = append(out,
			filepath.Join(home, ".docker", "run", "docker.sock"),
			filepath.Join(home, ".colima", "default", "docker.sock"),
			filepath.Join(home, ".orbstack", "run", "docker.sock"),
			filepath.Join(home, ".rd", "docker.sock"),
		)
		if goos == "darwin" {
			out = append(out, limaSockets(home)...)
		}
	}
	return append(out, "/run/podman/podman.sock", "/var/run/docker.sock")
}

// limaSockets lists each Lima instance's Docker socket, sorted by instance
// name; Lima names the directory after the instance so it cannot be a fixed path.
func limaSockets(home string) []string {
	matches, err := filepath.Glob(filepath.Join(home, ".lima", "*", "sock", "docker.sock"))
	if err != nil {
		return nil
	}
	return matches
}

// ConnectHost is the host the Docker client must dial explicitly, or "" when
// the SDK's own DOCKER_HOST handling is right. On macOS the standard socket
// path is often absent, so a detected desktop-runtime socket is used.
func ConnectHost(info RuntimeInfo, goos string) string {
	if info.Source == envRuntimeSocket || (info.Source == "detected" && goos == "darwin") {
		return info.Host
	}
	return ""
}

func classifySocket(host, source string) RuntimeInfo {
	lower := strings.ToLower(host)
	kind := kindFromPath(lower)
	rootless := strings.Contains(lower, "/run/user/")
	return RuntimeInfo{Kind: kind, Rootless: rootless, Host: host, Source: source}
}

// kindFromPath names the engine from a lower-cased socket URL. Desktop
// runtimes run a VM and are not rootless in the Linux sense. A tcp or ssh
// host or an unrecognised socket name stays unknown until the engine answers.
func kindFromPath(lower string) RuntimeKind {
	switch {
	case strings.Contains(lower, "podman"):
		return RuntimeKindPodman
	case strings.Contains(lower, "/.colima/"):
		return RuntimeKindColima
	case strings.Contains(lower, "/.lima/"):
		return RuntimeKindLima
	case strings.Contains(lower, "/.orbstack/"):
		return RuntimeKindOrbStack
	case strings.Contains(lower, "/.rd/"), strings.Contains(lower, "/.rancher-desktop/"):
		return RuntimeKindRancherDesktop
	case strings.Contains(lower, "/.docker/run/"), strings.Contains(lower, "/.docker/desktop/"):
		return RuntimeKindDockerDesktop
	case strings.HasPrefix(lower, "unix://") && strings.HasSuffix(lower, "docker.sock"):
		return RuntimeKindDocker
	}
	return RuntimeKindUnknown
}

// EngineIdentity is what an engine says about itself on /version and /info.
type EngineIdentity struct {
	PlatformName    string
	Components      []string
	OperatingSystem string
}

// RefineKind corrects a path-derived kind with the engine's own answer.
// A kind the path already pinned to a specific product is kept, except
// that a Podman answer always wins (Podman can sit behind a docker.sock path).
func RefineKind(pathKind RuntimeKind, id EngineIdentity) RuntimeKind {
	all := strings.ToLower(id.PlatformName + " " + id.OperatingSystem + " " + strings.Join(id.Components, " "))
	switch {
	case strings.Contains(all, "podman"):
		return RuntimeKindPodman
	case pathKind != RuntimeKindDocker && pathKind != RuntimeKindUnknown:
		return pathKind
	case strings.Contains(all, "orbstack"):
		return RuntimeKindOrbStack
	case strings.Contains(all, "colima"):
		return RuntimeKindColima
	case strings.Contains(all, "rancher desktop"):
		return RuntimeKindRancherDesktop
	case strings.Contains(all, "docker desktop"):
		return RuntimeKindDockerDesktop
	case strings.Contains(all, "docker engine"), strings.Contains(all, "moby"):
		return RuntimeKindDocker
	}
	return pathKind
}

// SupportLevel says how much this platform vouches for a runtime.
type SupportLevel string

// SupportLevel values reported by the container_runtime doctor check.
const (
	SupportSupported  SupportLevel = "supported"
	SupportBestEffort SupportLevel = "best-effort"
	SupportUnverified SupportLevel = "unverified"
)

// SupportFor grades a detected runtime: rootful Docker Engine on Linux is
// supported, everything else known is best-effort, unknown is unverified.
func SupportFor(info RuntimeInfo, goos string) (SupportLevel, string) {
	switch {
	case info.Kind == RuntimeKindUnknown:
		return SupportUnverified, "engine not recognised, behaviour untested"
	case info.Kind == RuntimeKindPodman:
		return SupportBestEffort, "Podman is exercised through its Docker-compatible API only"
	case info.Kind != RuntimeKindDocker:
		return SupportBestEffort, "desktop runtime, intended for levelrail-cli dev rather than production hosts"
	case info.Rootless:
		return SupportBestEffort, "rootless Docker has UID mapping and low-port limits"
	case goos != "linux":
		return SupportBestEffort, "Docker Engine outside Linux is intended for levelrail-cli dev"
	}
	return SupportSupported, "rootful Docker Engine on Linux"
}

func socketExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dialableSocket(path string) bool {
	d := net.Dialer{Timeout: socketDialTimeout}
	conn, err := d.DialContext(context.Background(), "unix", path)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
