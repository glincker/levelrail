package localmode

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const dockerDialTimeout = 2 * time.Second

// DockerEnv is the slice of the process environment socket detection reads.
type DockerEnv struct {
	Getenv func(string) string
	Exists func(path string) bool
	Dial   func(socketPath string) error
	GOOS   string
}

// RealDockerEnv is DockerEnv backed by the running process.
func RealDockerEnv(goos string) DockerEnv {
	return DockerEnv{
		Getenv: os.Getenv,
		Exists: func(p string) bool { _, err := os.Stat(p); return err == nil },
		Dial: func(p string) error {
			c, err := (&net.Dialer{Timeout: dockerDialTimeout}).DialContext(context.Background(), "unix", p)
			if err != nil {
				return err //nolint:wrapcheck // wrapped by the caller with the socket path
			}
			return c.Close()
		},
		GOOS: goos,
	}
}

// ErrDockerUnreachable is returned when no candidate socket accepts a connection.
var ErrDockerUnreachable = errors.New("docker is not reachable")

// DockerSocketCandidates lists engine sockets in preference order: explicit settings first,
// then Docker Desktop's per-user sockets, then the system default.
func DockerSocketCandidates(env DockerEnv) []string {
	var out []string
	add := func(p string) {
		p = strings.TrimPrefix(strings.TrimSpace(p), "unix://")
		if p != "" && !strings.Contains(p, "://") {
			out = append(out, p)
		}
	}
	add(env.Getenv("APP_CONTAINER_RUNTIME_SOCKET"))
	add(env.Getenv("DOCKER_HOST"))
	if home := strings.TrimSpace(env.Getenv("HOME")); home != "" {
		add(filepath.Join(home, ".docker", "run", "docker.sock"))
		add(filepath.Join(home, ".docker", "desktop", "docker.sock"))
		add(filepath.Join(home, ".colima", "default", "docker.sock"))
		add(filepath.Join(home, ".rd", "docker.sock"))
	}
	if xdg := strings.TrimSpace(env.Getenv("XDG_RUNTIME_DIR")); xdg != "" {
		add(filepath.Join(xdg, "docker.sock"))
	}
	add("/var/run/docker.sock")
	return out
}

// DetectDocker returns the first socket that exists and accepts a connection.
func DetectDocker(env DockerEnv) (string, error) {
	var tried []string
	for _, p := range DockerSocketCandidates(env) {
		if !env.Exists(p) {
			continue
		}
		if err := env.Dial(p); err != nil {
			tried = append(tried, fmt.Sprintf("%s (%v)", p, err))
			continue
		}
		return p, nil
	}
	return "", fmt.Errorf("%w: %s", ErrDockerUnreachable, DockerFix(env.GOOS, tried))
}

// DockerFix is the one-line next step for an operator whose Docker is not reachable.
func DockerFix(goos string, tried []string) string {
	var fix string
	switch goos {
	case "darwin":
		fix = "start Docker Desktop (or Colima / Rancher Desktop) and wait for the whale icon to settle"
	case "linux":
		fix = "start the daemon with 'sudo systemctl start docker' and make sure your user is in the docker group"
	default:
		fix = "start Docker and make sure its socket is reachable"
	}
	if len(tried) > 0 {
		fix += "; sockets found but refusing connections: " + strings.Join(tried, ", ")
	}
	return fix + ". To use another socket set DOCKER_HOST."
}
