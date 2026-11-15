package main

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/hostfw"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// meshGatewayDNS runs the per-bridge resolver and its host firewall rule behind a live setting.
type meshGatewayDNS struct {
	gw          *network.GatewayDNS
	rules       hostfw.DNSRules
	client      *docker.Client
	settings    *runtimesettings.Service
	localNodeID string
	legacy      string
	logger      *slog.Logger
	defaultGW   atomic.Pointer[netip.Addr]
	fwClean     bool
	fwErr       string
}

// newMeshGatewayDNS builds the manager. legacy is the static :53 address, if any, kept for non-gateway setups.
func newMeshGatewayDNS(client *docker.Client, meshCfg *meshSetup, legacy, tag string, logger *slog.Logger) *meshGatewayDNS {
	m := &meshGatewayDNS{
		client: client, settings: meshCfg.settings, localNodeID: meshCfg.localNodeID, legacy: legacy, logger: logger,
		rules: hostfw.DNSRules{Deps: hostfw.SystemDeps(), Tag: tag},
	}
	ctx := context.Background()
	m.gw = network.NewGatewayDNS(network.GatewayDNSConfig{
		Enabled:  func() bool { return m.settings.Bool(ctx, runtimesettings.KeyDNSMeshResolverEnabled) },
		Bridges:  m.bridges,
		MeshCIDR: func() netip.Prefix { return meshCIDR(logger) },
		Port:     dockerNameserverPort, Resolver: meshCfg.resolver, Logger: logger,
		BindTimeout: bootDockerCallTimeout,
	})
	return m
}

func (m *meshGatewayDNS) bridges(ctx context.Context) ([]network.GatewayBridge, error) {
	list, err := m.client.ManagedBridges(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]network.GatewayBridge, 0, len(list))
	for _, b := range list {
		out = append(out, network.GatewayBridge{Iface: b.Iface, Gateway: b.Gateway, Subnet: b.Subnet})
		if b.Name == "bridge" {
			gw := b.Gateway
			m.defaultGW.Store(&gw)
		}
	}
	return out, nil
}

// run reconciles listeners and the firewall rule until ctx ends.
func (m *meshGatewayDNS) run(ctx context.Context) {
	defer m.gw.Close()
	for {
		m.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.settings.Duration(ctx, runtimesettings.KeyDNSMeshResolverInterval)):
		}
	}
}

func (m *meshGatewayDNS) tick(ctx context.Context) {
	m.gw.Reconcile(ctx)
	if !m.settings.Bool(ctx, runtimesettings.KeyDNSMeshResolverEnabled) {
		if !m.fwClean {
			if err := m.rules.Remove(ctx); err != nil {
				m.logger.Warn("mesh dns: could not remove the firewall accept", slog.String("error", err.Error()))
				return
			}
			m.fwClean, m.fwErr = true, ""
		}
		return
	}
	m.fwClean = false
	err := m.rules.Ensure(ctx, m.gw.Interfaces())
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if msg != m.fwErr && msg != "" {
		m.logger.Warn("mesh dns: could not ensure the firewall accept for port 53", slog.String("error", msg))
	}
	m.fwErr = msg
}

// dnsFor is the create-time DNS pick: only a local node with a confirmed gateway listener gets one.
func (m *meshGatewayDNS) dnsFor(nodeID string) string {
	return pickContainerDNS(nodeID == "" || nodeID == m.localNodeID, m.gatewayConfirmed(), m.legacy)
}

func (m *meshGatewayDNS) gatewayConfirmed() string {
	if !m.settings.Bool(context.Background(), runtimesettings.KeyDNSMeshResolverEnabled) {
		return ""
	}
	if gw := m.defaultGW.Load(); gw != nil && m.gw.Confirmed(*gw) {
		return gw.String()
	}
	return ""
}

// pickContainerDNS never returns an address for a remote node: no agent runs a resolver, so it would be dead.
func pickContainerDNS(local bool, gateway, legacy string) string {
	if !local {
		return ""
	}
	if gateway != "" {
		return gateway
	}
	return legacy
}

func (m *meshGatewayDNS) status(ctx context.Context) network.GatewayStatus {
	st := network.GatewayStatus{GatewayReport: m.gw.Report(), FirewallBackend: string(m.rules.Backend())}
	if !st.Enabled {
		return st
	}
	if ifaces := m.gw.Interfaces(); len(ifaces) > 0 {
		present, err := m.rules.Present(ctx, ifaces)
		st.FirewallPresent = present
		st.FirewallFindings = m.rules.Diagnose(ctx, ifaces)
		if err != nil {
			st.FirewallError = err.Error()
		}
	}
	addrs := map[string]bool{}
	for _, l := range st.Listeners {
		if host, _, err := net.SplitHostPort(l.Addr); err == nil {
			addrs[strings.ToLower(host)] = true
		}
	}
	if using, total, err := m.client.DNSUsage(ctx, addrs); err == nil {
		st.ContainersUsing, st.ContainersTotal = using, total
	}
	return st
}
