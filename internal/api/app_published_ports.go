package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Error codes for direct host ports (UDP) on a compose deploy.
const (
	codePublishedPortInvalid    = "published_port_invalid"
	codePublishedPortConflict   = "published_port_conflict"
	codePublishedPortRemoteNode = "published_port_remote_node"
)

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
	return true
}

func portKey(p store.PublishedPort) store.PublishedPort {
	return store.PublishedPort{HostPort: p.HostPort, Protocol: p.EffectiveProtocol()}
}
