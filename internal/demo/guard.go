// Package demo builds and guards the read-only public demo instance: a
// seeded control plane with no Docker, ingress or agent listeners.
package demo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvDemoMode is the explicit opt-in the demo entrypoint requires.
const EnvDemoMode = "APP_DEMO_MODE"

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
	if !Enabled(getenv) {
		return fmt.Errorf("demo: %s must be set to 1 to run the demo entrypoint", EnvDemoMode)
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
	if addr := strings.TrimSpace(getenv("APP_HTTP_ADDR")); addr != "" && !isLoopbackAddr(addr) {
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
