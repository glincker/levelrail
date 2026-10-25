package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

// placementResolver turns a stored node_id into the display fields apps and
// databases carry. An empty id and the local node's own id both mean this
// server; an id that matches no node row resolves to no name.
type placementResolver struct {
	localNodeID string
	hostname    string
	names       map[string]string
	offline     map[string]bool
}

func newPlacementResolver(nodes []store.Node, localNodeID, hostname string) placementResolver {
	names := make(map[string]string, len(nodes))
	offline := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		names[n.ID] = n.Name
		offline[n.ID] = n.Status == store.NodeStatusOffline
	}
	return placementResolver{localNodeID: localNodeID, hostname: hostname, names: names, offline: offline}
}

// resolve returns the node name and whether the placement is this server.
// An unknown remote id returns an empty name and false.
func (p placementResolver) resolve(nodeID string) (name string, isLocal bool) {
	if nodeID == "" || nodeID == p.localNodeID {
		if p.localNodeID != "" && p.names[p.localNodeID] != "" {
			return p.names[p.localNodeID], true
		}
		return p.hostname, true
	}
	return p.names[nodeID], false
}

func (rt *Router) placementResolver(ctx context.Context) placementResolver {
	var nodes []store.Node
	if rt.nodes != nil {
		var err error
		nodes, err = rt.nodes.ListNodes(ctx)
		if err != nil {
			rt.logger.Warn("api: placement resolver: list nodes failed", slog.String("error", err.Error()))
		}
	}
	return newPlacementResolver(nodes, rt.localNodeID, localHostname())
}

func (p placementResolver) stampApp(res *appResource) {
	res.NodeName, res.NodeIsLocal = p.resolve(res.NodeID)
	res.NodeOffline = p.offline[res.NodeID]
}

func (p placementResolver) stampDatabase(res *databaseResource) {
	res.NodeName, res.NodeIsLocal = p.resolve(res.NodeID)
}
