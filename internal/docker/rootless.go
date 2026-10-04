package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeKind is the container engine DetectRuntimeSocket believes it is
// talking to, decided from the socket path alone.
type RuntimeKind string

// RuntimeKind values DetectRuntimeSocket can return.
const (
	RuntimeKindDocker  RuntimeKind = "docker"
	RuntimeKindPodman  RuntimeKind = "podman"
	RuntimeKindUnknown RuntimeKind = "unknown"
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
	return detectRuntimeSocket(lookupEnv, socketExists)
}

func detectRuntimeSocket(lookupEnv func(string) (string, bool), exists func(string) bool) RuntimeInfo {
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if v, ok := lookupEnv(envRuntimeSocket); ok && strings.TrimSpace(v) != "" {
		return classifySocket(strings.TrimSpace(v), envRuntimeSocket)
	}
	if v, ok := lookupEnv("DOCKER_HOST"); ok && strings.TrimSpace(v) != "" {
		return classifySocket(strings.TrimSpace(v), "DOCKER_HOST")
	}
	for _, path := range candidateSockets(lookupEnv) {
		if exists(path) {
			return classifySocket("unix://"+path, "detected")
		}
	}
	return classifySocket("unix:///var/run/docker.sock", "default")
}

// candidateSockets lists rootless Docker and Podman socket paths in the
// order upstream tooling (dockerd-rootless-setuptool, podman machine)
// actually creates them, checked before the rootful default.
func candidateSockets(lookupEnv func(string) (string, bool)) []string {
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
		out = append(out, filepath.Join(strings.TrimSpace(home), ".docker", "run", "docker.sock"))
	}
	return append(out, "/run/podman/podman.sock", "/var/run/docker.sock")
}

func classifySocket(host, source string) RuntimeInfo {
	kind := RuntimeKindDocker
	if strings.Contains(strings.ToLower(host), "podman") {
		kind = RuntimeKindPodman
	}
	rootless := strings.Contains(host, "/run/user/") || strings.Contains(host, "/.docker/run/")
	return RuntimeInfo{Kind: kind, Rootless: rootless, Host: host, Source: source}
}

func socketExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
