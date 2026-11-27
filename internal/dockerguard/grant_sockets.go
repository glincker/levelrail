package dockerguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// GrantSocketDir is where per-grant sockets live under the data dir.
func GrantSocketDir(dataDir string) string { return filepath.Join(dataDir, "dockerapi") }

// GrantSockets owns one profile socket per granted service. A grant never
// shares a socket and the daemon socket itself is never mounted anywhere.
type GrantSockets struct {
	ctx      context.Context
	dir      string
	upstream string
	policy   Policy
	tun      Tunables
	enabled  func() bool
	logger   *slog.Logger

	mu    sync.Mutex
	socks map[string]*grantSocket
}

type grantSocket struct {
	spec ProfileSpec
	srv  *Server
}

// GrantSocketsConfig is what NewGrantSockets needs. Enabled is the live
// templates.cap_docker_api setting; every request re-reads it.
type GrantSocketsConfig struct {
	Dir      string
	Upstream string
	Policy   Policy
	Tunables Tunables
	Enabled  func() bool
	Logger   *slog.Logger
}

// NewGrantSockets clears sockets a previous run left behind.
func NewGrantSockets(ctx context.Context, cfg GrantSocketsConfig) *GrantSockets {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	_ = os.RemoveAll(cfg.Dir)
	return &GrantSockets{ctx: ctx, dir: cfg.Dir, upstream: cfg.Upstream, policy: cfg.Policy, tun: cfg.Tunables,
		enabled: cfg.Enabled, logger: logger, socks: map[string]*grantSocket{}}
}

// Enabled reports the live setting; a nil reader means off.
func (g *GrantSockets) Enabled() bool { return g != nil && g.enabled != nil && g.enabled() }

// Ensure returns the host path of the socket for service, starting it or
// replacing it when the profile or project changed.
func (g *GrantSockets) Ensure(service string, spec ProfileSpec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if cur, ok := g.socks[service]; ok {
		if cur.spec == spec {
			return cur.srv.Socket, nil
		}
		_ = cur.srv.Close()
		delete(g.socks, service)
	}
	pg, err := NewProfileGuard(g.upstream, spec, g.policy, g.tun, g.enabled, g.logger)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(service))
	socket := filepath.Join(g.dir, hex.EncodeToString(sum[:6])+".sock")
	srv, err := ListenProfile(g.ctx, pg, g.tun.HeaderTimeout, socket)
	if err != nil {
		return "", fmt.Errorf("docker api socket for %s: %w", service, err)
	}
	g.socks[service] = &grantSocket{spec: spec, srv: srv}
	return socket, nil
}

// Revoke closes the socket of service, if any.
func (g *GrantSockets) Revoke(service string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if cur, ok := g.socks[service]; ok {
		_ = cur.srv.Close()
		delete(g.socks, service)
	}
}
