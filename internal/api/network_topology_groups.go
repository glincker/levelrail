package api

import (
	"net/netip"
	"os"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Topology group kinds.
const (
	topologyGroupLocal    = "local"
	topologyGroupNode     = "node"
	topologyGroupUnplaced = "unplaced"
)

// networkTopologyGroup is one card on the Network page: this server, an
// enrolled node, or the placements that match no known node.
type networkTopologyGroup struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// NodeID is the nodes-table ID when this group has a row; empty for a
	// control plane that has never registered itself (mesh off).
	NodeID      string `json:"node_id,omitempty"`
	Name        string `json:"name"`
	Hostname    string `json:"hostname,omitempty"`
	Address     string `json:"address,omitempty"`
	Region      string `json:"region,omitempty"`
	Status      string `json:"status,omitempty"`
	Schedulable bool   `json:"schedulable"`
	MeshAddress string `json:"mesh_address,omitempty"`
	// StaleNodeIDs lists the unknown placement IDs behind an unplaced group.
	StaleNodeIDs []string                          `json:"stale_node_ids,omitempty"`
	Apps         []networkTopologyAppResource      `json:"apps"`
	Databases    []networkTopologyDatabaseResource `json:"databases"`
}

type topologyGroupInput struct {
	nodes       []store.Node
	apps        []networkTopologyAppResource
	databases   []networkTopologyDatabaseResource
	localNodeID string
	hostname    string
	address     string
}

// groupTopology places every app and database on this server, a known node,
// or an unplaced group. An empty placement and the local node's own ID both
// mean this server, so a single-node install never shows "unplaced".
func groupTopology(in topologyGroupInput) []networkTopologyGroup {
	byID := make(map[string]store.Node, len(in.nodes))
	for _, n := range in.nodes {
		byID[n.ID] = n
	}

	local := networkTopologyGroup{
		Key: topologyGroupLocal, Kind: topologyGroupLocal, Name: in.hostname, Hostname: in.hostname,
		Address: in.address, Schedulable: true,
		Apps: []networkTopologyAppResource{}, Databases: []networkTopologyDatabaseResource{},
	}
	if n, ok := byID[in.localNodeID]; ok && in.localNodeID != "" {
		local.NodeID, local.Region, local.Status = n.ID, n.Region, string(n.Status)
		local.Schedulable, local.MeshAddress = n.Schedulable, n.MeshAddress
	}

	groups := map[string]*networkTopologyGroup{}
	var order []string
	for _, n := range in.nodes {
		if n.ID == in.localNodeID {
			continue
		}
		groups[n.ID] = &networkTopologyGroup{
			Key: n.ID, Kind: topologyGroupNode, NodeID: n.ID, Name: n.Name, Region: n.Region,
			Status: string(n.Status), Schedulable: n.Schedulable, MeshAddress: n.MeshAddress,
			Apps: []networkTopologyAppResource{}, Databases: []networkTopologyDatabaseResource{},
		}
		order = append(order, n.ID)
	}
	sort.Slice(order, func(i, j int) bool { return groups[order[i]].Name < groups[order[j]].Name })

	unplaced := networkTopologyGroup{
		Key: topologyGroupUnplaced, Kind: topologyGroupUnplaced, StaleNodeIDs: []string{},
		Apps: []networkTopologyAppResource{}, Databases: []networkTopologyDatabaseResource{},
	}
	stale := map[string]struct{}{}
	pick := func(nodeID string) *networkTopologyGroup {
		if nodeID == "" || nodeID == in.localNodeID {
			return &local
		}
		if g, ok := groups[nodeID]; ok {
			return g
		}
		stale[nodeID] = struct{}{}
		return &unplaced
	}
	for _, a := range in.apps {
		g := pick(a.NodeID)
		g.Apps = append(g.Apps, a)
	}
	for _, d := range in.databases {
		g := pick(d.NodeID)
		g.Databases = append(g.Databases, d)
	}

	out := []networkTopologyGroup{local}
	for _, id := range order {
		out = append(out, *groups[id])
	}
	if len(unplaced.Apps)+len(unplaced.Databases) > 0 {
		for id := range stale {
			unplaced.StaleNodeIDs = append(unplaced.StaleNodeIDs, id)
		}
		sort.Strings(unplaced.StaleNodeIDs)
		out = append(out, unplaced)
	}
	return out
}

// localAddress is the advertise host when it is a real address, else "".
func localAddress(advertiseHost string) string {
	h := strings.TrimSpace(advertiseHost)
	if h == "" || h == "localhost" {
		return ""
	}
	if ip, err := netip.ParseAddr(h); err == nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return ""
	}
	return h
}

// hostname hides the capture machine's real name in demo mode.
func (rt *Router) hostname() string {
	if rt.demo != nil {
		return "demo-01"
	}
	return localHostname()
}

func localHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "this server"
	}
	return h
}
