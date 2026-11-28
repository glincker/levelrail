package main

import (
	"context"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/network"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

// appIPMap caches container IP to app name, rebuilt lazily so the DNS hot path never waits on Docker.
type appIPMap struct {
	mu      sync.Mutex
	ips     map[string]string
	built   time.Time
	loading bool
}

// newMeshDNSQueryLog builds the query log whose knobs all come from live runtime settings.
func newMeshDNSQueryLog(settings *runtimesettings.Service, client *docker.Client, logger *slog.Logger) *network.QueryLog {
	m := &appIPMap{ips: map[string]string{}}
	ctx := context.Background()
	return network.NewQueryLog(network.QueryLogConfig{
		Enabled: func() bool { return settings.Bool(ctx, runtimesettings.KeyDNSQueryLogEnabled) },
		Size:    func() int { return settings.Int(ctx, runtimesettings.KeyDNSQueryLogSize) },
		Privacy: func() string { return settings.String(ctx, runtimesettings.KeyDNSQueryLogPrivacy) },
		AppForIP: func(a netip.Addr) string {
			return m.lookup(a, settings.Duration(ctx, runtimesettings.KeyDNSQueryLogAppRefresh), client, logger)
		},
	})
}

func (m *appIPMap) lookup(a netip.Addr, maxAge time.Duration, client *docker.Client, logger *slog.Logger) string {
	m.mu.Lock()
	app := m.ips[a.String()]
	stale := time.Since(m.built) > maxAge && !m.loading
	if stale {
		m.loading = true
	}
	m.mu.Unlock()
	if stale && client != nil {
		go m.refresh(client, logger)
	}
	return app
}

func (m *appIPMap) refresh(client *docker.Client, logger *slog.Logger) {
	ips, err := client.AppContainerIPs(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loading = false
	m.built = time.Now()
	if err != nil {
		logger.Warn("mesh dns: app ip map refresh failed", slog.String("error", err.Error()))
		return
	}
	m.ips = ips
}
