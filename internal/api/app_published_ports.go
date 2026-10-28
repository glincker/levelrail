package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Error codes for direct host ports (UDP) on a compose deploy.
const (
	codePublishedPortInvalid    = "published_port_invalid"
	codePublishedPortConflict   = "published_port_conflict"
	codePublishedPortRemoteNode = "published_port_remote_node"
	codePublishedPortInUse      = "published_port_in_use"
)

// procNetUDPTables are read for bound UDP sockets; absent off Linux.
var procNetUDPTables = []string{"/proc/net/udp", "/proc/net/udp6"}

// hostBoundUDP reads this server's bound UDP sockets. Like the TCP listener
// read, a missing table is skipped rather than failing the deploy; replaced
// in tests.
var hostBoundUDP = func() []diagnose.BoundSocket {
	var tables []string
	for _, p := range procNetUDPTables {
		if b, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed /proc paths
			tables = append(tables, string(b))
		}
	}
	return diagnose.ParseBoundUDP(tables...)
}

// udpBindClashes mirrors the kernel's UDP bind rule without SO_REUSEADDR:
// two binds on one port clash when either is a wildcard of the same family
// or both name the same address. A [::] socket is dual stack and clashes
// with everything.
func udpBindClashes(bind, sock netip.Addr) bool {
	bind, sock = bind.Unmap(), sock.Unmap()
	if sock.Is6() && sock.IsUnspecified() {
		return true
	}
	if bind.Is4() != sock.Is4() {
		return false
	}
	return bind.IsUnspecified() || sock.IsUnspecified() || bind == sock
}

// udpHostConflict names the first new UDP published port another process on
// this host already binds. Ports this app published before are skipped, its
// own running container holds them.
func udpHostConflict(previous, next []store.DesiredService) (store.PublishedPort, string, netip.AddrPort, bool) {
	held := map[store.PublishedPort]bool{}
	for _, svc := range previous {
		for _, p := range svc.PublishedPorts {
			held[portKey(p)] = true
		}
	}
	var socks []diagnose.BoundSocket
	read := false
	for _, svc := range next {
		ip, err := bindaddr.Resolve(svc.BindAddress)
		if err != nil {
			continue
		}
		bind, err := netip.ParseAddr(ip)
		if err != nil {
			continue
		}
		for _, p := range svc.PublishedPorts {
			if p.EffectiveProtocol() != spec.PortProtocolUDP || held[portKey(p)] {
				continue
			}
			if !read {
				socks, read = hostBoundUDP(), true
			}
			for _, s := range socks {
				if s.Port == p.HostPort && udpBindClashes(bind, s.Addr) {
					return p, svc.Name, netip.AddrPortFrom(s.Addr.Unmap(), uint16(s.Port)), true //nolint:gosec // port parsed below 65536
				}
			}
		}
	}
	return store.PublishedPort{}, "", netip.AddrPort{}, false
}

// admitPublishedPorts refuses invalid direct host ports, ones another app
// already holds, keyed by (host port, protocol), or ones on a service placed
// on a remote node, where node firewall and exposure rules are not managed.
// The returned release must run after the caller saves, so a concurrent
// deploy cannot pass the same check before this one's ports are stored.
func (rt *Router) admitPublishedPorts(w http.ResponseWriter, r *http.Request, appName string, previous, next []store.DesiredService) (func(), bool) {
	release, err := rt.admitHostPorts(r.Context(), appName, previous, next)
	if err == nil {
		return release, true
	}
	var refusal *portRefusal
	if errors.As(err, &refusal) {
		writeCodedError(w, refusal.status, refusal.code, refusal.msg)
		return nil, false
	}
	rt.internalError(w, "api: published ports: admission check failed", err, slog.String("app", appName))
	return nil, false
}

// AdmitPublishedPorts runs the API's host port admission for one service a
// spec file deploy (app.yaml via webhook, git push or build) is about to save.
func (rt *Router) AdmitPublishedPorts(ctx context.Context, desired store.DesiredService) (func(), error) {
	appName := desired.AppID
	var previous []store.DesiredService
	prev, err := rt.apps.GetDesiredService(ctx, desired.Name)
	switch {
	case err == nil:
		previous = []store.DesiredService{*prev}
		if appName == "" {
			appName = prev.AppID
		}
	case !errors.Is(err, store.ErrServiceNotFound):
		return nil, fmt.Errorf("load service %q for port check: %w", desired.Name, err)
	}
	if appName == "" {
		appName = desired.Name
	}
	return rt.admitHostPorts(ctx, appName, previous, []store.DesiredService{desired})
}

// portRefusal is an admission refusal with the status and code the API
// answers with; spec deploys show its message as the deploy failure.
type portRefusal struct {
	status int
	code   string
	msg    string
}

func (e *portRefusal) Error() string { return e.msg }

func refusePort(status int, code, msg string) error {
	return &portRefusal{status: status, code: code, msg: msg}
}

func (rt *Router) admitHostPorts(ctx context.Context, appName string, previous, next []store.DesiredService) (func(), error) {
	// Only a deploy that claims a host port can race another one for it.
	if !claimsHostPort(next) {
		return func() {}, rt.checkPublishedPorts(ctx, appName, previous, next)
	}
	rt.publishedPortsMu.Lock()
	if err := rt.checkPublishedPorts(ctx, appName, previous, next); err != nil {
		rt.publishedPortsMu.Unlock()
		return nil, err
	}
	if err := rt.pinnedHostPortsFree(ctx, previous, next); err != nil {
		rt.publishedPortsMu.Unlock()
		return nil, err
	}
	return rt.publishedPortsMu.Unlock, nil
}

// pinnedHostPortsFree runs the API's pinned host_port checks (control plane
// ports, another app's pin, a host listener) on every deploy path.
func (rt *Router) pinnedHostPortsFree(ctx context.Context, previous, next []store.DesiredService) error {
	prev := make(map[string]*store.DesiredService, len(previous))
	for i := range previous {
		prev[previous[i].Name] = &previous[i]
	}
	reserved := rt.reservedPublishedPorts(ctx)
	for _, svc := range next {
		if svc.HostPort == nil {
			continue
		}
		existing, node := prev[svc.Name], svc.NodeID
		if existing != nil {
			node = existing.NodeID
		}
		if node == rt.localNodeID {
			node = ""
		}
		unchanged := existing != nil && existing.HostPort != nil && *existing.HostPort == *svc.HostPort
		if label, ok := reserved[*svc.HostPort]; ok && !unchanged && node == "" {
			return refusePort(http.StatusConflict, codePublishedPortConflict,
				fmt.Sprintf("host port %d/tcp for service %q is reserved by the control plane (%s)", *svc.HostPort, svc.Name, label))
		}
		if eerr := rt.hostPortConflict(ctx, svc.Name, node, svc.HostPort, existing); eerr != nil {
			return refusePort(eerr.status, codePublishedPortConflict, eerr.msg)
		}
	}
	return nil
}

func claimsHostPort(services []store.DesiredService) bool {
	for _, svc := range services {
		if len(svc.PublishedPorts) > 0 || svc.HostPort != nil {
			return true
		}
	}
	return false
}

// reservedPublishedPorts are host ports the control plane listens on, kept
// off both protocols (443/udp carries HTTP/3); the mesh adds its UDP port.
func (rt *Router) reservedPublishedPorts(ctx context.Context) map[int]string {
	out := map[int]string{}
	for p := range reservedControlPlanePorts {
		out[p] = "ingress or dashboard"
	}
	for _, cp := range rt.standaloneSafety.ControlPorts {
		out[cp.Port] = cp.Label
	}
	for _, p := range rt.standaloneSafety.IngressPorts {
		out[p] = "ingress"
	}
	if rt.mesh != nil {
		port := network.DefaultListenPort
		if st, err := rt.mesh.Status(ctx); err == nil && st.ListenPort > 0 {
			port = st.ListenPort
		}
		out[port] = "mesh WireGuard"
	}
	for _, p := range rt.firewallRequiredPorts {
		out[p] = "required by the firewall guard"
	}
	for _, p := range rt.hostFirewallSSHPorts {
		out[p] = "ssh"
	}
	return out
}

func (rt *Router) checkPublishedPorts(ctx context.Context, appName string, previous, next []store.DesiredService) error {
	nodeOf := make(map[string]string, len(previous))
	for _, p := range previous {
		nodeOf[p.Name] = p.NodeID
	}
	want := map[store.PublishedPort]string{}
	self := make(map[string]bool, len(next))
	for _, svc := range next {
		self[svc.Name] = true
		if err := spec.ValidatePublishedPorts(svc.PublishedPorts); err != nil {
			return refusePort(http.StatusBadRequest, codePublishedPortInvalid, err.Error())
		}
		node, known := nodeOf[svc.Name]
		if !known {
			node = svc.NodeID
		}
		for _, p := range svc.PublishedPorts {
			if node != "" && node != rt.localNodeID {
				return refusePort(http.StatusBadRequest, codePublishedPortRemoteNode,
					fmt.Sprintf("host port %s can only be published on the control plane's own node, service %q is placed on node %q", p, svc.Name, node))
			}
			want[portKey(p)] = svc.Name
		}
	}
	pinned := map[int]string{}
	for _, svc := range next {
		if svc.HostPort == nil {
			continue
		}
		pinned[*svc.HostPort] = svc.Name
		if mine, taken := want[store.PublishedPort{HostPort: *svc.HostPort, Protocol: spec.PortProtocolTCP}]; taken {
			return refusePort(http.StatusConflict, codePublishedPortConflict,
				fmt.Sprintf("host port %d/tcp for service %q is already the host port of %q", *svc.HostPort, mine, svc.Name))
		}
	}
	if len(want) == 0 && len(pinned) == 0 {
		return nil
	}
	reserved := rt.reservedPublishedPorts(ctx)
	for p, svc := range want {
		if label, ok := reserved[p.HostPort]; ok {
			return refusePort(http.StatusConflict, codePublishedPortConflict,
				fmt.Sprintf("host port %s for service %q is reserved by the control plane (%s)", p, svc, label))
		}
	}
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return fmt.Errorf("list services for port check: %w", err)
	}
	for _, other := range all {
		if other.AppID == appName || self[other.Name] {
			continue
		}
		if other.HostPort != nil {
			if mine, taken := want[store.PublishedPort{HostPort: *other.HostPort, Protocol: spec.PortProtocolTCP}]; taken {
				return refusePort(http.StatusConflict, codePublishedPortConflict,
					fmt.Sprintf("host port %d/tcp for service %q is already the host port of %q", *other.HostPort, mine, other.Name))
			}
		}
		for _, p := range other.PublishedPorts {
			if mine, taken := pinned[p.HostPort]; taken && p.EffectiveProtocol() == spec.PortProtocolTCP {
				return refusePort(http.StatusConflict, codePublishedPortConflict,
					fmt.Sprintf("host port %d/tcp for service %q is already published by %q", p.HostPort, mine, other.Name))
			}
			if mine, taken := want[portKey(p)]; taken {
				return refusePort(http.StatusConflict, codePublishedPortConflict,
					fmt.Sprintf("host port %s for service %q is already published by %q", p, mine, other.Name))
			}
		}
	}
	if err := rt.publishedPortsFreeOnHost(ctx, self, previous, want); err != nil {
		return err
	}
	// After the app check, so a port another app holds names that app.
	if p, svc, sock, busy := udpHostConflict(previous, next); busy {
		return refusePort(http.StatusConflict, codePublishedPortInUse,
			fmt.Sprintf("host port %s for service %q is already bound by another process on this server (%s), stop it or pick another host port", p, svc, sock))
	}
	return nil
}

// publishedPortsFreeOnHost refuses a port an app stream holds, and a new tcp
// port a host process already listens on (this app's own ports are skipped).
func (rt *Router) publishedPortsFreeOnHost(ctx context.Context, self map[string]bool, previous []store.DesiredService, want map[store.PublishedPort]string) error {
	if len(want) == 0 {
		return nil
	}
	held := map[int]bool{}
	for _, svc := range previous {
		for _, p := range svc.PublishedPorts {
			held[p.HostPort] = true
		}
	}
	for _, port := range hostListeningPorts() {
		if mine, taken := want[store.PublishedPort{HostPort: port, Protocol: spec.PortProtocolTCP}]; taken && !held[port] {
			return refusePort(http.StatusConflict, codePublishedPortInUse,
				fmt.Sprintf("host port %d/tcp for service %q is already listened on by another process on this server, stop it or pick another host port", port, mine))
		}
	}
	if rt.appStreams == nil {
		return nil
	}
	streams, err := rt.appStreams.ListAllAppStreams(ctx)
	if err != nil {
		return fmt.Errorf("list streams for port check: %w", err)
	}
	for _, st := range streams {
		if self[st.ServiceName] {
			continue
		}
		if mine, taken := want[store.PublishedPort{HostPort: st.HostPort, Protocol: st.Protocol}]; taken {
			return refusePort(http.StatusConflict, codePublishedPortConflict,
				fmt.Sprintf("host port %d/%s for service %q is already used by a stream on %q", st.HostPort, st.Protocol, mine, st.ServiceName))
		}
	}
	return nil
}

func portKey(p store.PublishedPort) store.PublishedPort {
	return store.PublishedPort{HostPort: p.HostPort, Protocol: p.EffectiveProtocol()}
}
