package api

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// Hub tuning, all overridable by environment.
const (
	envHubConcurrency   = "APP_MIGRATE_HUB_CONCURRENCY"
	envHubDiskMargin    = "APP_MIGRATE_DISK_MARGIN"
	envHubMBPerSecond   = "APP_MIGRATE_ASSUMED_MBPS"
	envHubPasswordTTL   = "APP_MIGRATE_HUB_PASSWORD_TTL" //nolint:gosec // env var name, not a credential
	envHubInventoryTime = "APP_MIGRATE_INVENTORY_TIMEOUT"
	envHubReadyTimeout  = "APP_MIGRATE_TARGET_READY_TIMEOUT"
	envHubHelperVersion = "APP_MIGRATE_HELPER_VERSION"
)

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

func envFloat(name string, def float64) float64 {
	if f, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil && f > 0 {
		return f
	}
	return def
}

func envDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(name)); err == nil && d > 0 {
		return d
	}
	return def
}

type hubSecret struct {
	password string
	expires  time.Time
}

// hubState holds what must never be persisted: source passwords, kept in
// memory with a TTL, and which sessions have a copy running right now.
type hubState struct {
	mu        sync.Mutex
	passwords map[string]hubSecret
	running   map[string]bool
}

func newHubState() *hubState {
	return &hubState{passwords: map[string]hubSecret{}, running: map[string]bool{}}
}

func (h *hubState) setPassword(id, password string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.passwords[id] = hubSecret{password: password, expires: time.Now().Add(envDuration(envHubPasswordTTL, 2*time.Hour))}
}

func (h *hubState) password(id string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.passwords[id]
	if !ok || time.Now().After(s.expires) {
		delete(h.passwords, id)
		return "", false
	}
	return s.password, true
}

func (h *hubState) forget(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.passwords, id)
}

// begin marks a session's copy as running, false if it already is.
func (h *hubState) begin(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running[id] {
		return false
	}
	h.running[id] = true
	return true
}

func (h *hubState) end(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.running, id)
}

func (h *hubState) isRunning(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running[id]
}
