package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/diagnose"
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

// admitPublishedPorts refuses a deploy whose direct host ports another app
// already holds, keyed by (host port, protocol), or whose service sits on
// a remote node, where node firewall and exposure rules are not managed.
func (rt *Router) admitPublishedPorts(w http.ResponseWriter, r *http.Request, appName string, previous, next []store.DesiredService) bool {
	nodeOf := make(map[string]string, len(previous))
	for _, p := range previous {
		nodeOf[p.Name] = p.NodeID
	}
	want := map[store.PublishedPort]string{}
	for _, svc := range next {
		for _, p := range svc.PublishedPorts {
			if node := nodeOf[svc.Name]; node != "" && node != rt.localNodeID {
				writeCodedError(w, http.StatusBadRequest, codePublishedPortRemoteNode,
					fmt.Sprintf("host port %s can only be published on the control plane's own node, service %q is placed on node %q", p, svc.Name, node))
				return false
			}
			want[portKey(p)] = svc.Name
		}
	}
	if len(want) == 0 {
		return true
	}
	all, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: deploy compose: list services for port check failed", err, slog.String("app", appName))
		return false
	}
	for _, other := range all {
		if other.AppID == appName {
			continue
		}
		for _, p := range other.PublishedPorts {
			if mine, taken := want[portKey(p)]; taken {
				writeCodedError(w, http.StatusConflict, codePublishedPortConflict,
					fmt.Sprintf("host port %s for service %q is already published by %q", p, mine, other.Name))
				return false
			}
		}
	}
	// After the app check, so a port another app holds names that app.
	if p, svc, sock, busy := udpHostConflict(previous, next); busy {
		writeCodedError(w, http.StatusConflict, codePublishedPortInUse,
			fmt.Sprintf("host port %s for service %q is already bound by another process on this server (%s), stop it or pick another host port", p, svc, sock))
		return false
	}
	return true
}

func portKey(p store.PublishedPort) store.PublishedPort {
	return store.PublishedPort{HostPort: p.HostPort, Protocol: p.EffectiveProtocol()}
}
