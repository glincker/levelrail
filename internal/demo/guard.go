// Package demo builds and guards the read-only public demo instance: a
// seeded control plane with no Docker, ingress or agent listeners.
package demo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

// EnvDemoMode is the explicit opt-in the demo entrypoint requires.
const EnvDemoMode = "APP_DEMO_MODE"

// EnvDemoContainer opts a containerised demo into a non-loopback bind. It is
// honoured only when a container is detected.
const EnvDemoContainer = "APP_DEMO_CONTAINER"

// ContainerDefaultAddr is the bind used inside a container with the opt-in.
const ContainerDefaultAddr = "0.0.0.0:8080"

// ErrNotDemoInstance is returned when the data dir does not carry the seeder's marker.
var ErrNotDemoInstance = errors.New("demo: data dir was not produced by the demo seeder")

// forbiddenEnv lists the variables whose presence means a real Docker host,
// ingress or agent listener is configured; the demo refuses to start with any.
var forbiddenEnv = []string{
	"DOCKER_HOST",
	"APP_AGENT_ADDR",
	"APP_INGRESS_HTTP_ADDR",
	"APP_INGRESS_HTTPS_ADDR",
}

// Enabled reports whether the explicit demo opt-in is set to a true value.
func Enabled(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(EnvDemoMode))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// CheckEnvironment fails closed unless demo mode is explicitly enabled and
// nothing that reaches a real host (Docker, ingress, agents, a public bind) is configured.
func CheckEnvironment(getenv func(string) string) error {
	return CheckEnvironmentIn(getenv, InContainer, socketStat)
}

// InContainer reports a container signal: a runtime marker file or a
// container runtime named in PID 1's cgroup.
func InContainer() bool {
	for _, m := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(m); err == nil {
			return true
		}
	}
	b, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	for _, hint := range []string{"docker", "kubepods", "containerd", "libpod"} {
		if strings.Contains(string(b), hint) {
			return true
		}
	}
	return false
}

// ContainerOptIn reports whether the container bind opt-in is set to a true value.
func ContainerOptIn(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(EnvDemoContainer))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// ResolveAddr returns the bind address: APP_HTTP_ADDR if set, else the
// container default only with the opt-in and a container, else loopback.
func ResolveAddr(getenv func(string) string, inContainer func() bool, loopbackDefault string) string {
	if a := strings.TrimSpace(getenv("APP_HTTP_ADDR")); a != "" {
		return a
	}
	if ContainerOptIn(getenv) && inContainer() {
		return ContainerDefaultAddr
	}
	return loopbackDefault
}

// dockerSockets are the socket paths whose presence means the process could
// reach a real Docker daemon.
var dockerSockets = []string{"/var/run/docker.sock", "/run/docker.sock"}

// socketStat is the production socket probe; tests swap it through SetSocketStatForTest.
var socketStat = os.Stat

// SetSocketStatForTest replaces the Docker socket probe used by CheckEnvironment
// and returns a func that restores it. Not safe for parallel tests.
func SetSocketStatForTest(fn func(string) (os.FileInfo, error)) (restore func()) {
	prev := socketStat
	socketStat = fn
	return func() { socketStat = prev }
}

// CheckEnvironmentIn is CheckEnvironment with injectable container and stat probes.
func CheckEnvironmentIn(getenv func(string) string, inContainer func() bool, stat func(string) (os.FileInfo, error)) error {
	if !Enabled(getenv) {
		return fmt.Errorf("demo: %s must be set to 1 to run the demo entrypoint", EnvDemoMode)
	}
	// Checked before the opt-in so a mounted socket is refused in every mode.
	for _, sock := range dockerSockets {
		if _, err := stat(sock); err == nil {
			return fmt.Errorf("demo: refusing to start, a Docker socket is present at %s; the demo must never have Docker access", sock)
		}
	}
	var found []string
	for _, k := range forbiddenEnv {
		if strings.TrimSpace(getenv(k)) != "" {
			found = append(found, k)
		}
	}
	if len(found) > 0 {
		sort.Strings(found)
		return fmt.Errorf("demo: refusing to start with real host surfaces configured: %s", strings.Join(found, ", "))
	}
	optIn := ContainerOptIn(getenv)
	if optIn && !inContainer() {
		return fmt.Errorf("demo: %s is set but no container was detected, refusing to start", EnvDemoContainer)
	}
	if addr := strings.TrimSpace(getenv("APP_HTTP_ADDR")); addr != "" && !optIn && !isLoopbackAddr(addr) {
		return fmt.Errorf("demo: APP_HTTP_ADDR %q is not a loopback address", addr)
	}
	return nil
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CheckStore requires the seeder's marker and that every user row is one the
// seeder created, so a real instance's data dir can never be served as a demo.
func CheckStore(ctx context.Context, db *store.DB) error {
	if _, err := db.GetDemoMarker(ctx); err != nil {
		if errors.Is(err, store.ErrDemoMarkerNotFound) {
			return ErrNotDemoInstance
		}
		return fmt.Errorf("demo: read seed marker: %w", err)
	}
	users, err := db.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("demo: list users: %w", err)
	}
	for _, u := range users {
		if !IsSeededUserID(u.ID) {
			return fmt.Errorf("demo: refusing to start, data dir holds a non-seeded user %q", u.ID)
		}
	}
	return nil
}
